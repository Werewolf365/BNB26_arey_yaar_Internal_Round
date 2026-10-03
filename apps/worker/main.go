// Command worker executes builder rebuild jobs from the Quorum API queue.
//
// Loop: claim a job -> fetch source identity -> isolated rebuild
// (network-none container, read-only source mount) -> report digest.
// The worker never decides: it reports bytes, the quorum engine decides.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"

	"github.com/quorum/quorum/apps/worker/internal/client"
	"github.com/quorum/quorum/apps/worker/internal/rebuild"
)

func main() {
	api := flag.String("api", "http://localhost:8080", "Quorum API base URL")
	owner := flag.String("owner", "worker-1", "worker identity (lease owner)")
	poll := flag.Duration("poll", 5*time.Second, "queue poll interval")
	once := flag.Bool("once", false, "execute a single job then exit")
	timeout := flag.Duration("timeout", 10*time.Minute, "per-job rebuild timeout")
	image := flag.String("image", "golang:1.27.1-bookworm", "sandbox image for isolated archiving")
	dockerBin := flag.String("docker", "docker", "docker binary")
	gitBin := flag.String("git", "git", "git binary (source acquisition only)")
	workdir := flag.String("workdir", os.TempDir(), "scratch parent for fetched sources")
	allowFallback := flag.Bool("allow-host-fallback", false, "ONLY for dev: archive on host when no Docker daemon (logged, never default)")
	flag.Parse()

	c := client.New(*api, *owner)
	for {
		done, stop := runOnce(c, *timeout, *image, *dockerBin, *gitBin, *workdir, *allowFallback)
		_ = done
		if stop || *once {
			return
		}
		time.Sleep(*poll)
	}
}

// runOnce claims and executes one job; stop=true means exit the loop.
func runOnce(c *client.Client, timeout time.Duration, image, dockerBin, gitBin, workdir string, allowFallback bool) (bool, bool) {
	job, claimed, err := c.Claim()
	if err != nil {
		log.Printf("owner=%s claim failed: %v", c.Owner, err)
		return false, false
	}
	if !claimed {
		log.Printf("owner=%s queue empty", c.Owner)
		return false, false
	}
	log.Printf("owner=%s claimed job=%s builder=%s verification=%s (attempt %d)", c.Owner, job.ID, job.BuilderID, job.VerificationID, job.Attempts)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	repo, commit, err := c.ReleaseForVerification(job.VerificationID)
	if err != nil {
		_ = c.Complete(job.ID, false, "", "", "SOURCE_NOT_FOUND", err.Error())
		return true, false
	}
	// Source acquisition (network) happens outside the sandbox; the rebuild
	// itself runs network-isolated with a read-only mount.
	dir, err := rebuild.FetchCommit(ctx, gitBin, workdir, repo, commit)
	if err != nil {
		_ = c.Complete(job.ID, false, "", "", firstCode(err), err.Error())
		return true, false
	}
	defer func() { _ = os.RemoveAll(dir) }()
	digest, err := rebuild.DockerArchiveDigest(ctx, dockerBin, image, dir, commit)
	if err != nil {
		if !allowFallback {
			_ = c.Complete(job.ID, false, "", "", "BUILDER_FAILED", "sandbox unavailable and host fallback disabled: "+err.Error())
			return true, false
		}
		// Dev-only fallback: logged, and the digest still flows through the
		// quorum engine like any other evidence (never auto-trusted).
		log.Printf("owner=%s DEV host fallback: %v", c.Owner, err)
		if digest, err = rebuild.ArchiveDigest(ctx, gitBin, dir, commit); err != nil {
			_ = c.Complete(job.ID, false, "", "", firstCode(err), err.Error())
			return true, false
		}
	}
	if err := c.Complete(job.ID, true, digest, commit, "", ""); err != nil {
		log.Printf("owner=%s complete failed: %v", c.Owner, err)
		return true, false
	}
	log.Printf("owner=%s job=%s done digest=%s", c.Owner, job.ID, digest)
	return true, false
}

func firstCode(err error) string {
	s := err.Error()
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return s[:i]
		}
	}
	return "BUILDER_FAILED"
}
