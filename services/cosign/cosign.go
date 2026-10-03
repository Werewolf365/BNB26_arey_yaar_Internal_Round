// Package cosign is the Sigstore Cosign/Rekor evidence adapter.
//
// It shells the real `cosign` CLI (install:
// https://docs.sigstore.dev/cosign/installation/) and queries the Rekor
// transparency log over HTTP. Reads that need no credentials stay default;
// simple blob sign/verify pins explicit keys (bring-your-own key), while the
// DSSE attest-blob path (services/cosign/attest.go) additionally supports
// keyless (Fulcio/OIDC) signing passthrough and non-interactive keyless
// verification by certificate identity.
//
// Design mirrors services/ossrebuild: deterministic fixture/offline behavior
// is the default for tests; live mode is explicit and env-gated, and a
// missing binary is UNAVAILABLE with an actionable hint — never a faked
// VERIFIED.
package cosign

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// Verification states (adapter contract).
const (
	StateVerified     = "VERIFIED"
	StateInvalidSig   = "SIGNATURE_INVALID"
	StateUnavailable  = "UNAVAILABLE"
	StateInvalidInput = "INVALID_INPUT"
	StateNotFound     = "NOT_FOUND"
	StateMalformed    = "MALFORMED_EVIDENCE"
)

// Result is the normalized outcome of one cosign operation.
// Raw preserves untouched CLI output for forensics.
type Result struct {
	State  string `json:"state"`
	Detail string `json:"detail"`
	Raw    []byte `json:"-"`
}

// Provider shells the cosign CLI. Bin is the executable path; Timeout bounds
// each call; MaxAttempts allows one safe retry (verifications are reads).
type Provider struct {
	Bin         string
	Timeout     time.Duration
	MaxAttempts int
}

// DefaultProvider uses PATH lookup with sane bounds.
func DefaultProvider() Provider {
	return Provider{Bin: "cosign", Timeout: 90 * time.Second, MaxAttempts: 2}
}

func (p Provider) timeout() time.Duration {
	if p.Timeout <= 0 {
		return 90 * time.Second
	}
	return p.Timeout
}

func (p Provider) attempts() int {
	if p.MaxAttempts < 1 {
		return 1
	}
	return p.MaxAttempts
}

// run executes the CLI with timeout + safe retry. Both streams are merged
// into the returned error detail on failure (cosign prints status lines on
// stderr, so callers must not assume stdout-only output).
func (p Provider) run(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	var last error
	for i := 0; i < p.attempts(); i++ {
		cctx, cancel := context.WithTimeout(ctx, p.timeout())
		cmd := exec.CommandContext(cctx, p.Bin, args...)
		var so, se bytes.Buffer
		cmd.Stdout = &so
		cmd.Stderr = &se
		runErr := cmd.Run()
		cancel()
		if runErr == nil {
			return so.String(), se.String(), nil
		}
		if ctx.Err() != nil {
			return "", "", fmt.Errorf("UNAVAILABLE: %v", ctx.Err())
		}
		last = fmt.Errorf("%v: %s", runErr, strings.TrimSpace(so.String()+"\n"+se.String()))
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	return "", "", fmt.Errorf("UNAVAILABLE: %v", last)
}

// Probe checks the binary executes (exit 0 on `version`).
func (p Provider) Probe(ctx context.Context) error {
	if _, _, err := p.run(ctx, "version"); err != nil {
		return fmt.Errorf("UNAVAILABLE: cosign binary missing or broken: %v (install: https://docs.sigstore.dev/cosign/installation/)", err)
	}
	return nil
}

// SignBlob signs an artifact with an explicit key:
// cosign sign-blob --key <keyRef> --output-signature <sigOut> --yes <blob>.
// keyRef is a cosign key reference (file path, k8s://, or KMS URI).
// bundleOut is optional: when non-empty, --output-bundle is also passed so
// the Rekor transparency bundle is preserved alongside the signature.
func (p Provider) SignBlob(ctx context.Context, keyRef, blobPath, sigOut, bundleOut string) Result {
	if keyRef == "" || blobPath == "" || sigOut == "" {
		return Result{State: StateInvalidInput, Detail: "INVALID_INPUT: key, blob and output-signature are required"}
	}
	args := []string{"sign-blob", "--key", keyRef, "--output-signature", sigOut, "--yes"}
	if bundleOut != "" {
		args = append(args, "--output-bundle", bundleOut)
	}
	args = append(args, blobPath)
	stdout, stderr, err := p.run(ctx, args...)
	if err != nil {
		return Result{State: StateUnavailable, Detail: err.Error(), Raw: []byte(stdout + "\n" + stderr)}
	}
	return Result{State: StateVerified, Detail: "blob signed (" + strings.TrimSpace(stdout+" "+stderr) + ")", Raw: []byte(stdout + "\n" + stderr)}
}

// VerifyBlob verifies a blob signature with an explicit key:
// cosign verify-blob --key <keyRef> --signature <sigPath> [--bundle <bundle>] <blob>.
// Exit 0 => VERIFIED; any verification failure => SIGNATURE_INVALID (exit 1
// in tests/CI); missing binary/timeout => UNAVAILABLE. Raw output is always
// preserved so a decision can be re-audited.
func (p Provider) VerifyBlob(ctx context.Context, keyRef, blobPath, sigPath, bundlePath string) Result {
	if keyRef == "" || blobPath == "" || sigPath == "" {
		return Result{State: StateInvalidInput, Detail: "INVALID_INPUT: key, blob and signature are required"}
	}
	args := []string{"verify-blob", "--key", keyRef, "--signature", sigPath}
	if bundlePath != "" {
		args = append(args, "--bundle", bundlePath)
	}
	args = append(args, blobPath)
	stdout, stderr, err := p.run(ctx, args...)
	out := stdout + "\n" + stderr
	if err != nil {
		text := err.Error() + "\n" + out
		// The CLI never ran (binary missing, timeout, context cancel):
		// UNAVAILABLE with a hint, never a faked verdict.
		for _, marker := range []string{
			"executable file not found", "not found in %PATH%", "not found in $PATH",
			"deadline exceeded", "context canceled", "signal: killed",
		} {
			if strings.Contains(text, marker) {
				return Result{State: StateUnavailable, Detail: err.Error() + " (install: https://docs.sigstore.dev/cosign/installation/)", Raw: []byte(out)}
			}
		}
		// The CLI ran and refused: signature invalid.
		return Result{State: StateInvalidSig, Detail: "SIGNATURE_INVALID: " + strings.TrimSpace(text), Raw: []byte(out)}
	}
	if !strings.Contains(strings.ToLower(out), "verified") {
		// Exit 0 but no confirmation marker: treat as malformed evidence,
		// never silent success.
		return Result{State: StateMalformed, Detail: "MALFORMED_EVIDENCE: cosign exited 0 without a verification marker", Raw: []byte(out)}
	}
	return Result{State: StateVerified, Detail: "signature verified by cosign", Raw: []byte(out)}
}

// RekorClient queries a Rekor transparency log over HTTP.
// Default BaseURL is the public-good instance; tests inject httptest URLs.
type RekorClient struct {
	BaseURL string
	Timeout time.Duration
	// Client allows stub transport in tests; nil means http.DefaultClient.
	Client *http.Client
}

// DefaultRekor is the Sigstore public-good Rekor instance.
func DefaultRekor() RekorClient {
	return RekorClient{BaseURL: "https://rekor.sigstore.dev", Timeout: 30 * time.Second}
}

func (r RekorClient) timeout() time.Duration {
	if r.Timeout <= 0 {
		return 30 * time.Second
	}
	return r.Timeout
}

func (r RekorClient) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{Timeout: r.timeout()}
}

func (r RekorClient) get(ctx context.Context, path string) (int, []byte, error) {
	base := strings.TrimSuffix(r.BaseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("INVALID_INPUT: bad rekor request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := r.client().Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("UNAVAILABLE: rekor unreachable: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("UNAVAILABLE: rekor read failed: %v", err)
	}
	return resp.StatusCode, body, nil
}

// PublicKey fetches the Rekor log public key (liveness probe that needs no
// credentials). A 200 with a PEM-looking body means the log is reachable.
func (r RekorClient) PublicKey(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()
	code, body, err := r.get(ctx, "/api/v1/log/publicKey")
	if err != nil {
		return "", err
	}
	if code != http.StatusOK {
		return "", fmt.Errorf("UNAVAILABLE: rekor publicKey status %d", code)
	}
	text := strings.TrimSpace(string(body))
	if !strings.Contains(text, "BEGIN PUBLIC KEY") {
		return "", fmt.Errorf("MALFORMED_EVIDENCE: rekor publicKey is not PEM")
	}
	return text, nil
}

// Entry fetches one transparency entry by UUID and returns the raw JSON.
// 404 => NOT_FOUND (typed, not an error); other non-200 => UNAVAILABLE.
// The body is validated as JSON before return so callers never store poison.
func (r RekorClient) Entry(ctx context.Context, uuid string) ([]byte, error) {
	if strings.TrimSpace(uuid) == "" {
		return nil, fmt.Errorf("INVALID_INPUT: entry UUID is required")
	}
	ctx, cancel := context.WithTimeout(ctx, r.timeout())
	defer cancel()
	code, body, err := r.get(ctx, "/api/v1/log/entries/"+uuid)
	if err != nil {
		return nil, err
	}
	switch code {
	case http.StatusOK:
		var v any
		if jerr := json.Unmarshal(body, &v); jerr != nil {
			return nil, fmt.Errorf("MALFORMED_EVIDENCE: rekor entry is not JSON: %v", jerr)
		}
		return body, nil
	case http.StatusNotFound:
		return nil, fmt.Errorf("NOT_FOUND: no rekor entry %q", uuid)
	default:
		return nil, fmt.Errorf("UNAVAILABLE: rekor entry status %d", code)
	}
}
