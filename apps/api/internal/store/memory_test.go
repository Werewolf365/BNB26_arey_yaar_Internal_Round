package store_test

import (
	"context"
	"testing"

	"github.com/quorum/quorum/apps/api/internal/store"
)

// Direct unit tests for the memory backend (the Postgres backend is covered
// by the env-gated integration suite; the server suite covers both via API).
func TestMemoryStoreCRUD(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemoryStore()
	if err := m.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	rel, dup, err := m.CreateRelease(ctx, store.Release{Package: "p", Repo: "https://github.com/e/t", Commit: "c"}, "k1")
	if err != nil || dup || rel.Status != "PENDING" {
		t.Fatalf("create: %v %v %+v", err, dup, rel)
	}
	if _, dup, err := m.CreateRelease(ctx, store.Release{Repo: "https://github.com/e/t", Commit: "c"}, "k1"); err != nil || !dup {
		t.Fatal("idempotent replay")
	}
	if _, err := m.GetRelease(ctx, "nope"); err == nil {
		t.Fatal("missing release")
	}
	if _, err := m.ListReleases(ctx, 10); err != nil {
		t.Fatal(err)
	}
	v, dup, err := m.CreateVerification(ctx, store.Verification{ReleaseID: rel.ID, Decision: "VERIFIED"}, "vk1")
	if err != nil || dup || v.ID == "" {
		t.Fatalf("create verification: %v", err)
	}
	if _, _, err := m.CreateVerification(ctx, store.Verification{ReleaseID: "nope"}, "vk2"); err == nil {
		t.Fatal("orphan verification must fail")
	}
	if _, err := m.GetVerification(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpsertBuilder(ctx, store.Builder{ID: "b1", IndependenceGroup: "g1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpsertBuilder(ctx, store.Builder{ID: "b2"}); err == nil {
		t.Fatal("missing group must fail")
	}
	off := false
	if b, err := m.PatchBuilder(ctx, "b1", &off); err != nil || b.Enabled {
		t.Fatal("disable")
	}
	if _, err := m.PatchBuilder(ctx, "nope", &off); err == nil {
		t.Fatal("patch missing")
	}
	if _, err := m.CreatePolicy(ctx, store.PolicyRecord{ID: "p1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreatePolicy(ctx, store.PolicyRecord{ID: "p1"}); err == nil {
		t.Fatal("duplicate policy")
	}
	if _, err := m.GetPolicy(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	a1, err := m.AppendAudit(ctx, "e1", map[string]any{"x": 1}, "")
	if err != nil || a1.PreviousHash != "GENESIS" {
		t.Fatal("first audit")
	}
	a2, err := m.AppendAudit(ctx, "e2", map[string]any{"x": 2}, "")
	if err != nil || a2.PreviousHash != a1.RecordHash {
		t.Fatal("chained audit")
	}
	recs, err := m.ListAudit(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := store.VerifyChain(recs); !ok {
		t.Fatal("chain must verify")
	}
	if _, err := m.PutAnchor(ctx, store.Anchor{VerificationID: v.ID, TxHash: "0x1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.PutAnchor(ctx, store.Anchor{VerificationID: v.ID, TxHash: "0x2"}); err == nil {
		t.Fatal("duplicate anchor")
	}
	if _, err := m.PutAnchor(ctx, store.Anchor{VerificationID: "nope", TxHash: "0x3"}); err == nil {
		t.Fatal("orphan anchor")
	}
	eo, err := m.PutEvidence(ctx, store.EvidenceObject{VerificationID: v.ID, Kind: "k", StorageKey: "s", SHA256: "h"})
	if err != nil || eo.ID == "" {
		t.Fatal("put evidence")
	}
	if _, err := m.GetEvidence(ctx, eo.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetEvidence(ctx, "nope"); err == nil {
		t.Fatal("missing evidence")
	}
}
