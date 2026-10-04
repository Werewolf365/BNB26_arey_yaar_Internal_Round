package server_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/quorum/quorum/services/sigstore"
)

func TestBuildJobs(t *testing.T) {
	srv := newTestServer(t)
	rel := createRelease(t, srv, "")
	do(t, srv, "POST", "/api/v1/builders", `{"id":"builder-a","independenceGroup":"cloud-a"}`, nil)
	do(t, srv, "POST", "/api/v1/builders", `{"id":"builder-b","independenceGroup":"cloud-b"}`, nil)
	// Create a verification to attach jobs to.
	code, env := do(t, srv, "POST", "/api/v1/verifications", fmt.Sprintf(`{"releaseId":%q,"evidence":%s}`, rel, agreeEvidence()), nil)
	if code != 201 {
		t.Fatalf("seed verification: %d", code)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)

	// Enqueue requires registered builders.
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":["ghost"]}`, nil); code != 404 {
		t.Fatalf("unknown builder: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":[]}`, nil); code != 400 {
		t.Fatalf("empty builders: %d", code)
	}
	code, env = do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":["builder-a","builder-b"]}`, nil)
	if code != 201 {
		t.Fatalf("enqueue: %d", code)
	}
	var jobs []map[string]any
	if err := json.Unmarshal(env.Data, &jobs); err != nil || len(jobs) != 2 {
		t.Fatalf("want 2 jobs: %s", env.Data)
	}
	// Duplicate enqueue converges (unique verification+builder).
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":["builder-a"]}`, nil); code != 201 {
		t.Fatalf("re-enqueue: %d", code)
	}
	if code, env := do(t, srv, "GET", "/api/v1/verifications/"+vid+"/jobs", "", nil); code != 200 || strings.Count(string(env.Data), "QUEUED") != 2 {
		t.Fatalf("list jobs: %d %s", code, env.Data)
	}

	// Claim as worker-1, complete with the honest digest -> evaluation pending.
	code, env = do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"worker-1"}`, nil)
	if code != 200 || !strings.Contains(string(env.Data), `"claimed":true`) {
		t.Fatalf("claim: %d %s", code, env.Data)
	}
	var claimed struct {
		Job map[string]any `json:"job"`
	}
	if err := json.Unmarshal(env.Data, &claimed); err != nil {
		t.Fatal(err)
	}
	jid := claimed.Job["id"].(string)
	d := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	code, env = do(t, srv, "POST", "/api/v1/jobs/"+jid+"/complete",
		fmt.Sprintf(`{"ok":true,"digest":%q,"commit":"abc123"}`, d), nil)
	if code != 200 || !strings.Contains(string(env.Data), `"evaluation":"pending"`) {
		t.Fatalf("first completion pends evaluation: %d %s", code, env.Data)
	}
	// Double completion of a non-terminal requeued job is allowed (retry path);
	// terminal duplicates are rejected. Complete the second job -> auto VERIFIED.
	code, env = do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"worker-2"}`, nil)
	if code != 200 || !strings.Contains(string(env.Data), `"claimed":true`) {
		t.Fatalf("second claim: %d %s", code, env.Data)
	}
	var claimed2 struct {
		Job map[string]any `json:"job"`
	}
	_ = json.Unmarshal(env.Data, &claimed2)
	jid2 := claimed2.Job["id"].(string)
	code, env = do(t, srv, "POST", "/api/v1/jobs/"+jid2+"/complete",
		fmt.Sprintf(`{"ok":true,"digest":%q,"commit":"abc123"}`, d), nil)
	if code != 200 || !strings.Contains(string(env.Data), `"evaluation"`) {
		t.Fatalf("second completion: %d %s", code, env.Data)
	}
	if !strings.Contains(string(env.Data), "INSUFFICIENT_EVIDENCE") {
		t.Fatalf("unsigned worker reports must not auto-verify: %s", env.Data)
	}
	// Queue drained.
	if code, env := do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"worker-1"}`, nil); code != 200 || !strings.Contains(string(env.Data), `"claimed":false`) {
		t.Fatalf("drained queue: %d %s", code, env.Data)
	}
	// Unknown job paths.
	if code, _ := do(t, srv, "GET", "/api/v1/jobs/job_nope", "", nil); code != 404 {
		t.Fatalf("get missing job: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/jobs/job_nope/complete", `{"ok":true}`, nil); code != 404 {
		t.Fatalf("complete missing job: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":""}`, nil); code != 400 {
		t.Fatalf("claim without owner: %d", code)
	}
}

// Explicit JSON null attestation behaves like an omitted field: the worker
// sends `"attestation":null` when run without --signing-key, and that must
// complete as unsigned evidence — never ATTESTATION_INVALID.
func TestCompleteNullAttestation(t *testing.T) {
	srv := newTestServer(t)
	rel := createRelease(t, srv, "")
	do(t, srv, "POST", "/api/v1/builders", `{"id":"builder-n","independenceGroup":"cloud-n"}`, nil)
	code, env := do(t, srv, "POST", "/api/v1/verifications", fmt.Sprintf(`{"releaseId":%q}`, rel), nil)
	if code != 201 {
		t.Fatalf("seed: %d", code)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":["builder-n"]}`, nil); code != 201 {
		t.Fatalf("enqueue: %d", code)
	}
	code, env = do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"w"}`, nil)
	var claimed struct {
		Job map[string]any `json:"job"`
	}
	_ = json.Unmarshal(env.Data, &claimed)
	jid := claimed.Job["id"].(string)
	d := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	body := fmt.Sprintf(`{"ok":true,"digest":%q,"commit":"abc123","attestation":null}`, d)
	if code, env := do(t, srv, "POST", "/api/v1/jobs/"+jid+"/complete", body, nil); code != 200 || !strings.Contains(string(env.Data), `"evaluation"`) {
		t.Fatalf("null attestation must complete unsigned: %d %s", code, env.Data)
	}
	if code, env := do(t, srv, "GET", "/api/v1/verifications/"+vid+"/jobs", "", nil); code != 200 {
		t.Fatalf("jobs readable: %d", code)
	} else if !strings.Contains(string(env.Data), `"status":"SUCCEEDED"`) {
		t.Fatalf("null-attestation job must succeed unsigned: %s", env.Data)
	}
}

func TestBuildJobsFailurePath(t *testing.T) {
	srv := newTestServer(t)
	rel := createRelease(t, srv, "")
	do(t, srv, "POST", "/api/v1/builders", `{"id":"builder-a","independenceGroup":"cloud-a"}`, nil)
	code, env := do(t, srv, "POST", "/api/v1/verifications", fmt.Sprintf(`{"releaseId":%q,"evidence":%s}`, rel, agreeEvidence()), nil)
	if code != 201 {
		t.Fatalf("seed: %d", code)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)
	do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":["builder-a"]}`, nil)
	code, env = do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"w1"}`, nil)
	var claimed struct {
		Job map[string]any `json:"job"`
	}
	_ = json.Unmarshal(env.Data, &claimed)
	jid := claimed.Job["id"].(string)
	// Fail 3 times (maxAttempts) -> FAILED terminal + insufficient evaluation.
	for i := 0; i < 3; i++ {
		code, _ = do(t, srv, "POST", "/api/v1/jobs/"+jid+"/complete", `{"ok":false,"errorCode":"BUILDER_FAILED","errorDetail":"boom"}`, nil)
		if code != 200 {
			t.Fatalf("fail attempt %d: %d", i, code)
		}
		// Re-claim for the retry (except after the terminal one).
		if i < 2 {
			if code, _ := do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"w1"}`, nil); code != 200 {
				t.Fatalf("reclaim %d: %d", i, code)
			}
		}
	}
	if code, env := do(t, srv, "GET", "/api/v1/jobs/"+jid, "", nil); code != 200 || !strings.Contains(string(env.Data), "FAILED") {
		t.Fatalf("job must be FAILED: %d %s", code, env.Data)
	}
}

func TestSignedWorkerAttestationsCanSatisfyQuorum(t *testing.T) {
	srv := newTestServer(t)
	priv, err := sigstore.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := sigstore.PublicToPEM(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []struct{ id, group string }{{"builder-a", "cloud-a"}, {"builder-b", "cloud-b"}} {
		body, _ := json.Marshal(map[string]string{"id": b.id, "independenceGroup": b.group, "signingPublicKey": string(pub)})
		if code, env := do(t, srv, "POST", "/api/v1/builders", string(body), nil); code != 201 {
			t.Fatalf("register %s: %d %#v", b.id, code, env.Error)
		}
	}
	rel := createRelease(t, srv, "")
	code, env := do(t, srv, "POST", "/api/v1/verifications", fmt.Sprintf(`{"releaseId":%q,"evidence":%s}`, rel, agreeEvidence()), nil)
	if code != 201 {
		t.Fatalf("seed verification: %d", code)
	}
	vid := mustData[map[string]any](t, env)["id"].(string)
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vid+"/jobs", `{"builderIds":["builder-a","builder-b"]}`, nil); code != 201 {
		t.Fatalf("enqueue: %d", code)
	}
	d := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for n := 0; n < 2; n++ {
		_, claim := do(t, srv, "POST", "/api/v1/jobs/claim", fmt.Sprintf(`{"owner":"worker-%d"}`, n), nil)
		job := mustData[struct {
			Job struct {
				ID        string `json:"id"`
				BuilderID string `json:"builderId"`
			} `json:"job"`
		}](t, claim).Job
		if n == 0 {
			wrong := sigstore.ProvenanceStatement("source-archive", d, "wrong-commit", job.BuilderID, "https://quorum.example/buildtypes/git-archive/v1")
			forged, err := sigstore.SignStatement(wrong, priv)
			if err != nil {
				t.Fatal(err)
			}
			badPayload, _ := json.Marshal(map[string]any{"ok": true, "digest": d, "commit": "abc123", "attestation": forged})
			badCode, bad := do(t, srv, "POST", "/api/v1/jobs/"+job.ID+"/complete", string(badPayload), nil)
			if badCode != 422 || bad.Error == nil || bad.Error.Code != "ATTESTATION_INVALID" {
				t.Fatalf("replayed/wrong-commit attestation must be rejected: %d %#v", badCode, bad.Error)
			}
		}
		st := sigstore.ProvenanceStatement("source-archive", d, "abc123", job.BuilderID, "https://quorum.example/buildtypes/git-archive/v1")
		dsse, err := sigstore.SignStatement(st, priv)
		if err != nil {
			t.Fatal(err)
		}
		payload, _ := json.Marshal(map[string]any{"ok": true, "digest": d, "commit": "abc123", "attestation": dsse})
		code, response := do(t, srv, "POST", "/api/v1/jobs/"+job.ID+"/complete", string(payload), nil)
		if code != 200 {
			t.Fatalf("signed completion: %d %#v", code, response.Error)
		}
		if n == 1 && !strings.Contains(string(response.Data), "VERIFIED") {
			t.Fatalf("signed independent evidence must verify: %s", response.Data)
		}
	}
}

// Scoped claim via API: workers bound to one verification never receive
// another's jobs; unknown verification id is 404 (typo fails fast).
func TestClaimScopedToVerification(t *testing.T) {
	srv := newTestServer(t)
	rel := createRelease(t, srv, "")
	mkVer := func() string {
		code, env := do(t, srv, "POST", "/api/v1/verifications", fmt.Sprintf(`{"releaseId":%q}`, rel), nil)
		if code != 201 {
			t.Fatalf("seed: %d", code)
		}
		return mustData[map[string]any](t, env)["id"].(string)
	}
	va, vb := mkVer(), mkVer()
	do(t, srv, "POST", "/api/v1/builders", `{"id":"sca","independenceGroup":"g"}`, nil)
	do(t, srv, "POST", "/api/v1/builders", `{"id":"scb","independenceGroup":"g2"}`, nil)
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+va+"/jobs", `{"builderIds":["sca"]}`, nil); code != 201 {
		t.Fatalf("enqueue a: %d", code)
	}
	if code, _ := do(t, srv, "POST", "/api/v1/verifications/"+vb+"/jobs", `{"builderIds":["scb"]}`, nil); code != 201 {
		t.Fatalf("enqueue b: %d", code)
	}
	code, env := do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"wb","verificationId":"`+vb+`"}`, nil)
	if code != 200 || !strings.Contains(string(env.Data), `"claimed":true`) || !strings.Contains(string(env.Data), `"builderId":"scb"`) {
		t.Fatalf("scoped claim must take own job: %d %s", code, env.Data)
	}
	if code, env := do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"wb2","verificationId":"`+vb+`"}`, nil); code != 200 || !strings.Contains(string(env.Data), `"claimed":false`) {
		t.Fatalf("scoped claim on drained queue: %d %s", code, env.Data)
	}
	if code, env := do(t, srv, "POST", "/api/v1/jobs/claim", `{"owner":"wx","verificationId":"ver_nope"}`, nil); code != 404 {
		t.Fatalf("unknown verification scope must 404, got %d %+v", code, env.Error)
	}
}
