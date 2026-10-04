// Package client speaks to the Quorum API on behalf of one worker.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to the API base (e.g. http://localhost:8080).
type Client struct {
	Base       string
	HTTP       *http.Client
	Owner      string
	BuilderIDs []string // builders this worker may execute as
}

// Job is the claim/complete payload subset.
type Job struct {
	ID             string `json:"id"`
	VerificationID string `json:"verificationId"`
	BuilderID      string `json:"builderId"`
	Status         string `json:"status"`
	Attempts       int    `json:"attempts"`
	MaxAttempts    int    `json:"maxAttempts"`
	ResultDigest   string `json:"resultDigest"`
	ResultCommit   string `json:"resultCommit"`
}

func New(base, owner string) *Client {
	return &Client{Base: base, Owner: owner, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) call(method, path string, body any) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.Base+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("OPERATIONAL: api unreachable: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return raw, resp.StatusCode, nil
}

type apiEnvelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func unwrap(raw []byte, status int) (json.RawMessage, error) {
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("OPERATIONAL: bad api envelope: %v", err)
	}
	if status >= 400 {
		code := "OPERATIONAL"
		msg := string(raw)
		if env.Error != nil {
			code, msg = env.Error.Code, env.Error.Message
		}
		return nil, fmt.Errorf("%s: %s", code, msg)
	}
	return env.Data, nil
}

// Claim asks for one queued job. verificationID scopes the claim to one
// verification ("" = any queue); scoped workers never touch other runs' jobs.
func (c *Client) Claim(verificationID string) (Job, bool, error) {
	body := map[string]any{"owner": c.Owner, "leaseSeconds": 300}
	if verificationID != "" {
		body["verificationId"] = verificationID
	}
	raw, status, err := c.call("POST", "/api/v1/jobs/claim", body)
	if err != nil {
		return Job{}, false, err
	}
	data, err := unwrap(raw, status)
	if err != nil {
		return Job{}, false, err
	}
	var out struct {
		Claimed bool `json:"claimed"`
		Job     Job  `json:"job"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return Job{}, false, err
	}
	return out.Job, out.Claimed, nil
}

// Release fetches the source identity for a verification.
func (c *Client) ReleaseForVerification(verificationID string) (repo, commit string, err error) {
	raw, status, err := c.call("GET", "/api/v1/verifications/"+verificationID, nil)
	if err != nil {
		return "", "", err
	}
	data, err := unwrap(raw, status)
	if err != nil {
		return "", "", err
	}
	var ver struct {
		ReleaseID string `json:"releaseId"`
	}
	if err := json.Unmarshal(data, &ver); err != nil {
		return "", "", err
	}
	raw, status, err = c.call("GET", "/api/v1/releases/"+ver.ReleaseID, nil)
	if err != nil {
		return "", "", err
	}
	data, err = unwrap(raw, status)
	if err != nil {
		return "", "", err
	}
	var rel struct {
		Repo   string `json:"repo"`
		Commit string `json:"commit"`
	}
	if err := json.Unmarshal(data, &rel); err != nil {
		return "", "", err
	}
	return rel.Repo, rel.Commit, nil
}

// Complete reports the rebuild outcome. ok=false must carry an error code.
func (c *Client) Complete(jobID string, ok bool, digest, commit, errCode, errDetail string, attestation any) error {
	raw, status, err := c.call("POST", "/api/v1/jobs/"+jobID+"/complete",
		map[string]any{"ok": ok, "digest": digest, "commit": commit, "errorCode": errCode, "errorDetail": errDetail, "attestation": attestation})
	if err != nil {
		return err
	}
	_, err = unwrap(raw, status)
	return err
}
