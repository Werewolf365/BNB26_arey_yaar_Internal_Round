package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/quorum/quorum/apps/api/internal/server"
	"github.com/quorum/quorum/apps/api/internal/store"
)

type envelope struct {
	Data      json.RawMessage `json:"data"`
	Error     *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"requestId"`
}

func newTestServer() *server.Server { return server.New(store.NewMemoryStore()) }

// do sends a request; body may be a JSON string or nil.
func do(t *testing.T, srv *server.Server, method, path, body string, headers map[string]string) (int, envelope) {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("%s %s: bad envelope: %v (%s)", method, path, err, rec.Body.String())
	}
	if env.RequestID == "" {
		t.Fatalf("%s %s: missing requestId", method, path)
	}
	return rec.Code, env
}

func mustData[T any](t *testing.T, env envelope) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(env.Data, &v); err != nil {
		t.Fatalf("bad data: %v", err)
	}
	return v
}

func TestHealthReady(t *testing.T) {
	srv := newTestServer()
	if code, _ := do(t, srv, "GET", "/api/v1/health", "", nil); code != 200 {
		t.Fatalf("health: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/ready", "", nil); code != 200 {
		t.Fatalf("ready with memory store: %d", code)
	}
}

func createRelease(t *testing.T, srv *server.Server, key string) string {
	t.Helper()
	h := map[string]string{}
	if key != "" {
		h["Idempotency-Key"] = key
	}
	code, env := do(t, srv, "POST", "/api/v1/releases",
		`{"package":"quorum-tiny","ecosystem":"pypi","name":"quorum-tiny","version":"1.0.0","repo":"https://github.com/example/tiny","commit":"abc123","expectedDigest":"sha256:abc"}`, h)
	if code != 201 && code != 200 {
		t.Fatalf("create release: %d %v", code, env.Error)
	}
	var rel struct {
		ID string `json:"id"`
	}
	rel = mustData[struct {
		ID string `json:"id"`
	}](t, env)
	if rel.ID == "" {
		t.Fatal("missing release id")
	}
	return rel.ID
}

func TestReleases(t *testing.T) {
	srv := newTestServer()
	id := createRelease(t, srv, "rel-key-1")
	// Idempotent replay returns the same record with 200.
	code, env := do(t, srv, "POST", "/api/v1/releases",
		`{"repo":"https://github.com/example/tiny","commit":"abc123"}`, map[string]string{"Idempotency-Key": "rel-key-1"})
	if code != 200 {
		t.Fatalf("replay: %d", code)
	}
	if got := mustData[map[string]any](t, env)["id"]; got != id {
		t.Fatalf("replay must return same id: %v vs %s", got, id)
	}
	// List + get.
	if code, env := do(t, srv, "GET", "/api/v1/releases?limit=10", "", nil); code != 200 || !strings.Contains(string(env.Data), id) {
		t.Fatalf("list: %d", code)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/releases/"+id, "", nil); code != 200 {
		t.Fatalf("get: %d", code)
	}
	// 404 with typed code.
	if code, env := do(t, srv, "GET", "/api/v1/releases/nope", "", nil); code != 404 || env.Error.Code != "RELEASE_NOT_FOUND" {
		t.Fatalf("missing release: %d %v", code, env.Error)
	}
	// Invalid repo host.
	if code, env := do(t, srv, "POST", "/api/v1/releases", `{"repo":"https://evil.example/x","commit":"c"}`, nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("evil repo: %d %v", code, env.Error)
	}
	// Malformed JSON.
	if code, env := do(t, srv, "POST", "/api/v1/releases", `{oops`, nil); code != 400 {
		t.Fatalf("malformed: %d %v", code, env.Error)
	}
	// Missing fields.
	if code, _ := do(t, srv, "POST", "/api/v1/releases", `{"repo":"https://github.com/e/t"}`, nil); code != 400 {
		t.Fatalf("missing commit: %d", code)
	}
}

func agreeEvidence() string {
	d := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return fmt.Sprintf(`[
		{"builderId":"builder-a","independenceGroup":"cloud-a","sourceCommit":"abc123","artifactDigest":"%s","signatureValid":true,"verificationSource":"builder"},
		{"builderId":"builder-b","independenceGroup":"cloud-b","sourceCommit":"abc123","artifactDigest":"%s","signatureValid":true,"verificationSource":"builder"}
	]`, d, d)
}

func TestVerifications(t *testing.T) {
	srv := newTestServer()
	rel := createRelease(t, srv, "")
	body := fmt.Sprintf(`{"releaseId":%q,"evidence":%s}`, rel, agreeEvidence())
	code, env := do(t, srv, "POST", "/api/v1/verifications", body, map[string]string{"Idempotency-Key": "ver-key-1"})
	if code != 201 {
		t.Fatalf("create verification: %d %v", code, env.Error)
	}
	ver := mustData[map[string]any](t, env)
	if ver["decision"] != "VERIFIED" {
		t.Fatalf("want VERIFIED: %v", ver)
	}
	vid := ver["id"].(string)
	// Idempotent replay.
	if code, env := do(t, srv, "POST", "/api/v1/verifications", body, map[string]string{"Idempotency-Key": "ver-key-1"}); code != 200 || mustData[map[string]any](t, env)["id"] != vid {
		t.Fatalf("replay: %d", code)
	}
	// Get + reverify (new row, same decision).
	if code, _ := do(t, srv, "GET", "/api/v1/verifications/"+vid, "", nil); code != 200 {
		t.Fatalf("get: %d", code)
	}
	if code, env := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/reverify", "", nil); code != 201 || mustData[map[string]any](t, env)["id"] == vid {
		t.Fatalf("reverify: %d", code)
	}
	// Unknown release.
	if code, env := do(t, srv, "POST", "/api/v1/verifications", fmt.Sprintf(`{"releaseId":"rel_nope","evidence":%s}`, agreeEvidence()), nil); code != 404 {
		t.Fatalf("unknown release: %d %v", code, env.Error)
	}
	// No evidence -> 422 typed.
	if code, env := do(t, srv, "POST", "/api/v1/verifications", fmt.Sprintf(`{"releaseId":%q}`, rel), nil); code != 422 || env.Error.Code != "INSUFFICIENT_EVIDENCE" {
		t.Fatalf("empty evidence: %d %v", code, env.Error)
	}
	// Conflicting evidence -> recorded with visible minority (never hidden).
	d := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	confBody := fmt.Sprintf(`{"releaseId":%q,"evidence":[
		{"builderId":"builder-a","independenceGroup":"cloud-a","sourceCommit":"abc123","artifactDigest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","signatureValid":true,"verificationSource":"builder"},
		{"builderId":"builder-c","independenceGroup":"cloud-c","sourceCommit":"abc123","artifactDigest":"%s","signatureValid":true,"verificationSource":"builder"}]}`, rel, d)
	if code, env := do(t, srv, "POST", "/api/v1/verifications", confBody, nil); code != 201 {
		t.Fatalf("conflict record: %d %v", code, env.Error)
	} else if mustData[map[string]any](t, env)["decision"] != "INVESTIGATE" {
		t.Fatalf("1v1 split must investigate: %v", mustData[map[string]any](t, env))
	}
	// Reverify unknown id.
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/ver_nope/reverify", "", nil); code != 404 {
		t.Fatalf("reverify missing: %d", code)
	}
}

func TestConcurrentIdempotentReleases(t *testing.T) {
	srv := newTestServer()
	const n = 10
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, env := do(t, srv, "POST", "/api/v1/releases", `{"repo":"https://github.com/example/tiny","commit":"abc123"}`,
				map[string]string{"Idempotency-Key": "race-key"})
			ids[i] = mustData[map[string]any](t, env)["id"].(string)
		}(i)
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if ids[i] != ids[0] {
			t.Fatalf("concurrent duplicates must converge: %q vs %q", ids[i], ids[0])
		}
	}
}

func TestBuildersPolicies(t *testing.T) {
	srv := newTestServer()
	// Register + list + disable.
	if code, _ := do(t, srv, "POST", "/api/v1/builders", `{"id":"builder-a","independenceGroup":"cloud-a"}`, nil); code != 201 {
		t.Fatalf("register: %d", code)
	}
	if code, env := do(t, srv, "POST", "/api/v1/builders", `{"id":"builder-b"}`, nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("missing group: %d %v", code, env.Error)
	}
	if code, env := do(t, srv, "GET", "/api/v1/builders", "", nil); code != 200 || !strings.Contains(string(env.Data), "cloud-a") {
		t.Fatalf("list: %d", code)
	}
	if code, env := do(t, srv, "PATCH", "/api/v1/builders/builder-a", `{"enabled":false}`, nil); code != 200 || strings.Contains(string(env.Data), `"enabled":true`) {
		t.Fatalf("disable: %d %s", code, env.Data)
	}
	if code, _ := do(t, srv, "PATCH", "/api/v1/builders/nope", `{"enabled":false}`, nil); code != 404 {
		t.Fatalf("patch missing: %d", code)
	}
	// Policies.
	pol := `{"policyId":"policy-a","version":"v1","minBuilders":2,"requiredAgreement":2,"requiredIndependentGroups":2,"requireSourceMatch":true,"requireAllSignaturesValid":true,"conflictTolerance":1,"allowedVerificationSources":["builder"]}`
	if code, env := do(t, srv, "POST", "/api/v1/policies", pol, nil); code != 201 || !strings.Contains(string(env.Data), "sha256:") {
		t.Fatalf("create policy: %d %s", code, env.Data)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/policies", pol, nil); code != 409 {
		t.Fatalf("duplicate policy: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/policies", `{"policyId":"bad","minBuilders":0}`, nil); code != 400 {
		t.Fatalf("invalid policy: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/policies/validate", pol, nil); code != 200 {
		t.Fatalf("validate: %d", code)
	}
	if code, env := do(t, srv, "GET", "/api/v1/policies", "", nil); code != 200 || !strings.Contains(string(env.Data), "policy-a") {
		t.Fatalf("list policies: %d", code)
	}
}

func TestAuditAndEvidence(t *testing.T) {
	srv := newTestServer()
	rel := createRelease(t, srv, "")
	do(t, srv, "POST", "/api/v1/verifications", fmt.Sprintf(`{"releaseId":%q,"evidence":%s}`, rel, agreeEvidence()), nil)
	// Audit trail exists and verifies.
	if code, env := do(t, srv, "GET", "/api/v1/audit", "", nil); code != 200 || !strings.Contains(string(env.Data), "release.created") {
		t.Fatalf("audit list: %d %s", code, env.Data)
	}
	if code, env := do(t, srv, "POST", "/api/v1/audit/1/verify-chain", "", nil); code != 200 || !strings.Contains(string(env.Data), `"ok":true`) {
		t.Fatalf("verify-chain: %d %s", code, env.Data)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/audit/999999", "", nil); code != 404 {
		t.Fatalf("audit get missing: %d", code)
	}
	// Evidence: unknown id typed 404.
	if code, env := do(t, srv, "GET", "/api/v1/evidence/ev_nope", "", nil); code != 404 || env.Error.Code != "EVIDENCE_NOT_FOUND" {
		t.Fatalf("evidence 404: %d %v", code, env.Error)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/attestations/ev_nope", "", nil); code != 404 {
		t.Fatalf("attestation 404: %d", code)
	}
	// Blockchain: unreachable + not required -> 200 SKIPPED; missing anchor -> 404.
	anchorBody := `{"contract":"0x5FbDB2315678afecb367f032d93F642f64180aa3","verificationId":"0x1111111111111111111111111111111111111111111111111111111111111111","evidenceHash":"0x2222222222222222222222222222222222222222222222222222222222222222","artifactDigest":"0x3333333333333333333333333333333333333333333333333333333333333333","sourceCommit":"0x4444444444444444444444444444444444444444444444444444444444444444","policyHash":"0x5555555555555555555555555555555555555555555555555555555555555555","decision":"VERIFIED","rpc":"http://127.0.0.1:9"}`
	if code, env := do(t, srv, "POST", "/api/v1/blockchain/anchor", anchorBody, nil); code != 200 || !strings.Contains(string(env.Data), `"skipped":true`) {
		t.Fatalf("anchor skip: %d %s", code, env.Data)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/blockchain/0xabc", "", nil); code != 404 {
		t.Fatalf("anchor get missing: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/blockchain/anchor", `{"contract":"0x123"}`, nil); code != 400 && code != 500 {
		t.Fatalf("anchor invalid: %d", code)
	}
}
