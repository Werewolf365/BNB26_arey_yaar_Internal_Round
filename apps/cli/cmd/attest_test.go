package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "a.pem")
	pub := filepath.Join(dir, "b.pem")
	artifact := filepath.Join(dir, "tiny.bin")
	os.WriteFile(artifact, []byte("quorum-tiny-artifact-v1"), 0o600)
	env := filepath.Join(dir, "env.json")

	if code, _, _ := execute("attest", "genkey", "--private", priv, "--public", pub); code != 0 {
		t.Fatalf("genkey: %d", code)
	}
	if code, _, _ := execute("attest", "sign", "--artifact", artifact, "--commit", "abc123",
		"--builder", "builder-a", "--key", priv, "--output", env); code != 0 {
		t.Fatalf("sign: %d", code)
	}
	digest := "sha256:125e91d8d55d64934163a812f1f0ced7b1b54b57718d5391d3a224612ec91227"
	if code, _, _ := execute("attest", "verify", "--envelope", env, "--key", pub,
		"--expect-digest", digest, "--expect-commit", "abc123", "--allow-builder", "builder-a"); code != 0 {
		t.Fatalf("verify: %d", code)
	}
	// Wrong digest -> rejected.
	if code, _, _ := execute("attest", "verify", "--envelope", env, "--key", pub,
		"--expect-digest", "sha256:"+strings.Repeat("0", 64)); code != 1 {
		t.Fatalf("wrong digest: %d", code)
	}
	// Wrong commit -> rejected.
	if code, _, _ := execute("attest", "verify", "--envelope", env, "--key", pub,
		"--expect-digest", digest, "--expect-commit", "evil"); code != 1 {
		t.Fatalf("wrong commit: %d", code)
	}
	// Unallowlisted builder -> rejected.
	if code, _, _ := execute("attest", "verify", "--envelope", env, "--key", pub,
		"--allow-builder", "someone-else"); code != 1 {
		t.Fatalf("builder gate: %d", code)
	}
	// Foreign key -> rejected.
	priv2 := filepath.Join(dir, "c.pem")
	pub2 := filepath.Join(dir, "d.pem")
	execute("attest", "genkey", "--private", priv2, "--public", pub2)
	if code, _, _ := execute("attest", "verify", "--envelope", env, "--key", pub2); code != 1 {
		t.Fatalf("foreign key: %d", code)
	}
	// Missing flags -> invalid.
	if code, _, _ := execute("attest", "sign", "--artifact", artifact); code != 5 {
		t.Fatalf("sign flags: %d", code)
	}
	if code, _, _ := execute("attest", "verify", "--envelope", env); code != 5 {
		t.Fatalf("verify flags: %d", code)
	}
	if code, _, _ := execute("attest", "genkey", "--private", priv); code != 5 {
		t.Fatalf("genkey flags: %d", code)
	}
	// Bad envelope bytes -> invalid.
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte("{oops"), 0o600)
	if code, _, _ := execute("attest", "verify", "--envelope", bad, "--key", pub); code != 5 {
		t.Fatalf("bad envelope: %d", code)
	}
}
