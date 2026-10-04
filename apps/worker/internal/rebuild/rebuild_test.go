package rebuild_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/quorum/quorum/apps/worker/internal/rebuild"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
}

// seedRepo builds a local source repo with two commits; returns dir + head sha.
func seedRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "test@quorum.local")
	git(t, dir, "config", "user.name", "test")
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello quorum\n"), 0o600)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "one")
	out, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	return dir, string(out[:40])
}

func TestArchiveDeterministic(t *testing.T) {
	dir, head := seedRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, err := rebuild.ArchiveDigest(ctx, "git", dir, head)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rebuild.ArchiveDigest(ctx, "git", dir, head)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("same commit must archive identically")
	}
	// New commit changes the digest.
	os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello quorum!\n"), 0o600)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "two")
	out, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	c, err := rebuild.ArchiveDigest(ctx, "git", dir, string(out[:40]))
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Fatal("different commit must differ")
	}
	// Unknown commit fails loudly.
	if _, err := rebuild.ArchiveDigest(ctx, "git", dir, "deadbeef"); err == nil {
		t.Fatal("bad commit must fail")
	}
}

func TestFetchRejectsBadRemote(t *testing.T) {	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := rebuild.FetchCommit(ctx, "git", t.TempDir(), "https://evil.example/x", "abc"); err == nil {
		t.Fatal("non-allowlisted remote must be rejected before any network")
	}
	if _, err := rebuild.FetchCommit(ctx, "git", t.TempDir(), "https://github.com/example/tiny", ""); err == nil {
		t.Fatal("empty commit must be rejected")
	}
}

// Unreachable SHA on a real public repo must fail fast with an error — never
// hang on a credential prompt (every fetch runs with GIT_TERMINAL_PROMPT=0).
// Needs network: gated like the other live tests.
func TestFetchUnreachableShaFailsFast(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_NETWORK") != "1" {
		t.Skip("set QUORUM_LIVE_NETWORK=1 with github.com reachable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	start := time.Now()
	_, err := rebuild.FetchCommit(ctx, "git", t.TempDir(), "https://github.com/tukaani-project/xz.git", "ffffffffffffffffffffffffffffffffffffffff")
	if err == nil {
		t.Fatal("unreachable SHA must fail")
	}
	if time.Since(start) > 80*time.Second {
		t.Fatal("fetch must not hang waiting for credentials")
	}
}

// TestDockerArchiveFailsLoudly is the pipefail regression test: a failing
// git must surface as BUILDER_FAILED, never as unanimous agreement on
// sha256(e3b0c44…) — the hash of empty stdin. Env-gated like its neighbor.
func TestDockerArchiveFailsLoudly(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_DOCKER") != "1" {
		t.Skip("set QUORUM_LIVE_DOCKER=1 with a Docker daemon")
	}
	dir, _ := seedRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	d, err := rebuild.DockerArchiveDigest(ctx, "docker", "", dir, "deadbeefdoesnotexist")
	if err == nil {
		t.Fatalf("bogus commit must fail loudly, got %s", d)
	}
	if d == "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatal("empty-stdin digest must never be reported")
	}
}

// TestDockerArchiveDeterministic needs a daemon (env-gated).
func TestDockerArchiveDeterministic(t *testing.T) {
	if os.Getenv("QUORUM_LIVE_DOCKER") != "1" {
		t.Skip("set QUORUM_LIVE_DOCKER=1 with a Docker daemon")
	}
	dir, head := seedRepo(t) // local dir; bind-mounted read-only into the sandbox
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	a, err := rebuild.DockerArchiveDigest(ctx, "docker", "", dir, head)
	if err != nil {
		t.Fatalf("isolated archive: %v", err)
	}
	b, err := rebuild.DockerArchiveDigest(ctx, "docker", "", dir, head)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("sandboxed rebuilds must match")
	}
	// NOTE (proven live): host git (2.51) vs container git (2.39.5) archives of
	// the SAME commit differ (tar/pax encoding drift across implementations).
	// Comparability requires homogeneous builder toolchains — recorded in
	// docs/builders.md. The engine never assumes cross-toolchain equality.
	t.Logf("sandbox digest: %s", a)
}
