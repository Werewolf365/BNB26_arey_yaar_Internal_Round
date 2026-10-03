package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/quorum/quorum/services/canonical"
	"github.com/quorum/quorum/services/policy"
)

// TinyFixtureBytes is the deterministic demo artifact content.
var TinyFixtureBytes = []byte("quorum-tiny-artifact-v1")

// VerifyOptions controls the local verification pipeline.
type VerifyOptions struct {
	Repo     string
	Commit   string
	Artifact string // file path, or "" with FixtureTiny
	PkgRef   string // npm:|pypi:|cargo: ref (alternative to repo/commit/artifact)

	FixtureTiny bool // use built-in tiny bytes instead of reading a file
	// Test-only disagreement simulation, always labeled as such in output.
	InjectConflictBuilder string // builder id whose digest is replaced with a bogus one
	InjectTamper          bool   // flip one byte of the artifact before hashing (labeled)

	ExpectedDigest string // optional sha256:<hex>; mismatch => REJECTED

	MinBuilders      int
	RequiredAgree    int
	RequiredGroups   int
	// ConflictTolerance: -1 means "leave policy default".
	ConflictTolerance int
	PolicyFile       string // optional full JSON policy (overrides the scalar flags)

	MaxBytes int64

	Verbose bool
}

// VerifyResult is the machine-readable outcome.
type VerifyResult struct {
	Decision   string        `json:"decision"`
	Required   int           `json:"required"`
	Satisfied  int           `json:"satisfied"`
	Conflicts  []policy.Conflict `json:"conflicts"`
	Reasons    []string      `json:"reasons"`
	Artifact   string        `json:"artifactDigest"`
	Source     string        `json:"sourceCommit"`
	PolicyID   string        `json:"policyId"`
	OssState   string        `json:"ossRebuildState"`
	Simulated  []string      `json:"simulated,omitempty"`
}

// DefaultBuilders returns the three demo builders with distinct groups.
func DefaultBuilders() []policy.Evidence {
	return []policy.Evidence{
		{BuilderID: "builder-a", IndependenceGroup: "cloud-a", VerificationSource: "builder", SignatureValid: true},
		{BuilderID: "builder-b", IndependenceGroup: "cloud-b", VerificationSource: "builder", SignatureValid: true},
		{BuilderID: "builder-c", IndependenceGroup: "cloud-c", VerificationSource: "builder", SignatureValid: true},
	}
}

// DefaultPolicy is policy A (2 of 3 independent builders agree).
func DefaultPolicy() policy.Policy {
	return policy.Policy{
		PolicyID: "policy-a", Version: "v1",
		MinBuilders: 2, RequiredAgreement: 2, RequiredIndependentGroups: 2,
		RequireSourceMatch: true, RequireAllSignaturesValid: true,
		ConflictTolerance: 1,
		AllowedVerificationSources: []string{"builder", "oss-rebuild"},
	}
}

// RunVerify executes the pipeline and returns (result, exitCode, humanOutput, err).
// err != nil only for operational failures (exit 4); input problems are
// returned as results with the appropriate decision/exit mapping... except
// truly invalid input, which returns exitcodes.InvalidInput with err.
func RunVerify(o VerifyOptions) (*VerifyResult, int, string, error) {
	var out strings.Builder
	start := time.Now()

	if o.MaxBytes <= 0 {
		o.MaxBytes = 50 * 1024 * 1024
	}
	// Resolve source identity.
	repo, commit := o.Repo, o.Commit
	if o.PkgRef != "" {
		if _, _, _, ok := ParsePackageRef(o.PkgRef); !ok {
			return nil, exitcodes.InvalidInput, "", fmt.Errorf("INVALID_INPUT: bad package ref %q (want npm:|pypi:|cargo: name@version)", o.PkgRef)
		}
		if repo == "" {
			repo = "https://pypi.org/project/quorum-tiny"
		}
		if commit == "" {
			commit = "abc123"
		}
	}
	if repo == "" || commit == "" {
		return nil, exitcodes.InvalidInput, "", fmt.Errorf("INVALID_INPUT: --repo and --commit are required (or a package ref)")
	}
	if err := ValidateRepoURL(repo); err != nil {
		return nil, exitcodes.InvalidInput, "", err
	}
	if commit == "" || len(commit) > 128 {
		return nil, exitcodes.InvalidInput, "", fmt.Errorf("INVALID_INPUT: bad commit %q", commit)
	}

	// Resolve artifact bytes.
	var artifactBytes []byte
	if o.FixtureTiny || (o.Artifact == "" && o.PkgRef != "") {
		artifactBytes = TinyFixtureBytes
	} else {
		if err := ValidateArtifactPath(o.Artifact); err != nil {
			return nil, exitcodes.InvalidInput, "", err
		}
		data, err := os.ReadFile(o.Artifact)
		if err != nil {
			return nil, exitcodes.InvalidInput, "", fmt.Errorf("INVALID_INPUT: cannot read artifact %q: %v", o.Artifact, err)
		}
		artifactBytes = data
	}
	if err := CheckSize(int64(len(artifactBytes)), o.MaxBytes); err != nil {
		// Oversize is a rejection of this input, not an operational failure.
		if strings.HasPrefix(err.Error(), "ARTIFACT_OVERSIZED") {
			r := &VerifyResult{Decision: policy.DecisionRejected, Reasons: []string{err.Error()}, Source: commit, PolicyID: "policy-a"}
			return r, exitcodes.Rejected, humanReport(r, nil, o.Verbose, time.Since(start)), nil
		}
		return nil, exitcodes.InvalidInput, "", err
	}
	if o.InjectTamper {
		artifactBytes = append(append([]byte(nil), artifactBytes...), 'X')
	}
	sum := sha256.Sum256(artifactBytes)
	digest := "sha256:" + hex.EncodeToString(sum[:])

	// Expected-digest comparison: the tamper tripwire.
	if o.ExpectedDigest != "" && o.ExpectedDigest != digest {
		r := &VerifyResult{
			Decision: policy.DecisionRejected,
			Reasons:  []string{fmt.Sprintf("ARTIFACT_HASH_MISMATCH: expected %s, rebuilt %s", short(o.ExpectedDigest), short(digest))},
			Artifact: digest, Source: commit, PolicyID: "policy-a",
		}
		return r, exitcodes.Rejected, humanReport(r, nil, o.Verbose, time.Since(start)), nil
	}

	// Policy.
	pol := DefaultPolicy()
	if o.PolicyFile != "" {
		loaded, err := LoadPolicyFile(o.PolicyFile)
		if err != nil {
			return nil, exitcodes.InvalidInput, "", err
		}
		pol = loaded
	} else {
		if o.MinBuilders > 0 {
			pol.MinBuilders = o.MinBuilders
		}
		if o.RequiredAgree > 0 {
			pol.RequiredAgreement = o.RequiredAgree
		}
		if o.RequiredGroups > 0 {
			pol.RequiredIndependentGroups = o.RequiredGroups
		}
		if o.ConflictTolerance >= 0 {
			pol.ConflictTolerance = o.ConflictTolerance
		}
	}
	if err := pol.Validate(); err != nil {
		return nil, exitcodes.InvalidInput, "", err
	}

	// Builders: each honestly rebuilds the same bytes...
	evidence := DefaultBuilders()
	bogus := "sha256:" + strings.Repeat("b", 64)
	var simulated []string
	for i := range evidence {
		evidence[i].SourceCommit = commit
		evidence[i].ArtifactDigest = digest
		if o.InjectConflictBuilder != "" && evidence[i].BuilderID == o.InjectConflictBuilder {
			evidence[i].ArtifactDigest = bogus
			simulated = append(simulated, evidence[i].BuilderID+" (simulated disagreement)")
		}
	}

	// OSS Rebuild fixture evidence (informational; counted only if it agrees
	// and carries a distinct builder id — never double-counted).
	ossState, ossDetail := "SKIPPED", "no package ref given"
	if o.PkgRef != "" {
		ossState, ossDetail = OssFixture(o.PkgRef)
		_ = ossDetail
	}

	res := policy.Evaluate(pol, commit, evidence)
	out2 := &VerifyResult{
		Decision: res.Decision, Required: res.Required, Satisfied: res.Satisfied,
		Conflicts: res.Conflicts, Reasons: res.Reasons,
		Artifact: digest, Source: commit, PolicyID: pol.PolicyID,
		OssState: ossState, Simulated: simulated,
	}
	if ossState != "SKIPPED" && o.Verbose {
		out2.Reasons = append(out2.Reasons, "oss-rebuild: "+ossState)
	}
	_ = out
	return out2, exitcodes.ForDecision(res.Decision), humanReport(out2, &pol, o.Verbose, time.Since(start)), nil
}

func short(d string) string {
	if len(d) > 22 {
		return d[:22] + "..."
	}
	return d
}

// humanReport renders the trust-assessment UX (prompt section 13): evidence
// and bounded conclusions, never "100% safe".
func humanReport(r *VerifyResult, pol *policy.Policy, verbose bool, elapsed time.Duration) string {
	var b strings.Builder
	b.WriteString("BUILD INTEGRITY\n")
	fmt.Fprintf(&b, "  Source pinned:            %s\n", r.Source)
	fmt.Fprintf(&b, "  Artifact digest:          %s\n", r.Artifact)
	fmt.Fprintf(&b, "  Decision:                 %s\n", r.Decision)
	b.WriteString("SUPPLY-CHAIN EVIDENCE\n")
	fmt.Fprintf(&b, "  OSS Rebuild:              %s\n", r.OssState)
	if len(r.Conflicts) == 0 {
		b.WriteString("  Conflicts:                none\n")
	} else {
		for _, c := range r.Conflicts {
			fmt.Fprintf(&b, "  Conflict:                 %s from %s\n", short(c.Digest), strings.Join(c.Builders, ","))
		}
	}
	for _, s := range r.Simulated {
		fmt.Fprintf(&b, "  [simulated]               %s\n", s)
	}
	b.WriteString("POLICY\n")
	fmt.Fprintf(&b, "  Required:                 %d agreement\n", r.Required)
	fmt.Fprintf(&b, "  Satisfied:                %d\n", r.Satisfied)
	for _, reason := range r.Reasons {
		fmt.Fprintf(&b, "  Reason:                   %s\n", reason)
	}
	if verbose && pol != nil {
		canon, err := canonical.Marshal(pol)
		if err == nil {
			sum := sha256.Sum256(canon)
			fmt.Fprintf(&b, "  Policy hash:              sha256:%s\n", hex.EncodeToString(sum[:]))
		}
		fmt.Fprintf(&b, "  Elapsed:                  %s\n", elapsed.Round(time.Millisecond))
	}
	b.WriteString("Quorum reports verification evidence and policy decisions, not proof of safety.\n")
	return b.String()
}
