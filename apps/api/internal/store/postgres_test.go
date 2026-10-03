package store_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/services/policy"
)

// TestPostgresIntegration is env-gated (QUORUM_TEST_POSTGRES=1) and needs the
// compose postgres (postgres://quorum:quorum@localhost:5432/quorum by default).
// It exercises migrations, constraints, idempotency, and audit linkage.
func TestPostgresIntegration(t *testing.T) {
	if os.Getenv("QUORUM_TEST_POSTGRES") != "1" {
		t.Skip("set QUORUM_TEST_POSTGRES=1 with compose postgres running")
	}
	dsn := os.Getenv("QUORUM_TEST_DATABASE_URL")
	if dsn == "" {
		// Compose maps host 5433 -> container 5432 (host 5432 is often taken
		// by a machine-local PostgreSQL; see infra/compose/docker-compose.yml).
		dsn = "postgres://quorum:quorum@localhost:5433/quorum?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	uniq := fmt.Sprintf("%d", time.Now().UnixNano())
	pg, err := store.Connect(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	defer pg.Close()
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := pg.Migrate(ctx); err != nil {
		t.Fatalf("re-migrate must be idempotent: %v", err)
	}
	if err := pg.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	rel, dup, err := pg.CreateRelease(ctx, store.Release{Package: "p", Repo: "https://github.com/e/t", Commit: "abc123"}, "pg-key-1-"+uniq)
	if err != nil || dup {
		t.Fatalf("create: %v %v", err, dup)
	}
	again, dup, err := pg.CreateRelease(ctx, store.Release{Package: "p", Repo: "https://github.com/e/t", Commit: "abc123"}, "pg-key-1-"+uniq)
	if err != nil || !dup || again.ID != rel.ID {
		t.Fatalf("idempotent replay: %v %v", err, dup)
	}
	if _, err := pg.GetRelease(ctx, "rel_missing"); err == nil {
		t.Fatal("missing release must 404")
	}
	d := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	ev := []policy.Evidence{
		{BuilderID: "a", IndependenceGroup: "g1", SourceCommit: "abc123", ArtifactDigest: d, SignatureValid: true, VerificationSource: "builder"},
		{BuilderID: "b", IndependenceGroup: "g2", SourceCommit: "abc123", ArtifactDigest: d, SignatureValid: true, VerificationSource: "builder"},
	}
	res := policy.Evaluate(policy.Policy{PolicyID: "p", Version: "v1", MinBuilders: 1, RequiredAgreement: 1, RequiredIndependentGroups: 1, ConflictTolerance: 1}, "abc123", ev)
	ver, dup, err := pg.CreateVerification(ctx, store.Verification{ReleaseID: rel.ID, PolicyID: "p", Decision: res.Decision, Required: res.Required, Satisfied: res.Satisfied, Result: res, Evidence: ev, ExpectedSrc: "abc123"}, "pg-vkey-1-"+uniq)
	if err != nil || dup || ver.Decision == "" {
		t.Fatalf("create verification: %v", err)
	}
	if _, _, err := pg.CreateVerification(ctx, store.Verification{ReleaseID: "rel_missing"}, "pg-vkey-2"); err == nil {
		t.Fatal("verification for missing release must fail (FK)")
	}
	if _, err := pg.UpsertBuilder(ctx, store.Builder{ID: "pg-builder-a-" + uniq, IndependenceGroup: "cloud-a"}); err != nil {
		t.Fatal(err)
	}
	bs, err := pg.ListBuilders(ctx)
	if err != nil || len(bs) == 0 {
		t.Fatal("list builders")
	}
	off := false
	if _, err := pg.PatchBuilder(ctx, "pg-builder-a-"+uniq, &off); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.CreatePolicy(ctx, store.PolicyRecord{ID: "pg-policy-a-" + uniq, Version: "v1", Body: resPolicy(), PolicyHash: "sha256:x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.CreatePolicy(ctx, store.PolicyRecord{ID: "pg-policy-a-" + uniq, Version: "v1", Body: resPolicy(), PolicyHash: "sha256:x"}); err == nil {
		t.Fatal("duplicate policy must fail (unique)")
	}
	if _, err := pg.AppendAudit(ctx, "test.event", map[string]any{"k": "v"}, ver.ID); err != nil {
		t.Fatal(err)
	}
	recs, err := pg.ListAudit(ctx, 50)
	if err != nil || len(recs) == 0 {
		t.Fatal("list audit")
	}
	if ok, at, _ := store.VerifyChain(recs); !ok {
		t.Fatalf("persisted chain must verify (broken at %d)", at)
	}
	if _, err := pg.PutAnchor(ctx, store.Anchor{VerificationID: ver.ID, TxHash: "0xabc", ChainID: "0x7a69", ContractAddress: "0x5FbDB2315678afecb367f032d93F642f64180aa3", EventFound: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.PutAnchor(ctx, store.Anchor{VerificationID: ver.ID, TxHash: "0xdef", ChainID: "0x7a69", ContractAddress: "0x5FbDB2315678afecb367f032d93F642f64180aa3"}); err == nil {
		t.Fatal("duplicate anchor must fail (PK)")
	}
	if _, err := pg.GetAnchor(ctx, ver.ID); err != nil {
		t.Fatal(err)
	}
	eo, err := pg.PutEvidence(ctx, store.EvidenceObject{VerificationID: ver.ID, Kind: "attestation", StorageKey: "sha256/abc", SHA256: "abc"})
	if err != nil || eo.ID == "" {
		t.Fatal("put evidence")
	}
	if _, err := pg.GetEvidence(ctx, eo.ID); err != nil {
		t.Fatal(err)
	}

	// Job queue: enqueue (idempotent), claim, complete, lease expiry.
	if err := pg.DeleteAllJobs(ctx); err != nil {
		t.Fatalf("cleanup jobs: %v", err)
	}
	if _, err := pg.UpsertBuilder(ctx, store.Builder{ID: "pg-builder-a-" + uniq, IndependenceGroup: "cloud-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.UpsertBuilder(ctx, store.Builder{ID: "pg-builder-b-" + uniq, IndependenceGroup: "cloud-b"}); err != nil {
		t.Fatal(err)
	}
	jobs, err := pg.EnqueueJobs(ctx, ver.ID, []string{"pg-builder-a-" + uniq, "pg-builder-b-" + uniq})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := pg.EnqueueJobs(ctx, "ver_missing", []string{"pg-builder-a-" + uniq}); err == nil {
		t.Fatal("enqueue for missing verification must fail")
	}
	requeued, err := pg.EnqueueJobs(ctx, ver.ID, []string{"pg-builder-a-" + uniq})
	if err != nil || len(requeued) != 1 || requeued[0].ID != jobs[0].ID {
		t.Fatal("re-enqueue must converge")
	}
	j1, claimed, err := pg.ClaimJob(ctx, "worker-1", time.Second)
	if err != nil || !claimed || j1.Attempts != 1 {
		t.Fatalf("claim: %v %v", err, claimed)
	}
	if _, claimed, err := pg.ClaimJob(ctx, "worker-2", time.Minute); err != nil || !claimed {
		t.Fatalf("second claim takes the other job: %v", err)
	}
	if _, claimed, _ := pg.ClaimJob(ctx, "worker-3", time.Minute); claimed {
		t.Fatal("drained queue must not claim")
	}
	// Expire worker-1's lease by waiting, then reclaim.
	time.Sleep(1100 * time.Millisecond)
	j1b, claimed, err := pg.ClaimJob(ctx, "worker-3", time.Minute)
	if err != nil || !claimed || j1b.ID != j1.ID || j1b.Attempts != 2 {
		t.Fatalf("expired lease must be reclaimable: %v %v", err, claimed)
	}
	done, err := pg.CompleteJob(ctx, j1b.ID, true, "sha256:abc", "abc123", "", "", "", false)
	if err != nil || done.Status != "SUCCEEDED" {
		t.Fatalf("complete: %v", err)
	}
	if _, err := pg.CompleteJob(ctx, j1b.ID, true, "sha256:abc", "abc123", "", "", "", false); err == nil {
		t.Fatal("terminal job must reject recomplete")
	}
	if _, _, err := pg.ClaimJob(ctx, "", time.Minute); err == nil {
		t.Fatal("claim without owner must fail")
	}
}

func resPolicy() policy.Policy {
	return policy.Policy{PolicyID: "p", Version: "v1", MinBuilders: 1, RequiredAgreement: 1, RequiredIndependentGroups: 1, ConflictTolerance: 1}
}
