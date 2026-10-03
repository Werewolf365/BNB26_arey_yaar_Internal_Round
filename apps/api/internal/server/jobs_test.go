package server_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
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
	if !strings.Contains(string(env.Data), "VERIFIED") || strings.Contains(string(env.Data), "WITH_CONFLICT") {
		t.Fatalf("two agreeing rebuilds must auto-verify: %s", env.Data)
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