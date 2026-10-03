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

func TestFetchRejectsBadRemote(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := rebuild.FetchCommit(ctx, "git", t.TempDir(), "https://evil.example/x", "abc"); err == nil {
		t.Fatal("non-allowlisted remote must be rejected before any network")
	}
	if _, err := rebuild.FetchCommit(ctx, "git", t.TempDir(), "https://github.com/example/tiny", ""); err == nil {
		t.Fatal("empty commit must be rejected")
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
