// Package rebuild executes isolated source rebuilds: fetch a pinned commit,
// produce a deterministic source archive, and hash it. Two workers archiving
// the same commit produce identical bytes; anything else is a real signal.
package rebuild

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/quorum/quorum/internal/runner"
)

// Spec describes one rebuild job.
type Spec struct {
	RepoURL string
	Commit  string
	Image   string // container image for isolated archiving ("" = host git)
	Timeout time.Duration
}

// FetchCommit materializes repoURL@commit into a fresh directory under parent.
// Only allowlisted https remotes are accepted (same SSRF rule as the CLI).
func FetchCommit(ctx context.Context, gitBin, parent, repoURL, commit string) (string, error) {
	if err := runner.ValidateRepoURL(repoURL); err != nil {
		return "", err
	}
	if commit == "" || len(commit) > 128 {
		return "", fmt.Errorf("INVALID_INPUT: bad commit %q", commit)
	}
	dir, err := os.MkdirTemp(parent, "quorum-src-*")
	if err != nil {
		return "", fmt.Errorf("OPERATIONAL: temp dir: %v", err)
	}
	steps := [][]string{
		{"init"},
		{"remote", "add", "origin", repoURL},
		{"fetch", "--depth", "1", "origin", commit},
		{"checkout", "FETCH_HEAD"},
	}
	for _, args := range steps {
		cmd := exec.CommandContext(ctx, gitBin, append([]string{"-C", dir}, args...)...)
		// Non-interactive, bounded fetches: a public repo must never trigger
		// a credential prompt (observed: GitHub username prompt on failed
		// SHA fetch). With prompting disabled the failure surfaces as an
		// error immediately instead of hanging a headless worker.
		cmd.Env = append(os.Environ(),
			"GIT_TERMINAL_PROMPT=0",
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=credential.helper",
			"GIT_CONFIG_VALUE_0=",
		)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("SOURCE_NOT_FOUND: fetch %s@%s: %v (%s)", repoURL, short(commit), err, strings.TrimSpace(stderr.String()))
		}
	}
	return dir, nil
}

// ArchiveDigest hashes `git archive <commit>` bytes (deterministic per tree).
func ArchiveDigest(ctx context.Context, gitBin, repoDir, commit string) (string, error) {
	cmd := exec.CommandContext(ctx, gitBin, "-C", repoDir, "archive", commit)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("BUILDER_FAILED: git archive: %v (%s)", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.Len() == 0 {
		return "", fmt.Errorf("BUILDER_FAILED: empty archive for %s", short(commit))
	}
	sum := sha256.Sum256(stdout.Bytes())
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// RunLocal fetches and archives on the host (used in tests and fallback).
func RunLocal(ctx context.Context, gitBin, parent string, spec Spec) (digest string, cleanup func(), err error) {
	dir, err := FetchCommit(ctx, gitBin, parent, spec.RepoURL, spec.Commit)
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(dir) }
	digest, err = ArchiveDigest(ctx, gitBin, dir, spec.Commit)
	if err != nil {
		return "", cleanup, err
	}
	return digest, cleanup, nil
}

// DockerArchiveDigest runs git archive inside a network-isolated container
// with the fetched tree bind-mounted read-only. The container never sees the
// network; source acquisition stays outside the sandbox.
func DockerArchiveDigest(ctx context.Context, dockerBin, image, repoDir, commit string) (string, error) {
	if image == "" {
		image = "golang:1.27.1-bookworm"
	}
	abs, err := filepath.Abs(repoDir)
	if err != nil {
		return "", fmt.Errorf("INVALID_INPUT: %v", err)
	}
	// NOTE: the `&&` chain is load-bearing (and POSIX, unlike pipefail which
	// dash rejects). A bare `git ... | sha256sum` reports only sha256sum's
	// status, so a failing git would hash empty stdin and every builder
	// would "agree" on sha256(e3b0c44…) — a fake unanimous quorum. With `&&`
	// any git failure fails the rebuild loudly instead.
	// safe.directory is scoped to this one-shot invocation over our own
	// read-only fetch (bind mounts trip git's dubious-ownership guard).
	script := fmt.Sprintf("git -c safe.directory=/src -C /src archive %s > /tmp/quorum-archive.tar && sha256sum /tmp/quorum-archive.tar", commit)
	cmd := exec.CommandContext(ctx, dockerBin, "run", "--rm", "--network", "none",
		"-v", abs+":/src:ro", image, "sh", "-c", script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("BUILDER_FAILED: isolated archive: %v (%s)", err, strings.TrimSpace(stderr.String()))
	}
	fields := strings.Fields(stdout.String())
	if len(fields) == 0 || len(fields[0]) != 64 {
		return "", fmt.Errorf("BUILDER_FAILED: bad sha256sum output %q", stdout.String())
	}
	return "sha256:" + fields[0], nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}
