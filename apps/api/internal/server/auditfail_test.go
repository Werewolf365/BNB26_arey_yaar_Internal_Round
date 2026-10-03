package server_test

import (
	"context"
	"testing"

	"github.com/quorum/quorum/apps/api/internal/server"
	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/services/storage"
)

// failStore delegates to memory but fails selected methods on demand:
// exercises handler store-failure branches (partial-failure honesty)
// without a DB.
type failStore struct {
	store.Store
	fail map[string]bool
}

func (f *failStore) AppendAudit(ctx context.Context, ev string, p map[string]any, v string) (store.AuditRecord, error) {
	if f.fail["AppendAudit"] {
		return store.AuditRecord{}, context.DeadlineExceeded // arbitrary operational error
	}
	return f.Store.AppendAudit(ctx, ev, p, v)
}

func (f *failStore) CreateVerification(ctx context.Context, v store.Verification, k string) (store.Verification, bool, error) {
	if f.fail["CreateVerification"] {
		return store.Verification{}, false, context.DeadlineExceeded
	}
	return f.Store.CreateVerification(ctx, v, k)
}

func (f *failStore) ListBuilders(ctx context.Context) ([]store.Builder, error) {
	if f.fail["ListBuilders"] {
		return nil, context.DeadlineExceeded
	}
	return f.Store.ListBuilders(ctx)
}

func (f *failStore) ListJobs(ctx context.Context, v string) ([]store.BuildJob, error) {
	if f.fail["ListJobs"] {
		return nil, context.DeadlineExceeded
	}
	return f.Store.ListJobs(ctx, v)
}

func (f *failStore) GetVerification(ctx context.Context, id string) (store.Verification, error) {
	if f.fail["GetVerification"] {
		return store.Verification{}, context.DeadlineExceeded
	}
	return f.Store.GetVerification(ctx, id)
}

func (f *failStore) GetRelease(ctx context.Context, id string) (store.Release, error) {
	if f.fail["GetRelease"] {
		return store.Release{}, context.DeadlineExceeded
	}
	return f.Store.GetRelease(ctx, id)
}

func (f *failStore) PutAnchor(ctx context.Context, a store.Anchor) (store.Anchor, error) {
	if f.fail["PutAnchor"] {
		return store.Anchor{}, context.DeadlineExceeded
	}
	return f.Store.PutAnchor(ctx, a)
}

func TestAuditFailureIs500(t *testing.T) {
	fs, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st := &failStore{Store: store.NewMemoryStore(), fail: map[string]bool{}}
	srv := server.New(st, fs)

	// Seed with audit working.
	code, env := do(t, srv, "POST", "/api/v1/releases", `{"repo":"https://github.com/example/t","commit":"c"}`, nil)
	if code != 201 {
		t.Fatalf("seed release: %d %v", code, env.Error)
	}
	rel := mustData[map[string]any](t, env)["id"].(string)
	code, env = do(t, srv, "POST", "/api/v1/verifications", `{"releaseId":"`+rel+`"}`, nil)
	if code != 201 {
		t.Fatalf("seed verification: %d %v", code, env.Error)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)
	code, _ = do(t, srv, "POST", "/api/v1/builders", `{"id":"ab","independenceGroup":"g"}`, nil)
	if code != 201 {
		t.Fatalf("seed builder: %d", code)
	}

	// Flip audit to failing: mutating routes must 500, never half-claim success.
	st.fail["AppendAudit"] = true
	for _, tc := range []struct{ method, path, body string }{{"POST", "/api/v1/releases", `{"repo":"https://github.com/example/t","commit":"c"}`},
		{"POST", "/api/v1/verifications", `{"releaseId":"` + rel + `"}`},
		{"POST", "/api/v1/verifications/" + vid + "/reverify", ""},
		{"POST", "/api/v1/builders", `{"id":"ab2","independenceGroup":"g"}`},
		{"POST", "/api/v1/policies", testPolicy},
		{"POST", "/api/v1/verifications/" + vid + "/jobs", `{"builderIds":["ab"]}`},
		{"POST", "/api/v1/evidence", `{"verificationId":"` + vid + `","kind":"log","data":"eA=="}`},
	} {
		code, env := do(t, srv, tc.method, tc.path, tc.body, nil)
		if code != 500 || env.Error.Code != "INTERNAL_ERROR" {
			t.Fatalf("%s %s: audit failure must be 500, got %d %+v", tc.method, tc.path, code, env.Error)
		}
	}
}

func TestStoreFailuresPerMethod(t *testing.T) {
	fs, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st := &failStore{Store: store.NewMemoryStore(), fail: map[string]bool{}}
	srv := server.New(st, fs)
	code, env := do(t, srv, "POST", "/api/v1/releases", `{"repo":"https://github.com/example/t","commit":"c"}`, nil)
	if code != 201 {
		t.Fatalf("seed release: %d", code)
	}
	rel := mustData[map[string]any](t, env)["id"].(string)
	code, env = do(t, srv, "POST", "/api/v1/verifications", `{"releaseId":"`+rel+`"}`, nil)
	if code != 201 {
		t.Fatalf("seed verification: %d", code)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)
	if code, _ := do(t, srv, "POST", "/api/v1/builders", `{"id":"wb","independenceGroup":"g"}`, nil); code != 201 {
		t.Fatalf("seed builder: %d", code)
	}

	// Verification create failure (release resolves, create fails).
	st.fail["CreateVerification"] = true
	if code, _ := do(t, srv, "POST", "/api/v1/verifications", `{"releaseId":"`+rel+`"}`, nil); code != 500 {
		t.Fatalf("create failure must be 500, got %d", code)
	}
	st.fail["CreateVerification"] = false

	// Reverify release lookup failure (memory enforces the FK, so force it).
	st.fail["GetRelease"] = true
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/reverify", "", nil); code != 500 {
		t.Fatalf("reverify release failure must be 500, got %d", code)
	}
	st.fail["GetRelease"] = false
	// Reverify create failure.
	st.fail["CreateVerification"] = true
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/reverify", "", nil); code != 500 {
		t.Fatalf("reverify create failure must be 500, got %d", code)
	}
	st.fail["CreateVerification"] = false

	// Job-queue store failures: the auto-eval branches only run when every
	// sibling is terminal, and each completion consumes its job (enqueue
	// dedups per builder), so every stage gets a fresh builder+job.
	for _, b := range []string{"s1", "s2", "s3", "s4", "s5", "s6"} {
		if code, _ := do(t, srv, "POST", "/api/v1/builders", `{"id":"`+b+`","independenceGroup":"g"}`, nil); code != 201 {
			t.Fatalf("seed %s: %d", b, code)
		}
	}
	runStage := func(builder, fail string) (int, envelope) {
		st.fail = map[string]bool{}
		code, env := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":["`+builder+`"]}`, nil)
		if code != 201 {
			t.Fatalf("enqueue %s: %d", builder, code)
		}
		code, env = do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"w","leaseSeconds":60}`, nil)
		if code != 200 {
			t.Fatalf("claim %s: %d", builder, code)
		}
		if claimed, _ := mustData[map[string]any](t, env)["claimed"].(bool); !claimed {
			t.Fatalf("nothing to claim for %s", builder)
		}
		code, env = do(t, srv, "GET", "/api/v1/verifications/"+vid+"/jobs", "", nil)
		jobs := mustData[[]map[string]any](t, env)
		var jid string
		for _, j := range jobs {
			if j["builderId"].(string) == builder {
				jid = j["id"].(string)
			}
		}
		if jid == "" {
			t.Fatalf("no job for %s", builder)
		}
		st.fail[fail] = true
		defer func() { st.fail[fail] = false }()
		return do(t, srv, "POST", "/api/v1/jobs/"+jid+"/complete", `{"ok":true,"digest":"sha256:abc","commit":"c"}`, nil)
	}
	// Enqueue-time builder lookup failure needs no job at all.
	st.fail["ListBuilders"] = true
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":["wb"]}`, nil); code != 500 {
		t.Fatalf("builder lookup failure must be 500, got %d", code)
	}
	st.fail["ListBuilders"] = false
	if code, _ := do(t, srv, "POST", "/api/v1/builders", `{"id":"wb","independenceGroup":"g"}`, nil); code != 201 {
		t.Fatalf("seed builder: %d", code)
	}
	stages := []struct{ builder, fail string }{
		{"s1", "ListJobs"},
		{"s2", "GetVerification"},
		{"s3", "GetRelease"},
		{"s4", "ListBuilders"},
		{"s5", "CreateVerification"},
		{"s6", "AppendAudit"},
	}
	for _, sg := range stages {
		code, env := runStage(sg.builder, sg.fail)
		if code != 500 || env.Error.Code != "INTERNAL_ERROR" {
			t.Fatalf("complete with %s down must be 500, got %d %+v", sg.fail, code, env.Error)
		}
	}
}
