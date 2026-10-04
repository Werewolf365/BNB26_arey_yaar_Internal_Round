package cosign_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	qcosign "github.com/quorum/quorum/services/cosign"
)

func TestParseTrustRootValid(t *testing.T) {
	raw := []byte(`{"allowedIssuers":["https://token.actions.githubusercontent.com"],"allowedIdentities":["https://github.com/myorg/myrepo","regexp:^https://github.com/myorg/.*"],"rekorUrl":"https://rekor.example","requireRekor":true}`)
	root, err := qcosign.ParseTrustRoot(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(root.AllowedIssuers) != 1 || len(root.AllowedIdentities) != 2 {
		t.Fatalf("unexpected root: %+v", root)
	}
	if !root.RekorEnforced() || root.RekorFor() != "https://rekor.example" {
		t.Fatalf("rekor not enforced: %+v", root)
	}
}

func TestParseTrustRootRejectsUnknownFields(t *testing.T) {
	if _, err := qcosign.ParseTrustRoot([]byte(`{"allowedIssuers":[],"evil":1}`)); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestParseTrustRootRejectsBadRegexp(t *testing.T) {
	if _, err := qcosign.ParseTrustRoot([]byte(`{"allowedIdentities":["regexp:([bad"]}`)); err == nil {
		t.Fatal("expected error for bad regexp")
	}
}

func TestAuthorizeKeyless(t *testing.T) {
	root := qcosign.TrustRoot{
		AllowedIssuers:    []string{"https://token.actions.githubusercontent.com"},
		AllowedIdentities: []string{"https://github.com/myorg/myrepo", "regexp:^https://github.com/myorg/.*"},
	}
	if err := root.AuthorizeKeyless("https://github.com/myorg/myrepo", "https://token.actions.githubusercontent.com"); err != nil {
		t.Fatalf("exact identity: %v", err)
	}
	if err := root.AuthorizeKeyless("https://github.com/myorg/other", "https://token.actions.githubusercontent.com"); err != nil {
		t.Fatalf("regexp identity: %v", err)
	}
	if err := root.AuthorizeKeyless("https://github.com/evil/repo", "https://token.actions.githubusercontent.com"); err == nil {
		t.Fatal("unlisted identity must fail")
	}
	if err := root.AuthorizeKeyless("https://github.com/myorg/myrepo", "https://evil.example"); err == nil {
		t.Fatal("unlisted issuer must fail")
	}
	if err := qcosign.DefaultTrustRoot().AuthorizeKeyless("anyone", "anything"); err == nil {
		t.Fatal("deny-all root must refuse everything")
	}
}

func TestTrustRootFromEnv(t *testing.T) {
	t.Setenv("QUORUM_TRUST_ROOT", `{"allowedIssuers":["https://a.example"],"allowedIdentities":["x"]}`)
	root, err := qcosign.TrustRootFromEnv()
	if err != nil {
		t.Fatalf("env inline: %v", err)
	}
	if len(root.AllowedIssuers) != 1 {
		t.Fatalf("bad root: %+v", root)
	}
	path := filepath.Join(t.TempDir(), "root.json")
	if err := os.WriteFile(path, []byte(`{"allowedIssuers":["https://b.example"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUORUM_TRUST_ROOT", "@"+path)
	root, err = qcosign.TrustRootFromEnv()
	if err != nil || root.AllowedIssuers[0] != "https://b.example" {
		t.Fatalf("env @path: %+v %v", root, err)
	}
	t.Setenv("QUORUM_TRUST_ROOT", "")
	root, err = qcosign.TrustRootFromEnv()
	if err != nil || len(root.AllowedIssuers) != 0 {
		t.Fatalf("empty env must be deny-all: %+v %v", root, err)
	}
}

func TestVerifyAttestationWithTrustDeniesBeforeCLI(t *testing.T) {
	// qcosign.Provider points at a nonexistent binary: if trust enforcement runs
	// first, we get SIGNER_NOT_TRUSTED without ever needing cosign.
	p := qcosign.Provider{Bin: filepath.Join(t.TempDir(), "no-such-cosign")}
	root := qcosign.TrustRoot{AllowedIssuers: []string{"https://ok.example"}, AllowedIdentities: []string{"good"}}
	res := p.VerifyAttestationWithTrust(context.Background(), root,
		qcosign.AttestOpts{CertIdentity: "evil", CertIssuer: "https://ok.example"}, "b", "e", "")
	if res.State != qcosign.StateInvalidSig {
		t.Fatalf("unlisted identity must be SIGNATURE_INVALID, got %+v", res)
	}
	// Explicit-key anchors pass through to the CLI (which is missing here):
	// the verdict must come from the CLI run, never a faked VERIFIED.
	res = p.VerifyAttestationWithTrust(context.Background(), root,
		qcosign.AttestOpts{KeyRef: "k"}, "b", "e", "")
	if res.State == qcosign.StateVerified {
		t.Fatalf("missing CLI must never verify, got %+v", res)
	}
}

func TestVerifyAttestationWithTrustRekorRequired(t *testing.T) {
	// Real CLI success (stub) + Rekor-required root but no UUID: must fail
	// closed, never VERIFIED without the required transparency evidence.
	dir := t.TempDir()
	blob := filepath.Join(dir, "b.bin")
	if err := os.WriteFile(blob, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(dir, "b.bundle")
	if err := os.WriteFile(env, []byte("STUBATTEST:data"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := qcosign.Provider{Bin: stubBin(t), Timeout: 10 * time.Second, MaxAttempts: 1}
	root := qcosign.TrustRoot{
		AllowedIssuers:    []string{"https://ok.example"},
		AllowedIdentities: []string{"good"},
		RequireRekor:      true,
	}
	opts := qcosign.AttestOpts{CertIdentity: "good", CertIssuer: "https://ok.example"}
	res := p.VerifyAttestationWithTrust(ctx(t), root, opts, blob, env, "")
	if res.State == qcosign.StateVerified {
		t.Fatalf("must not verify without Rekor evidence: %+v", res)
	}
	if res.State != qcosign.StateUnavailable {
		t.Fatalf("want UNAVAILABLE asking for --rekor-entry, got %+v", res)
	}

	// Same, but with a stub Rekor log confirming the entry: VERIFIED.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"deadbeef":{"body":"entry","integratedTime":1}}`))
	}))
	defer srv.Close()
	root.RekorURL = srv.URL
	res = p.VerifyAttestationWithTrust(ctx(t), root, opts, blob, env, "deadbeef")
	if res.State != qcosign.StateVerified {
		t.Fatalf("rekor-confirmed entry must verify: %+v", res)
	}
}
