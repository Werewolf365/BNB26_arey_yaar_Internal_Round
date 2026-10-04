package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/quorum/quorum/apps/api/internal/store"
)

func seedVerification(t *testing.T, m *store.MemoryStore) string {
	t.Helper()
	ctx := context.Background()
	rel, _, err := m.CreateRelease(ctx, store.Release{Repo: "https://github.com/example/t", Commit: "abc"}, "")
	if err != nil {
		t.Fatal(err)
	}
	ver, _, err := m.CreateVerification(ctx, store.Verification{ReleaseID: rel.ID, PolicyID: "policy-a", Decision: "INSUFFICIENT_EVIDENCE"}, "")
	if err != nil {
		t.Fatal(err)
	}
	return ver.ID
}

func TestMemoryJobsQueue(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemoryStore()

	if _, err := m.EnqueueJobs(ctx, "ver_nope", []string{"b1"}); err == nil {
		t.Fatal("enqueue on missing verification must fail")
	}
	vid := seedVerification(t, m)
	jobs, err := m.EnqueueJobs(ctx, vid, []string{"b1", "b2"})
	if err != nil || len(jobs) != 2 || jobs[0].Status != "QUEUED" || jobs[0].MaxAttempts != 3 {
		t.Fatalf("enqueue: %+v %v", jobs, err)
	}
	again, err := m.EnqueueJobs(ctx, vid, []string{"b1"})
	if err != nil || len(again) != 1 || again[0].ID != jobs[0].ID {
		t.Fatalf("duplicate enqueue must return existing: %+v %v", again, err)
	}
	all, err := m.ListJobs(ctx, "")
	if err != nil || len(all) != 2 {
		t.Fatalf("list all: %+v %v", all, err)
	}
	one, err := m.ListJobs(ctx, vid)
	if err != nil || len(one) != 2 {
		t.Fatalf("list filtered: %+v %v", one, err)
	}
	if _, err := m.GetJob(ctx, "job_nope"); err == nil {
		t.Fatal("get missing job must fail")
	}
	if _, _, err := m.ClaimJob(ctx, "", time.Minute, ""); err == nil {
		t.Fatal("claim without owner must fail")
	}
	j1, claimed, err := m.ClaimJob(ctx, "w1", 30*time.Millisecond, "")
	if err != nil || !claimed || j1.LeaseOwner != "w1" || j1.Attempts != 1 {
		t.Fatalf("claim: %+v %v %v", j1, claimed, err)
	}
	// Second worker gets the other job.
	if _, claimed, err := m.ClaimJob(ctx, "w2", time.Minute, ""); err != nil || !claimed {
		t.Fatalf("second claim: %v %v", claimed, err)
	}
	// Nothing left claimable right now.
	if _, claimed, err := m.ClaimJob(ctx, "w3", time.Minute, ""); err != nil || claimed {
		t.Fatalf("empty queue must not claim: %v %v", claimed, err)
	}
	// After the short lease expires the first job is reclaimable.
	time.Sleep(50 * time.Millisecond)
	jr, claimed, err := m.ClaimJob(ctx, "w3", time.Minute, "")
	if err != nil || !claimed || jr.ID != j1.ID || jr.Attempts != 2 {
		t.Fatalf("expiry reclaim: %+v %v %v", jr, claimed, err)
	}
	// Fail twice (attempts 2 < max 3): requeued, not terminal.
	for i := 0; i < 2; i++ {
		jf, err := m.CompleteJob(ctx, jr.ID, false, "", "", "BUILDER_FAILED", "boom", "", false)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && jf.Status != "QUEUED" {
			t.Fatalf("retryable failure must requeue, got %s", jf.Status)
		}
	}
	// Claim again (attempts 3) then fail: terminal FAILED.
	jc, claimed, err := m.ClaimJob(ctx, "w4", time.Minute, "")
	if err != nil || !claimed {
		t.Fatalf("reclaim: %v %v", claimed, err)
	}
	jf, err := m.CompleteJob(ctx, jc.ID, false, "", "", "BUILDER_FAILED", "boom", "", false)
	if err != nil || jf.Status != "FAILED" || jf.ErrorCode != "BUILDER_FAILED" {
		t.Fatalf("exhausted failure must be FAILED: %+v %v", jf, err)
	}
	if _, err := m.CompleteJob(ctx, jc.ID, true, "d", "c", "", "", "", false); err == nil {
		t.Fatal("completing a terminal job must conflict")
	}
	if _, err := m.CompleteJob(ctx, "job_nope", true, "d", "c", "", "", "", false); err == nil {
		t.Fatal("completing a missing job must fail")
	}
	// Success path on the remaining job.
	rest, _ := m.ListJobs(ctx, vid)
	var other string
	for _, j := range rest {
		if j.ID != jc.ID {
			other = j.ID
		}
	}
	done, err := m.CompleteJob(ctx, other, true, "sha256:abc", "abc", "", "", "", false)
	if err != nil || done.Status != "SUCCEEDED" || done.ResultDigest != "sha256:abc" {
		t.Fatalf("success: %+v %v", done, err)
	}
}

func TestMemoryAnchorsAndLists(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemoryStore()
	if _, err := m.GetAnchor(ctx, "nope"); err == nil {
		t.Fatal("missing anchor must fail")
	}
	vid := seedVerification(t, m)
	if _, err := m.PutAnchor(ctx, store.Anchor{VerificationID: "ver_nope", TxHash: "0x1"}); err == nil {
		t.Fatal("anchor on missing verification must fail")
	}
	a, err := m.PutAnchor(ctx, store.Anchor{VerificationID: vid, TxHash: "0x1", ChainID: "31337"})
	if err != nil || a.TxHash != "0x1" {
		t.Fatalf("put anchor: %+v %v", a, err)
	}
	got, err := m.GetAnchor(ctx, vid)
	if err != nil || got.ChainID != "31337" {
		t.Fatalf("get anchor: %+v %v", got, err)
	}
	if _, err := m.ListBuilders(ctx); err != nil {
		t.Fatalf("empty builders list: %v", err)
	}
	if _, err := m.ListPolicies(ctx); err != nil {
		t.Fatalf("empty policies list: %v", err)
	}
	if _, err := m.UpsertBuilder(ctx, store.Builder{ID: "b1", IndependenceGroup: "g1"}); err != nil {
		t.Fatal(err)
	}
	bs, err := m.ListBuilders(ctx)
	if err != nil || len(bs) != 1 {
		t.Fatalf("builders: %+v %v", bs, err)
	}
	if _, err := m.CreatePolicy(ctx, store.PolicyRecord{ID: "p1"}); err != nil {
		t.Fatal(err)
	}
	ps, err := m.ListPolicies(ctx)
	if err != nil || len(ps) != 1 {
		t.Fatalf("policies: %+v %v", ps, err)
	}
}

// Scoped claims isolate verifications: a worker bound to vid-a must never
// receive vid-b's jobs (reruns and parallel demos coexist safely).
func TestClaimJobScopedToVerification(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemoryStore()
	va := seedVerification(t, m)
	vb := seedVerification(t, m)
	if _, err := m.EnqueueJobs(ctx, va, []string{"ba"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EnqueueJobs(ctx, vb, []string{"bb"}); err != nil {
		t.Fatal(err)
	}
	// Unscoped still works (backward compatible): takes oldest overall.
	if _, claimed, err := m.ClaimJob(ctx, "w-any", time.Minute, ""); err != nil || !claimed {
		t.Fatalf("unscoped claim: %v %v", claimed, err)
	}
	// Scoped to vb skips va's leftovers and takes vb's job.
	j, claimed, err := m.ClaimJob(ctx, "w-b", time.Minute, vb)
	if err != nil || !claimed || j.VerificationID != vb {
		t.Fatalf("scoped claim must take own verification's job: %+v %v %v", j, claimed, err)
	}
	// Scoped to a drained verification claims nothing (never another's job).
	if _, claimed, err := m.ClaimJob(ctx, "w-a2", time.Minute, vb); err != nil || claimed {
		t.Fatalf("scoped claim on drained queue must not claim: %v %v", claimed, err)
	}
	// Unknown scope is simply empty (server maps to 404 before reaching here,
	// but the store must never leak across scopes).
	if _, claimed, err := m.ClaimJob(ctx, "w-x", time.Minute, "ver_nope"); err != nil || claimed {
		t.Fatalf("unknown scope must not claim: %v %v", claimed, err)
	}
}
