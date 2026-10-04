package cosign_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	qcosign "github.com/quorum/quorum/services/cosign"
)

// stubBin builds the deterministic cosign stand-in once per test run.
// The stub lives in testdata (ignored by the go tool) so the suite needs no
// network, no keys, and no cosign install.
func stubBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "cosignstub")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	src := filepath.Join("testdata", "cosignstub", "main.go")
	cmd := exec.Command("go", "build", "-o", bin, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v\n%s", err, out)
	}
	return bin
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestProbe(t *testing.T) {
	p := qcosign.Provider{Bin: stubBin(t), Timeout: 10 * time.Second, MaxAttempts: 1}
	if err := p.Probe(ctx(t)); err != nil {
		t.Fatalf("stub must probe: %v", err)
	}
	missing := qcosign.Provider{Bin: "cosign-definitely-missing", Timeout: 5 * time.Second, MaxAttempts: 1}
	if err := missing.Probe(ctx(t)); err == nil {
		t.Fatal("missing binary must fail probe")
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	bin := stubBin(t)
	p := qcosign.Provider{Bin: bin, Timeout: 10 * time.Second, MaxAttempts: 1}
	dir := t.TempDir()
	blob := filepath.Join(dir, "artifact.bin")
	os.WriteFile(blob, []byte("quorum-cosign-roundtrip"), 0o600)
	sig := filepath.Join(dir, "artifact.sig")
	bundle := filepath.Join(dir, "artifact.bundle.json")

	signed := p.SignBlob(ctx(t), "test-key", blob, sig, bundle)
	if signed.State != qcosign.StateVerified {
		t.Fatalf("sign: %+v", signed)
	}
	if _, err := os.Stat(bundle); err != nil {
		t.Fatalf("bundle must be preserved: %v", err)
	}
	got := p.VerifyBlob(ctx(t), "test-key", blob, sig, bundle)
	if got.State != qcosign.StateVerified {
		t.Fatalf("verify: %+v", got)
	}
	if len(got.Raw) == 0 {
		t.Fatal("raw output must be preserved for audit")
	}
}

func TestVerifyRejects(t *testing.T) {
	bin := stubBin(t)
	p := qcosign.Provider{Bin: bin, Timeout: 10 * time.Second, MaxAttempts: 1}
	dir := t.TempDir()
	blob := filepath.Join(dir, "a.bin")
	os.WriteFile(blob, []byte("data"), 0o600)

	// Forged signature bytes.
	forged := filepath.Join(dir, "forged.sig")
	os.WriteFile(forged, []byte("not-a-real-signature"), 0o600)
	if got := p.VerifyBlob(ctx(t), "test-key", blob, forged, ""); got.State != qcosign.StateInvalidSig {
		t.Fatalf("forged sig must be SIGNATURE_INVALID, got %+v", got)
	}
	// Wrong key.
	sig := filepath.Join(dir, "a.sig")
	if r := p.SignBlob(ctx(t), "test-key", blob, sig, ""); r.State != qcosign.StateVerified {
		t.Fatalf("sign: %+v", r)
	}
	if got := p.VerifyBlob(ctx(t), "WRONG", blob, sig, ""); got.State != qcosign.StateInvalidSig {
		t.Fatalf("wrong key must be SIGNATURE_INVALID, got %+v", got)
	}
	// Missing binary is UNAVAILABLE, never a faked verdict.
	absent := qcosign.Provider{Bin: "cosign-definitely-missing", Timeout: 5 * time.Second, MaxAttempts: 1}
	if got := absent.VerifyBlob(ctx(t), "k", blob, sig, ""); got.State != qcosign.StateUnavailable {
		t.Fatalf("missing binary must be UNAVAILABLE, got %+v", got)
	}
	// Empty args are INVALID_INPUT, not operational.
	if got := p.VerifyBlob(ctx(t), "", "", "", ""); got.State != qcosign.StateInvalidInput {
		t.Fatalf("empty args must be INVALID_INPUT, got %+v", got)
	}
	if got := p.SignBlob(ctx(t), "", blob, sig, ""); got.State != qcosign.StateInvalidInput {
		t.Fatalf("empty key must be INVALID_INPUT, got %+v", got)
	}
}

func rekorFixture(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/log/publicKey", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE.stub\n-----END PUBLIC KEY-----"))
	})
	mux.HandleFunc("/api/v1/log/entries/", func(w http.ResponseWriter, r *http.Request) {
		uuid := strings.TrimPrefix(r.URL.Path, "/api/v1/log/entries/")
		if uuid == "deadbeef" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"deadbeef":{"body":"stub-entry","integratedTime":123}}`))
			return
		}
		http.NotFound(w, r)
	})
	return httptest.NewServer(mux)
}

func TestRekorClient(t *testing.T) {
	srv := rekorFixture(t)
	defer srv.Close()
	r := qcosign.RekorClient{BaseURL: srv.URL, Timeout: 10 * time.Second}

	pem, err := r.PublicKey(ctx(t))
	if err != nil || !strings.Contains(pem, "BEGIN PUBLIC KEY") {
		t.Fatalf("publicKey: %v %q", err, pem)
	}
	entry, err := r.Entry(ctx(t), "deadbeef")
	if err != nil || !strings.Contains(string(entry), "stub-entry") {
		t.Fatalf("entry: %v %s", err, entry)
	}
	if _, err := r.Entry(ctx(t), "no-such-uuid"); err == nil || !strings.Contains(err.Error(), "NOT_FOUND") {
		t.Fatalf("unknown uuid must be NOT_FOUND, got %v", err)
	}
	if _, err := r.Entry(ctx(t), ""); err == nil || !strings.Contains(err.Error(), "INVALID_INPUT") {
		t.Fatalf("empty uuid must be INVALID_INPUT, got %v", err)
	}
	down := qcosign.RekorClient{BaseURL: "http://127.0.0.1:1", Timeout: 3 * time.Second}
	if _, err := down.PublicKey(ctx(t)); err == nil || !strings.Contains(err.Error(), "UNAVAILABLE") {
		t.Fatalf("dead log must be UNAVAILABLE, got %v", err)
	}
}

// TestLiveRekorLiveness hits the real public-good log (env-gated, read-only,
// no credentials). It asserts reachability only — never pinned entry data.
func TestLiveRekorLiveness(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_COSIGN") != "1" {
		t.Skip("set QUORUM_LIVE_COSIGN=1 with network access")
	}
	r := qcosign.DefaultRekor()
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pem, err := r.PublicKey(c)
	if err != nil {
		t.Skipf("rekor unreachable: %v", err)
	}
	if !strings.Contains(pem, "BEGIN PUBLIC KEY") {
		t.Fatalf("unexpected publicKey body %q", pem)
	}
}

// TestLiveCosignRoundTrip exercises the real CLI end-to-end (env-gated).
// Requires cosign on PATH; uses an ephemeral key (COSIGN_PASSWORD must be set,
// empty string is fine) in a temp dir so nothing touches real keys.
func TestLiveCosignRoundTrip(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_COSIGN") != "1" {
		t.Skip("set QUORUM_LIVE_COSIGN=1 with cosign on PATH")
	}
	p := qcosign.DefaultProvider()
	c, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := p.Probe(c); err != nil {
		t.Skipf("cosign absent: %v", err)
	}
	if _, ok := os.LookupEnv("COSIGN_PASSWORD"); !ok {
		t.Skip("set COSIGN_PASSWORD (empty ok) for ephemeral key generation")
	}
	dir := t.TempDir()
	blob := filepath.Join(dir, "live.bin")
	os.WriteFile(blob, []byte("quorum-live-cosign"), 0o600)
	prefix := filepath.Join(dir, "cosign")
	key := prefix + ".key"
	if out, err := exec.CommandContext(c, "cosign", "generate-key-pair", "--output-key-prefix", prefix).CombinedOutput(); err != nil {
		t.Skipf("keygen unavailable: %v\n%s", err, out)
	}
	pub := prefix + ".pub"
	sig := filepath.Join(dir, "live.sig")
	bundle := filepath.Join(dir, "live.bundle.json")
	if r := p.SignBlob(c, key, blob, sig, bundle); r.State != qcosign.StateVerified {
		t.Fatalf("live sign: %+v", r)
	}
	// Verify with the Rekor bundle: the CLI checks transparency inclusion
	// against the real log, not just the key.
	if r := p.VerifyBlob(c, pub, blob, sig, bundle); r.State != qcosign.StateVerified {
		t.Fatalf("live verify (rekor bundle): %+v", r)
	}
}

func TestRekorEdges(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/log/publicKey", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not-a-pem-body"))
	})
	mux.HandleFunc("/api/v1/log/entries/json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json{"))
	})
	mux.HandleFunc("/api/v1/log/entries/boom", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/api/v1/log/entries/bad", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	rr := qcosign.RekorClient{BaseURL: srv.URL, Timeout: 10 * time.Second}
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := rr.PublicKey(c); err == nil {
		t.Fatal("non-PEM publicKey must fail")
	}
	if _, err := rr.Entry(c, "json"); err == nil {
		t.Fatal("non-JSON entry must fail")
	}
	if _, err := rr.Entry(c, "boom"); err == nil {
		t.Fatal("500 entry must fail")
	}
	if _, err := rr.Entry(c, "bad"); err == nil {
		t.Fatal("400 entry must fail")
	}
	// Unparseable base URL fails request construction, never a network call.
	badURL := qcosign.RekorClient{BaseURL: "http://x\x7f", Timeout: 10 * time.Second}
	if _, err := badURL.Entry(c, "x"); err == nil {
		t.Fatal("bad base URL must fail")
	}
	// Entry against a dead endpoint surfaces UNAVAILABLE.
	dead := qcosign.RekorClient{BaseURL: "http://127.0.0.1:1", Timeout: 3 * time.Second}
	if _, err := dead.Entry(c, "x"); err == nil {
		t.Fatal("dead endpoint entry must fail")
	}
	// Truncated body fails the read, non-200 publicKey fails liveness.
	weird := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "publicKey") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Length", "abc")
		w.WriteHeader(http.StatusOK)
	}))
	defer weird.Close()
	wr := qcosign.RekorClient{BaseURL: weird.URL, Timeout: 10 * time.Second}
	if _, err := wr.PublicKey(c); err == nil {
		t.Fatal("500 publicKey must fail")
	}
	if _, err := wr.Entry(c, "whatever"); err == nil {
		t.Fatal("truncated body must fail")
	}
}

func TestRekorCustomClient(t *testing.T) {
	srv := rekorFixture(t)
	defer srv.Close()
	r := qcosign.RekorClient{BaseURL: srv.URL, Timeout: 10 * time.Second, Client: srv.Client()}
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pem, err := r.PublicKey(c)
	if err != nil || !strings.Contains(pem, "BEGIN PUBLIC KEY") {
		t.Fatalf("custom client: %v", err)
	}
}
