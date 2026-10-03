package policy_test

import (
	"strings"
	"testing"

	"github.com/quorum/quorum/services/policy"
)

func TestPolicyHash(t *testing.T) {
	h1 := policy.PolicyHash([]byte(`{"policyId":"a"}`))
	if !strings.HasPrefix(h1, "sha256:") || len(h1) != 7+64 {
		t.Fatalf("hash shape: %q", h1)
	}
	if h2 := policy.PolicyHash([]byte(`{"policyId":"a"}`)); h1 != h2 {
		t.Fatal("hash must be deterministic")
	}
	if h3 := policy.PolicyHash([]byte(`{"policyId":"b"}`)); h1 == h3 {
		t.Fatal("hash must change with content")
	}
}

func TestValidateBranches(t *testing.T) {
	good := basePolicy()
	if err := good.Validate(); err != nil {
		t.Fatalf("base policy must validate: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*policy.Policy)
	}{
		{"minBuilders", func(p *policy.Policy) { p.MinBuilders = 0 }},
		{"requiredAgreement", func(p *policy.Policy) { p.RequiredAgreement = 0 }},
		{"groups", func(p *policy.Policy) { p.RequiredIndependentGroups = 0 }},
		{"agreementExceedsMin", func(p *policy.Policy) { p.RequiredAgreement = p.MinBuilders + 1 }},
		{"negativeTolerance", func(p *policy.Policy) { p.ConflictTolerance = -1 }},
	}
	for _, tc := range cases {
		p := basePolicy()
		tc.mutate(&p)
		if err := p.Validate(); err == nil {
			t.Fatalf("%s must fail validation", tc.name)
		}
	}
}

func TestEvaluateErrorAndInvestigate(t *testing.T) {
	bad := basePolicy()
	bad.MinBuilders = 0
	res := policy.Evaluate(bad, "c", nil)
	if res.Decision != policy.DecisionError {
		t.Fatalf("invalid policy must yield ERROR, got %s", res.Decision)
	}
	// No digest reaches agreement with conflicts present -> INVESTIGATE.
	p := basePolicy()
	p.RequiredAgreement = 2
	p.MinBuilders = 2
	ev := []policy.Evidence{
		{BuilderID: "a", IndependenceGroup: "g1", SourceCommit: "c", ArtifactDigest: "sha256:" + strings.Repeat("a", 64), SignatureValid: true, VerificationSource: "builder"},
		{BuilderID: "b", IndependenceGroup: "g2", SourceCommit: "c", ArtifactDigest: "sha256:" + strings.Repeat("b", 64), SignatureValid: true, VerificationSource: "builder"},
	}
	if res := policy.Evaluate(p, "c", ev); res.Decision != policy.DecisionInvestigate {
		t.Fatalf("split evidence must INVESTIGATE, got %s", res.Decision)
	}
	// Conflicts beyond tolerance -> INVESTIGATE.
	p2 := basePolicy() // 2-of-3, tolerance 1
	p2.ConflictTolerance = 0
	ev3 := []policy.Evidence{
		{BuilderID: "a", IndependenceGroup: "g1", SourceCommit: "c", ArtifactDigest: "sha256:" + strings.Repeat("a", 64), SignatureValid: true, VerificationSource: "builder"},
		{BuilderID: "b", IndependenceGroup: "g2", SourceCommit: "c", ArtifactDigest: "sha256:" + strings.Repeat("a", 64), SignatureValid: true, VerificationSource: "builder"},
		{BuilderID: "c", IndependenceGroup: "g3", SourceCommit: "c", ArtifactDigest: "sha256:" + strings.Repeat("b", 64), SignatureValid: true, VerificationSource: "builder"},
	}
	if res := policy.Evaluate(p2, "c", ev3); res.Decision != policy.DecisionInvestigate {
		t.Fatalf("over-tolerance conflict must INVESTIGATE, got %s", res.Decision)
	}
	// Empty evidence -> INSUFFICIENT_EVIDENCE.
	if res := policy.Evaluate(basePolicy(), "c", nil); res.Decision != policy.DecisionInsufficient {
		t.Fatalf("empty evidence must be INSUFFICIENT, got %s", res.Decision)
	}
}

func TestEvaluateExclusions(t *testing.T) {
	mk := func(id, group, commit, digest string, sig bool, src string) policy.Evidence {
		return policy.Evidence{BuilderID: id, IndependenceGroup: group, SourceCommit: commit,
			ArtifactDigest: digest, SignatureValid: sig, VerificationSource: src}
	}
	d := "sha256:" + strings.Repeat("a", 64)
	// Same independence group counts once: two builders, one group cannot
	// satisfy a 2-group policy.
	p := basePolicy()
	p.RequiredAgreement = 2
	p.MinBuilders = 2
	p.RequiredIndependentGroups = 2
	ev := []policy.Evidence{mk("a", "g", "c", d, true, "builder"), mk("b", "g", "c", d, true, "builder")}
	res := policy.Evaluate(p, "c", ev)
	if len(res.ExcludedEvidence) == 0 {
		t.Fatal("same-group evidence must be excluded with a reason")
	}
	if res.Decision == policy.DecisionVerified {
		t.Fatalf("one group must not VERIFY a 2-group policy: %s", res.Decision)
	}
	// Invalid signature excluded when the policy requires valid sigs.
	ev2 := []policy.Evidence{mk("a", "g1", "c", d, true, "builder"), mk("b", "g2", "c", d, false, "builder")}
	if res := policy.Evaluate(p, "c", ev2); len(res.ExcludedEvidence) == 0 {
		t.Fatal("bad signature must be excluded")
	}
	// Source mismatch excluded.
	ev3 := []policy.Evidence{mk("a", "g1", "c", d, true, "builder"), mk("b", "g2", "OTHER", d, true, "builder")}
	if res := policy.Evaluate(p, "c", ev3); len(res.ExcludedEvidence) == 0 {
		t.Fatal("source mismatch must be excluded")
	}
	// Disallowed verification source excluded.
	p4 := basePolicy()
	p4.AllowedVerificationSources = []string{"builder"}
	p4.RequiredAgreement = 1
	p4.MinBuilders = 1
	p4.RequiredIndependentGroups = 1
	ev4 := []policy.Evidence{mk("a", "g1", "c", d, true, "oss-rebuild")}
	if res := policy.Evaluate(p4, "c", ev4); len(res.ExcludedEvidence) == 0 || res.Decision != policy.DecisionInsufficient {
		t.Fatalf("disallowed source must be excluded: %+v", res)
	}
	// Duplicate builder id counted once.
	ev5 := []policy.Evidence{mk("a", "g1", "c", d, true, "builder"), mk("a", "g2", "c", d, true, "builder")}
	if res := policy.Evaluate(p4, "c", ev5); len(res.ExcludedEvidence) == 0 {
		t.Fatal("duplicate builder must be excluded")
	}
}
