package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// cosignStub builds the deterministic cosign stand-in (see
// services/cosign/testdata/cosignstub) so CLI tests need no install.
func cosignStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "cosignstub")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	src := filepath.Join("..", "..", "..", "services", "cosign", "testdata", "cosignstub", "main.go")
	cmd := exec.Command("go", "build", "-o", bin, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v\n%s", err, out)
	}
	return bin
}

func TestAttestCosignMatrix(t *testing.T) {
	bin := cosignStub(t)
	dir := t.TempDir()
	artifact := filepath.Join(dir, "a.bin")
	os.WriteFile(artifact, []byte("quorum-cosign-cli"), 0o600)
	sig := filepath.Join(dir, "a.sig")

	if code, _, _ := execute("attest", "cosign-sign",
		"--artifact", artifact, "--key", "test-key",
		"--output-signature", sig, "--cosign-bin", bin); code != 0 {
		t.Fatalf("cosign-sign must exit 0, got %d", code)
	}
	if code, out, _ := execute("attest", "cosign-verify",
		"--artifact", artifact, "--key", "test-key",
		"--signature", sig, "--cosign-bin", bin); code != 0 || !strings.Contains(out, "verified") {
		t.Fatalf("valid cosign signature must exit 0: %d %s", code, out)
	}
	// Forged signature -> rejected.
	forged := filepath.Join(dir, "forged.sig")
	os.WriteFile(forged, []byte("forged"), 0o600)
	if code, _, _ := execute("attest", "cosign-verify",
		"--artifact", artifact, "--key", "test-key",
		"--signature", forged, "--cosign-bin", bin); code != 1 {
		t.Fatalf("forged cosign signature must exit 1, got %d", code)
	}
	// Missing binary -> operational, never a faked verdict.
	if code, _, _ := execute("attest", "cosign-verify",
		"--artifact", artifact, "--key", "test-key",
		"--signature", sig, "--cosign-bin", "cosign-definitely-missing"); code != 4 {
		t.Fatalf("missing cosign must exit 4, got %d", code)
	}
	// Missing flags -> invalid.
	for _, args := range [][]string{
		{"attest", "cosign-sign", "--artifact", artifact},
		{"attest", "cosign-verify", "--artifact", artifact},
		{"attest", "rekor-get"},
	} {
		if code, _, _ := execute(args...); code != 5 {
			t.Fatalf("%v must exit 5, got %d", args, code)
		}
	}
}

func TestAttestRekorGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/log/entries/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/good") {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"good":{"body":"entry"}}`))
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	if code, out, _ := execute("attest", "rekor-get", "--uuid", "good", "--rekor-url", srv.URL); code != 0 || !strings.Contains(out, "entry") {
		t.Fatalf("known entry must exit 0: %d %s", code, out)
	}
	if code, _, _ := execute("attest", "rekor-get", "--uuid", "missing", "--rekor-url", srv.URL); code == 0 {
		t.Fatal("unknown entry must not exit 0")
	}
	if code, _, _ := execute("attest", "rekor-get", "--uuid", "x", "--rekor-url", "http://127.0.0.1:1"); code != 4 {
		t.Fatalf("dead log must exit 4, got %d", code)
	}
}
