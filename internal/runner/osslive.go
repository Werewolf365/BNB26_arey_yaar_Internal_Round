package runner

import (
	"context"
	"time"

	"github.com/quorum/quorum/services/ossrebuild"
	"github.com/quorum/quorum/services/policy"
)

// liveResult is the outcome of one live OSS Rebuild lookup for the verify
// pipeline. Evidence is only attached on SUPPORTED_AND_VERIFIED with a usable
// digest; every other state becomes a visible note (never a silent verdict).
type liveResult struct {
	state    string
	evidence *policy.Evidence
	note     string
	err      error
}

func liveOssEvidence(o VerifyOptions) liveResult {
	p := ossrebuild.DefaultProvider()
	if o.OssBin != "" {
		p.Bin = o.OssBin
	}
	p.Timeout = 100 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	res, err := p.Lookup(ctx, o.PkgRef)
	if err != nil {
		return liveResult{err: err}
	}
	if res.State != ossrebuild.StateVerified {
		return liveResult{state: res.State, note: res.State + ": " + res.Detail}
	}
	if res.UpstreamDigest == "" {
		return liveResult{state: ossrebuild.StateMalformed, note: "verified rebuild carried no digest"}
	}
	// The source commit is the rebuild's own claim. If it is empty we report
	// no commit at all (""), letting the source-match rule exclude the entry
	// with a reason rather than faking agreement with the pinned commit.
	return liveResult{
		state: res.State,
		evidence: &policy.Evidence{
			BuilderID: "oss-rebuild", IndependenceGroup: "oss-rebuild",
			SourceCommit: res.SourceCommit, ArtifactDigest: res.UpstreamDigest,
			SignatureValid: true, VerificationSource: "oss-rebuild",
		},
	}
}
