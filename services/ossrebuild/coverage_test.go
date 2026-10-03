package ossrebuild_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/quorum/quorum/services/ossrebuild"
)

func stubOss(t *testing.T) ossrebuild.Provider {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "ossstub")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, filepath.Join("testdata", "ossstub", "main.go"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v\n%s", err, out)
	}
	return ossrebuild.Provider{Bin: bin, Timeout: 10 * time.Second, MaxAttempts: 1}
}

func TestLookupVerified(t *testing.T) {
	p := stubOss(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := p.Probe(ctx); err != nil {
		t.Fatalf("stub must probe: %v", err)
	}
	got, err := p.Lookup(ctx, "pypi:absl-py@2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != ossrebuild.StateVerified {
		t.Fatalf("want VERIFIED: %+v", got)
	}
	if got.UpstreamDigest != "sha256:9a28abb62774ae4e8edbe2dd4c49ffcd45a6a848952a5eccc6a49f3f0fc1e2f3" {
		t.Fatalf("digest: %q", got.UpstreamDigest)
	}
	if got.SourceCommit != "37dad4d356ca9e13f1c533ad6309631b397a2b6b" {
		t.Fatalf("commit: %q", got.SourceCommit)
	}
	if got.Artifact == "" || got.RebuiltAt == "" || len(got.Raw) == 0 {
		t.Fatalf("fields must be populated: %+v", got)
	}
}

func TestLookupNotFound(t *testing.T) {
	p := stubOss(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := p.Lookup(ctx, "pypi:absl-py@0.0.0-bogus-quorum")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != ossrebuild.StateNotFound {
		t.Fatalf("want NOT_FOUND: %+v", got)
	}
}

func TestLookupFailed(t *testing.T) {
	p := stubOss(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := p.Lookup(ctx, "pypi:absl-py@9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != ossrebuild.StateFailed {
		t.Fatalf("want FAILED: %+v", got)
	}
}

func TestLookupTimeout(t *testing.T) {
	p := ossrebuild.Provider{Bin: "oss-rebuild-definitely-missing", Timeout: time.Second, MaxAttempts: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	got, err := p.Lookup(ctx, "pypi:absl-py@2.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != ossrebuild.StateUnavailable {
		t.Fatalf("missing binary must be UNAVAILABLE: %+v", got)
	}
}
