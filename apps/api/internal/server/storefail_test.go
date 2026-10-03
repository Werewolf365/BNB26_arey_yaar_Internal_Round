package server_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/quorum/quorum/apps/api/internal/server"
	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/services/policy"
	"github.com/quorum/quorum/services/storage"
)

// errorStore fails every operation: exercises handler store-failure paths
// and the 500 redaction branch (operational detail -> logs, never client).
type errorStore struct{ err error }

func (e errorStore) Migrate(ctx context.Context) error { return e.err }
func (e errorStore) Ping(ctx context.Context) error    { return e.err }
func (e errorStore) CreateRelease(ctx context.Context, r store.Release, k string) (store.Release, bool, error) {
	return store.Release{}, false, e.err
}
func (e errorStore) ListReleases(ctx context.Context, n int) ([]store.Release, error) {
	return nil, e.err
}
func (e errorStore) GetRelease(ctx context.Context, id string) (store.Release, error) {
	return store.Release{}, e.err
}
func (e errorStore) CreateVerification(ctx context.Context, v store.Verification, k string) (store.Verification, bool, error) {
	return store.Verification{}, false, e.err
}
func (e errorStore) GetVerification(ctx context.Context, id string) (store.Verification, error) {
	return store.Verification{}, e.err
}
func (e errorStore) UpsertBuilder(ctx context.Context, b store.Builder) (store.Builder, error) {
	return store.Builder{}, e.err
}
func (e errorStore) ListBuilders(ctx context.Context) ([]store.Builder, error) {
	return nil, e.err
}
func (e errorStore) PatchBuilder(ctx context.Context, id string, en *bool) (store.Builder, error) {
	return store.Builder{}, e.err
}
func (e errorStore) CreatePolicy(ctx context.Context, p store.PolicyRecord) (store.PolicyRecord, error) {
	return store.PolicyRecord{}, e.err
}
func (e errorStore) ListPolicies(ctx context.Context) ([]store.PolicyRecord, error) {
	return nil, e.err
}
func (e errorStore) GetPolicy(ctx context.Context, id string) (store.PolicyRecord, error) {
	return store.PolicyRecord{}, e.err
}
func (e errorStore) AppendAudit(ctx context.Context, ev string, p map[string]any, v string) (store.AuditRecord, error) {
	return store.AuditRecord{}, e.err
}
func (e errorStore) ListAudit(ctx context.Context, n int) ([]store.AuditRecord, error) {
	return nil, e.err
}
func (e errorStore) PutAnchor(ctx context.Context, a store.Anchor) (store.Anchor, error) {
	return store.Anchor{}, e.err
}
func (e errorStore) GetAnchor(ctx context.Context, v string) (store.Anchor, error) {
	return store.Anchor{}, e.err
}
func (e errorStore) PutEvidence(ctx context.Context, e2 store.EvidenceObject) (store.EvidenceObject, error) {
	return store.EvidenceObject{}, e.err
}
func (e errorStore) GetEvidence(ctx context.Context, id string) (store.EvidenceObject, error) {
	return store.EvidenceObject{}, e.err
}
func (e errorStore) EnqueueJobs(ctx context.Context, v string, b []string) ([]store.BuildJob, error) {
	return nil, e.err
}
func (e errorStore) ListJobs(ctx context.Context, v string) ([]store.BuildJob, error) {
	return nil, e.err
}
func (e errorStore) GetJob(ctx context.Context, id string) (store.BuildJob, error) {
	return store.BuildJob{}, e.err
}
func (e errorStore) ClaimJob(ctx context.Context, o string, l time.Duration) (store.BuildJob, bool, error) {
	return store.BuildJob{}, false, e.err
}
func (e errorStore) CompleteJob(ctx context.Context, id string, ok bool, d, c, ec, ed, attestation string, signatureValid bool) (store.BuildJob, error) {
	return store.BuildJob{}, e.err
}

func TestStoreFailureIs500Redacted(t *testing.T) {
	fs, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(errorStore{err: fmt.Errorf("db is down")}, fs)
	routes := []struct{ method, path, body string }{
		{"GET", "/api/v1/ready", ""},
		{"GET", "/api/v1/releases", ""},
		{"GET", "/api/v1/releases/rel_x", ""},
		{"POST", "/api/v1/releases", `{"repo":"https://github.com/example/t","commit":"c"}`},
		{"POST", "/api/v1/verifications", `{"releaseId":"rel_x"}`},
		{"GET", "/api/v1/verifications/ver_x", ""},
		{"POST", "/api/v1/verifications/ver_x/reverify", ""},
		{"POST", "/api/v1/verifications/ver_x/jobs", `{"builderIds":["b"]}`},
		{"GET", "/api/v1/verifications/ver_x/jobs", ""},
		{"GET", "/api/v1/jobs/job_x", ""},
		{"POST", "/api/v1/jobs/claim", `{"owner":"w","leaseSeconds":5}`},
		{"POST", "/api/v1/jobs/job_x/complete", `{"ok":true}`},
		{"GET", "/api/v1/builders", ""},
		{"POST", "/api/v1/builders", `{"id":"b","independenceGroup":"g"}`},
		{"PATCH", "/api/v1/builders/b", `{"enabled":true}`},
		{"GET", "/api/v1/policies", ""},
		{"POST", "/api/v1/policies", testPolicy},
		{"GET", "/api/v1/audit", ""},
		{"GET", "/api/v1/audit/1", ""},
		{"POST", "/api/v1/audit/1/verify-chain", ""},
		{"GET", "/api/v1/evidence/ev_x", ""},
		{"GET", "/api/v1/evidence/ev_x/blob", ""},
		{"GET", "/api/v1/blockchain/0xv", ""},
	}
	for _, rt := range routes {
		code, env := do(t, srv, rt.method, rt.path, rt.body, nil)
		if code != 500 || env.Error.Code != "INTERNAL_ERROR" || env.Error.Message != "internal error" {
			t.Fatalf("%s %s: want redacted 500, got %d %+v", rt.method, rt.path, code, env.Error)
		}
		if env.RequestID == "" {
			t.Fatalf("%s %s: 500s must still carry requestId", rt.method, rt.path)
		}
	}
	_ = policy.DecisionVerified // keep policy import if unused in future edits
}
