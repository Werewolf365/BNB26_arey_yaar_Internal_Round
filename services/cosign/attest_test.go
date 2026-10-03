package cosign_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	qcosign "github.com/quorum/quorum/services/cosign"
)

func attestCtx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return c
}

func attestFixture(t *testing.T, bin, keyRef string) (blob, predicate, env string) {
	t.Helper()
	dir := t.TempDir()
	blob = filepath.Join(dir, "artifact.bin")
	os.WriteFile(blob, []byte("quorum-cosign-attest"), 0o600)
	predicate = filepath.Join(dir, "predicate.json")
	os.WriteFile(predicate, []byte(`{"foo":"bar"}`), 0o600)
	env = filepath.Join(dir, "attestation.bundle.json")
	p := qcosign.Provider{Bin: bin, Timeout: 10 * time.Second, MaxAttempts: 1}
	if r := p.AttestBlob(attestCtx(t), keyRef, blob, predicate, "custom", env); r.State != qcosign.StateVerified {
		t.Fatalf("attest: %+v", r)
	}
	return blob, predicate, env
}

func TestAttestRoundTrip(t *testing.T) {
	bin := stubBin(t)
	p := qcosign.Provider{Bin: bin, Timeout: 10 * time.Second, MaxAttempts: 1}
	blob, predicate, env := attestFixture(t, bin, "test-key")
	_ = predicate

	raw, err := os.ReadFile(env)
	if err != nil || !strings.HasPrefix(string(raw), "STUBATTEST:") {
		t.Fatalf("envelope must be a STUBATTEST bundle: %v %q", err, raw)
	}
	got := p.VerifyBlobAttestation(attestCtx(t), qcosign.AttestOpts{KeyRef: "test-key"}, blob, env)
	if got.State != qcosign.StateVerified {
		t.Fatalf("verify attestation: %+v", got)
	}
	if len(got.Raw) == 0 {
		t.Fatal("raw output must be preserved for audit")
	}
}

func TestAttestKeylessVerify(t *testing.T) {
	bin := stubBin(t)
	p := qcosign.Provider{Bin: bin, Timeout: 10 * time.Second, MaxAttempts: 1}
	// Keyless signing (no keyRef) passes through to the CLI; the stub
	// records the envelope without a key.
	blob, _, env := attestFixture(t, bin, "")
	opts := qcosign.AttestOpts{CertIdentity: "ci@example.com", CertIssuer: "https://accounts.example.com"}
	if got := p.VerifyBlobAttestation(attestCtx(t), opts, blob, env); got.State != qcosign.StateVerified {
		t.Fatalf("keyless verify must be VERIFIED, got %+v", got)
	}
	// Keyless verify with a forged envelope is still rejected.
	forged := filepath.Join(t.TempDir(), "forged.bundle.json")
	os.WriteFile(forged, []byte("not-an-attestation"), 0o600)
	if got := p.VerifyBlobAttestation(attestCtx(t), opts, blob, forged); got.State != qcosign.StateInvalidSig {
		t.Fatalf("forged envelope must be SIGNATURE_INVALID, got %+v", got)
	}
}

func TestAttestRejects(t *testing.T) {
	bin := stubBin(t)
	p := qcosign.Provider{Bin: bin, Timeout: 10 * time.Second, MaxAttempts: 1}
	blob, _, env := attestFixture(t, bin, "test-key")

	// Forged envelope bytes.
	forged := filepath.Join(t.TempDir(), "forged.bundle.json")
	os.WriteFile(forged, []byte("not-an-attestation"), 0o600)
	if got := p.VerifyBlobAttestation(attestCtx(t), qcosign.AttestOpts{KeyRef: "test-key"}, blob, forged); got.State != qcosign.StateInvalidSig {
		t.Fatalf("forged envelope must be SIGNATURE_INVALID, got %+v", got)
	}
	// Wrong key.
	if got := p.VerifyBlobAttestation(attestCtx(t), qcosign.AttestOpts{KeyRef: "WRONG"}, blob, env); got.State != qcosign.StateInvalidSig {
		t.Fatalf("wrong key must be SIGNATURE_INVALID, got %+v", got)
	}
	// Missing binary is UNAVAILABLE, never a faked verdict.
	absent := qcosign.Provider{Bin: "cosign-definitely-missing", Timeout: 5 * time.Second, MaxAttempts: 1}
	if got := absent.VerifyBlobAttestation(attestCtx(t), qcosign.AttestOpts{KeyRef: "k"}, blob, env); got.State != qcosign.StateUnavailable {
		t.Fatalf("missing binary verify must be UNAVAILABLE, got %+v", got)
	}
	if got := absent.AttestBlob(attestCtx(t), "k", blob, filepath.Join(t.TempDir(), "p.json"), "custom", filepath.Join(t.TempDir(), "o.json")); got.State != qcosign.StateUnavailable {
		t.Fatalf("missing binary attest must be UNAVAILABLE, got %+v", got)
	}
	// Empty args are INVALID_INPUT, not operational.
	if got := p.VerifyBlobAttestation(attestCtx(t), qcosign.AttestOpts{KeyRef: "k"}, "", ""); got.State != qcosign.StateInvalidInput {
		t.Fatalf("empty paths must be INVALID_INPUT, got %+v", got)
	}
	if got := p.VerifyBlobAttestation(attestCtx(t), qcosign.AttestOpts{}, blob, env); got.State != qcosign.StateInvalidInput {
		t.Fatalf("no trust anchor must be INVALID_INPUT, got %+v", got)
	}
	if got := p.VerifyBlobAttestation(attestCtx(t), qcosign.AttestOpts{CertIdentity: "only-id"}, blob, env); got.State != qcosign.StateInvalidInput {
		t.Fatalf("half keyless identity must be INVALID_INPUT, got %+v", got)
	}
	if got := p.AttestBlob(attestCtx(t), "", "", "", "", ""); got.State != qcosign.StateInvalidInput {
		t.Fatalf("empty attest args must be INVALID_INPUT, got %+v", got)
	}
}

// TestLiveAttestRoundTrip exercises the real attest-blob /
// verify-blob-attestation round-trip end-to-end (env-gated). Requires cosign
// on PATH plus network (Rekor); uses an ephemeral key (COSIGN_PASSWORD must
// be set, empty string is fine) in a temp dir so nothing touches real keys.
func TestLiveAttestRoundTrip(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_COSIGN") != "1" {
		t.Skip("set QUORUM_LIVE_COSIGN=1 with cosign on PATH")
	}
	p := qcosign.DefaultProvider()
	c, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	if err := p.Probe(c); err != nil {
		t.Skipf("cosign absent: %v", err)
	}
	if _, ok := os.LookupEnv("COSIGN_PASSWORD"); !ok {
		t.Skip("set COSIGN_PASSWORD (empty ok) for ephemeral key generation")
	}
	dir := t.TempDir()
	blob := filepath.Join(dir, "live.bin")
	os.WriteFile(blob, []byte("quorum-live-cosign-attest"), 0o600)
	predicate := filepath.Join(dir, "predicate.json")
	os.WriteFile(predicate, []byte(`{"live":"quorum-attest"}`), 0o600)
	key := filepath.Join(dir, "cosign.key")
	if out, err := exec.CommandContext(c, "cosign", "generate-key-pair", "--private-key", key).CombinedOutput(); err != nil {
		t.Skipf("keygen unavailable: %v\n%s", err, out)
	}
	pub := key + ".pub"
	env := filepath.Join(dir, "live.bundle.json")
	if r := p.AttestBlob(c, key, blob, predicate, "custom", env); r.State != qcosign.StateVerified {
		t.Fatalf("live attest: %+v", r)
	}
	if r := p.VerifyBlobAttestation(c, qcosign.AttestOpts{KeyRef: pub}, blob, env); r.State != qcosign.StateVerified {
		t.Fatalf("live verify attestation: %+v", r)
	}
}

func TestAttestEdgePaths(t *testing.T) {
	bin := stubBin(t)
	p := qcosign.Provider{Bin: bin, Timeout: 10 * time.Second, MaxAttempts: 1}
	dir := t.TempDir()
	blob := filepath.Join(dir, "a.bin")
	os.WriteFile(blob, []byte("edge"), 0o600)
	predicate := filepath.Join(dir, "p.json")
	os.WriteFile(predicate, []byte(`{}`), 0o600)
	out := filepath.Join(dir, "o.bundle.json")

	// Empty predicate type falls back to the cosign default.
	if r := p.AttestBlob(attestCtx(t), "test-key", blob, predicate, "", out); r.State != qcosign.StateVerified {
		t.Fatalf("empty type must default: %+v", r)
	}
	// Missing binary on attest is UNAVAILABLE, never a verdict.
	absent := qcosign.Provider{Bin: "cosign-definitely-missing", Timeout: 5 * time.Second, MaxAttempts: 1}
	if r := absent.AttestBlob(attestCtx(t), "k", blob, predicate, "custom", out); r.State != qcosign.StateUnavailable {
		t.Fatalf("missing binary attest must be UNAVAILABLE: %+v", r)
	}
	// RekorURL passthrough (stub ignores it, exercises the flag branch).
	opts := qcosign.AttestOpts{KeyRef: "test-key", RekorURL: "https://rekor.sigstore.dev"}
	if r := p.VerifyBlobAttestation(attestCtx(t), opts, blob, predicate); r.State != qcosign.StateInvalidSig {
		t.Fatalf("non-envelope bundle must be invalid: %+v", r)
	}
	// Exit 0 without a marker is MALFORMED, never silent success.
	if r := p.VerifyBlobAttestation(attestCtx(t), qcosign.AttestOpts{KeyRef: "NOMARKER"}, blob, out); r.State != qcosign.StateMalformed {
		t.Fatalf("markerless exit 0 must be MALFORMED: %+v", r)
	}
	if r := p.VerifyBlob(attestCtx(t), "NOMARKER", blob, out, ""); r.State != qcosign.StateMalformed {
		t.Fatalf("markerless blob verify must be MALFORMED: %+v", r)
	}
	// Canceled context surfaces UNAVAILABLE from run().
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if r := p.SignBlob(canceled, "k", blob, out, ""); r.State != qcosign.StateUnavailable {
		t.Fatalf("canceled ctx must be UNAVAILABLE: %+v", r)
	}
}
