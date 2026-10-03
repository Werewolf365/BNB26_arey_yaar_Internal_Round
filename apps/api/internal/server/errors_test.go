package server_test

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quorum/quorum/apps/api/internal/server"
	"github.com/quorum/quorum/apps/api/internal/store"
	"github.com/quorum/quorum/services/storage"
	"golang.org/x/crypto/sha3"
)

// badJSON hits every JSON-decoding route with garbage to lock the 400 path.
func TestHandlerBadJSON(t *testing.T) {
	srv := newTestServer(t)
	posts := []string{
		"/api/v1/releases", "/api/v1/verifications",
		"/api/v1/jobs/claim", "/api/v1/jobs/x/complete",
		"/api/v1/evidence", "/api/v1/builders",
		"/api/v1/policies", "/api/v1/policies/validate",
		"/api/v1/blockchain/anchor",
	}
	for _, p := range posts {
		if code, env := do(t, srv, "POST", p, "{oops", nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
			t.Fatalf("POST %s garbage must be 400 INVALID_INPUT: %d %+v", p, code, env.Error)
		}
	}
	if code, env := do(t, srv, "PATCH", "/api/v1/builders/x", "{oops", nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("PATCH garbage must be 400: %d %+v", code, env.Error)
	}
}

func TestHandlerNotFoundAndValidation(t *testing.T) {
	srv := newTestServer(t)
	cases := []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/api/v1/releases/nope", "", 404},
		{"GET", "/api/v1/verifications/nope", "", 404},
		{"POST", "/api/v1/verifications/nope/reverify", "", 404},
		{"POST", "/api/v1/verifications", `{"releaseId":"rel_nope"}`, 404},
		{"POST", "/api/v1/verifications", `{"releaseId":""}`, 400},
		{"POST", "/api/v1/releases", `{"repo":"","commit":""}`, 400},
		{"POST", "/api/v1/releases", `{"repo":"https://evil.example/x","commit":"c"}`, 400},
		{"POST", "/api/v1/verifications/x/jobs", `{"builderIds":[]}`, 404}, // verification lookup first
		{"POST", "/api/v1/jobs/claim", `{"owner":""}`, 400},
		{"POST", "/api/v1/jobs/job_nope/complete", `{"ok":true}`, 404},
		{"GET", "/api/v1/jobs/job_nope", "", 404},
		{"POST", "/api/v1/evidence", `{"verificationId":"ver_nope","kind":"log","data":"eA=="}`, 404},
		{"POST", "/api/v1/evidence", `{"verificationId":"","kind":"","data":""}`, 400},
		{"PATCH", "/api/v1/builders/nope", `{"enabled":true}`, 404},
		{"POST", "/api/v1/policies", `{"policyId":"p","minBuilders":0}`, 400},
		{"POST", "/api/v1/policies/validate", `{"policyId":"p","minBuilders":0}`, 400},
		{"GET", "/api/v1/audit/999999", "", 404},
		{"GET", "/api/v1/blockchain/0xabc", "", 404},
		{"POST", "/api/v1/blockchain/anchor", `{"contract":"0x5FbDB2315678afecb367f032d93F642f64180aa3","verificationId":"0x` + strings.Repeat("1", 64) + `","evidenceHash":"0x` + strings.Repeat("2", 64) + `","artifactDigest":"0x` + strings.Repeat("3", 64) + `","sourceCommit":"0x` + strings.Repeat("4", 64) + `","policyHash":"0x` + strings.Repeat("5", 64) + `","decision":"BOGUS"}`, 400},
	}
	for _, tc := range cases {
		if code, _ := do(t, srv, tc.method, tc.path, tc.body, nil); code != tc.code {
			t.Fatalf("%s %s: want %d, got %d", tc.method, tc.path, tc.code, code)
		}
	}
	// Invalid inline policy is validated after the release resolves.
	rel := createRelease(t, srv, "rel-key-err")
	if code, env := do(t, srv, "POST", "/api/v1/verifications",
		`{"releaseId":"`+rel+`","policy":{"policyId":"p","minBuilders":0}}`, nil); code != 400 || env.Error.Code != "INVALID_POLICY" {
		t.Fatalf("invalid policy must be 400 INVALID_POLICY: %d %+v", code, env.Error)
	}
	// Garbage JSON on an existing verification exercises the decode path
	// where one exists (reverify takes no body, so it has no decode path).
	code, env := do(t, srv, "POST", "/api/v1/verifications",
		`{"releaseId":"`+rel+`"}`, nil)
	if code != 201 {
		t.Fatalf("seed verification: %d %v", code, env.Error)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/reverify", "", nil); code != 201 {
		t.Fatalf("reverify must succeed: %d", code)
	}
	// Lookup-before-decode routes: seed first, then the malformed input.
	if code, env := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", "{oops", nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("jobs garbage must be 400: %d %+v", code, env.Error)
	}
	if code, env := do(t, srv, "POST", "/api/v1/evidence",
		`{"verificationId":"`+vid+`","kind":"log","data":"!!!"}`, nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("bad base64 must be 400: %d %+v", code, env.Error)
	}
	if code, env := do(t, srv, "POST", "/api/v1/evidence",
		`{"verificationId":"`+vid+`","kind":"log","sha256":"`+strings.Repeat("0", 64)+`","data":"eA=="}`, nil); code != 400 || env.Error.Code != "INVALID_INPUT" {
		t.Fatalf("sha mismatch must be 400: %d %+v", code, env.Error)
	}
}

func TestEnqueueUnknownBuilder(t *testing.T) {
	srv := newTestServer(t)
	rel := createRelease(t, srv, "rel-key-jobs")
	code, env := do(t, srv, "POST", "/api/v1/verifications",
		`{"releaseId":"`+rel+`"}`, nil)
	if code != 201 {
		t.Fatalf("seed verification: %d %v", code, env.Error)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)
	code, env = do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs",
		`{"builderIds":["ghost-builder"]}`, nil)
	if code != 404 || env.Error.Code != "BUILDER_NOT_FOUND" {
		t.Fatalf("unknown builder must be 404 BUILDER_NOT_FOUND: %d %+v", code, env.Error)
	}
}

func TestStatusCodeMapping(t *testing.T) {
	srv := newTestServer(t)
	// Required anchor against a dead RPC -> 502 BLOCKCHAIN_UNAVAILABLE.
	anchor := `{"contract":"0x5FbDB2315678afecb367f032d93F642f64180aa3","verificationId":"0x` + strings.Repeat("1", 64) + `","evidenceHash":"0x` + strings.Repeat("2", 64) + `","artifactDigest":"0x` + strings.Repeat("3", 64) + `","sourceCommit":"0x` + strings.Repeat("4", 64) + `","policyHash":"0x` + strings.Repeat("5", 64) + `","decision":"VERIFIED","rpc":"http://127.0.0.1:9","required":true}`
	if code, env := do(t, srv, "POST", "/api/v1/blockchain/anchor", anchor, nil); code != 502 || env.Error.Code != "BLOCKCHAIN_UNAVAILABLE" {
		t.Fatalf("dead required anchor must be 502: %d %+v", code, env.Error)
	}
	// Completing a terminal job twice -> 409 JOB_CONFLICT.
	rel := createRelease(t, srv, "rel-key-conflict")
	code, env := do(t, srv, "POST", "/api/v1/verifications", `{"releaseId":"`+rel+`"}`, nil)
	if code != 201 {
		t.Fatalf("seed: %d", code)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)
	if code, _ := do(t, srv, "POST", "/api/v1/builders", `{"id":"wb","independenceGroup":"g"}`, nil); code != 201 {
		t.Fatalf("register: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":["wb"]}`, nil); code != 201 {
		t.Fatalf("enqueue: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"w","leaseSeconds":60}`, nil); code != 200 {
		t.Fatalf("claim: %d", code)
	}
	code, env = do(t, srv, "GET", "/api/v1/verifications/"+vid+"/jobs", "", nil)
	if code != 200 {
		t.Fatalf("list jobs: %d", code)
	}
	jobs := mustData[[]map[string]any](t, env)
	jid := jobs[0]["id"].(string)
	complete := "/api/v1/jobs/" + jid + "/complete"
	if code, _ := do(t, srv, "POST", complete, `{"ok":true,"digest":"sha256:abc","commit":"c"}`, nil); code != 200 {
		t.Fatalf("complete: %d", code)
	}
	if code, env := do(t, srv, "POST", complete, `{"ok":true,"digest":"sha256:abc","commit":"c"}`, nil); code != 409 || env.Error.Code != "JOB_CONFLICT" {
		t.Fatalf("re-complete must be 409 JOB_CONFLICT: %d %+v", code, env.Error)
	}
}

func TestInternalErrorMapping(t *testing.T) {
	// Nil blob backend forces INTERNAL_ERROR -> 500 with a redacted message
	// (the operational detail goes to logs, never to the client).
	srv := server.New(store.NewMemoryStore(), nil)
	rel := createRelease(t, srv, "rel-key-500")
	code, env := do(t, srv, "POST", "/api/v1/verifications",
		`{"releaseId":"`+rel+`"}`, nil)
	if code != 201 {
		t.Fatalf("seed verification: %d %v", code, env.Error)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)
	code, env = do(t, srv, "POST", "/api/v1/evidence",
		`{"verificationId":"`+vid+`","kind":"log","data":"eA=="}`, nil)
	if code != 500 || env.Error.Code != "INTERNAL_ERROR" || env.Error.Message != "internal error" {
		t.Fatalf("nil backend must be 500 redacted: %d %+v", code, env.Error)
	}
}

func TestAnchorAndEvidenceIntegrity(t *testing.T) {
	dir := t.TempDir()
	fs, err := storage.NewFilesystem(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(store.NewMemoryStore(), fs)
	rel := createRelease(t, srv, "rel-key-chain")
	code, env := do(t, srv, "POST", "/api/v1/verifications", `{"releaseId":"`+rel+`"}`, nil)
	if code != 201 {
		t.Fatalf("seed: %d", code)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)

	// Unknown policy id resolves to 404.
	if code, env := do(t, srv, "POST", "/api/v1/verifications",
		`{"releaseId":"`+rel+`","policyId":"pol_nope"}`, nil); code != 404 {
		t.Fatalf("unknown policy must be 404, got %d %+v", code, env.Error)
	}

	// Submit evidence, corrupt it on disk, re-read must trip tamper evidence.
	code, env = do(t, srv, "POST", "/api/v1/evidence",
		`{"verificationId":"`+vid+`","kind":"log","data":"`+base64.StdEncoding.EncodeToString([]byte("real-bytes"))+`"}`, nil)
	if code != 201 {
		t.Fatalf("submit: %d %v", code, env.Error)
	}
	eo := mustData[map[string]any](t, env)
	key, _ := eo["storageKey"].(string)
	hexpart := strings.TrimPrefix(key, "sha256/")
	blobPath := filepath.Join(dir, "sha256", hexpart[:2], hexpart[2:])
	if err := os.WriteFile(blobPath, []byte("tampered!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	eid, _ := eo["id"].(string)
	if code, env := do(t, srv, "GET", "/api/v1/evidence/"+eid+"/blob", "", nil); code != 422 || env.Error.Code != "AUDIT_TAMPERED" {
		t.Fatalf("corrupt blob must be 422 AUDIT_TAMPERED: %d %+v", code, env.Error)
	}

	// Successful anchor against a fake chain records the receipt.
	h := sha3.NewLegacyKeccak256()
	h.Write([]byte("VerificationAnchored(bytes32,bytes32,bytes32,bytes32,bytes32,uint8,uint256)"))
	eventSig := "0x" + hex.EncodeToString(h.Sum(nil))
	contract := "0x5FbDB2315678afecb367f032d93F642f64180aa3"
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		reply := func(v any) {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": v})
		}
		switch req.Method {
		case "eth_chainId":
			reply("0x7a69")
		case "eth_sendTransaction":
			reply("0xtx1")
		default:
			reply(map[string]any{"status": "0x1", "blockNumber": "0x9", "logs": []any{
				map[string]any{"address": contract, "topics": []string{eventSig}},
			}})
		}
	}))
	defer fake.Close()
	d := "0x" + strings.Repeat("7", 64)
	anchorBody := `{"contract":"` + contract + `","verificationId":"` + d + `","evidenceHash":"` + d + `","artifactDigest":"` + d + `","sourceCommit":"` + d + `","policyHash":"` + d + `","decision":"VERIFIED","rpc":"` + fake.URL + `"}`
	if code, env := do(t, srv, "POST", "/api/v1/blockchain/anchor", anchorBody, nil); code != 201 {
		t.Fatalf("fake anchor must be 201, got %d %+v", code, env.Error)
	}
	if code, env := do(t, srv, "GET", "/api/v1/blockchain/"+d, "", nil); code != 404 {
		t.Fatalf("bytes32 anchor id has no verification row (best-effort link): %d %+v", code, env.Error)
	}
}

func TestReadAndPolicyPaths(t *testing.T) {
	st := store.NewMemoryStore()
	fs, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(st, fs)
	rel := createRelease(t, srv, "rel-key-reads")

	// Inline valid policy is stored on the verification.
	inline := `{"releaseId":"` + rel + `","policy":{"policyId":"custom","version":"v1","minBuilders":1,"requiredAgreement":1,"requiredIndependentGroups":1,"conflictTolerance":0}}`
	if code, _ := do(t, srv, "POST", "/api/v1/verifications", inline, nil); code != 201 {
		t.Fatalf("inline policy: %d", code)
	}
	// Stored policy resolves by id.
	if code, _ := do(t, srv, "POST", "/api/v1/policies", testPolicy, nil); code != 201 {
		t.Fatalf("seed policy: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/verifications", `{"releaseId":"`+rel+`","policyId":"p1"}`, nil); code != 201 {
		t.Fatalf("policy by id: %d", code)
	}
	// Evidence metadata read.
	code, env := do(t, srv, "POST", "/api/v1/verifications", `{"releaseId":"`+rel+`"}`, nil)
	vid := mustData[map[string]any](t, env)["id"].(string)
	code, env = do(t, srv, "POST", "/api/v1/evidence",
		`{"verificationId":"`+vid+`","kind":"log","data":"eA=="}`, nil)
	if code != 201 {
		t.Fatalf("submit: %d", code)
	}
	eid := mustData[map[string]any](t, env)["id"].(string)
	if code, _ := do(t, srv, "GET", "/api/v1/evidence/"+eid, "", nil); code != 200 {
		t.Fatalf("evidence metadata: %d", code)
	}
	// Audit record read (ids start at 1).
	if code, _ := do(t, srv, "GET", "/api/v1/audit/1", "", nil); code != 200 {
		t.Fatalf("audit get: %d", code)
	}
	// Anchor read after a direct store link (anchors reference verifications).
	if _, err := st.PutAnchor(context.Background(), store.Anchor{VerificationID: vid, TxHash: "0x1"}); err != nil {
		t.Fatal(err)
	}
	if code, _ := do(t, srv, "GET", "/api/v1/blockchain/"+vid, "", nil); code != 200 {
		t.Fatalf("anchor get: %d", code)
	}
	// Blob download with no backend configured.
	noblobs := server.New(store.NewMemoryStore(), nil)
	if code, env := do(t, noblobs, "GET", "/api/v1/evidence/ev_x/blob", "", nil); code != 404 {
		t.Fatalf("missing evidence is 404 before backend check: %d %+v", code, env.Error)
	}
}
