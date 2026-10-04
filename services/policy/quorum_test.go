package policy_test

import (
	"testing"

	"github.com/quorum/quorum/services/policy"
)

func basePolicy() policy.Policy {
	return policy.Policy{
		PolicyID: "policy-a", Version: "v1",
		MinBuilders: 2, RequiredAgreement: 2, RequiredIndependentGroups: 2,
		RequireSourceMatch: true, RequireAllSignaturesValid: true,
		ConflictTolerance:          1,
		AllowedVerificationSources: []string{"builder", "oss-rebuild"},
	}
}

func ev(builder, group, digest string) policy.Evidence {
	return policy.Evidence{
		BuilderID: builder, IndependenceGroup: group,
		SourceCommit: "abc123", ArtifactDigest: digest,
		SignatureValid: true, VerificationSource: "builder",
	}
}

// Determinism: same evidence + same policy = same decision (order-independent).
func TestDeterminism(t *testing.T) {
	p := basePolicy()
	d := "sha256:" + "ab"
	for i := 0; i < 64; i++ {
		d = "sha256:abc"
	}
	_ = d
	a := []policy.Evidence{ev("builder-b", "cloud-b", "sha256:abc"), ev("builder-a", "cloud-a", "sha256:abc")}
	b := []policy.Evidence{ev("builder-a", "cloud-a", "sha256:abc"), ev("builder-b", "cloud-b", "sha256:abc")}
	ra := policy.Evaluate(p, "abc123", a)
	rb := policy.Evaluate(p, "abc123", b)
	if ra.Decision != rb.Decision || ra.Satisfied != rb.Satisfied {
		t.Fatalf("non-deterministic: %+v vs %+v", ra, rb)
	}
	if ra.Decision != policy.DecisionVerified {
		t.Fatalf("expected VERIFIED, got %s (%v)", ra.Decision, ra.Reasons)
	}
}

// Independence: two builders in the same group cannot satisfy 2-group policy.
func TestSameGroupNotIndependent(t *testing.T) {
	p := basePolicy()
	got := policy.Evaluate(p, "abc123", []policy.Evidence{
		ev("builder-1", "aws", "sha256:abc"),
		ev("builder-2", "aws", "sha256:abc"),
	})
	if got.Decision == policy.DecisionVerified || got.Decision == policy.DecisionVerifiedConflict {
		t.Fatalf("same-group builders must not verify: %+v", got)
	}
}

// 2/3 with conflict -> VERIFIED_WITH_CONFLICT.
func TestConflictVisible(t *testing.T) {
	p := basePolicy()
	got := policy.Evaluate(p, "abc123", []policy.Evidence{
		ev("builder-a", "cloud-a", "sha256:abc"),
		ev("builder-b", "cloud-b", "sha256:abc"),
		{BuilderID: "builder-c", IndependenceGroup: "cloud-c", SourceCommit: "abc123", ArtifactDigest: "sha256:def", SignatureValid: true, VerificationSource: "builder"},
	})
	if got.Decision != policy.DecisionVerifiedConflict {
		t.Fatalf("expected VERIFIED_WITH_CONFLICT, got %s", got.Decision)
	}
	if len(got.Conflicts) != 1 {
		t.Fatalf("minority evidence must be surfaced, got %+v", got.Conflicts)
	}
}

// Insufficient quorum.
func TestInsufficient(t *testing.T) {
	p := basePolicy()
	got := policy.Evaluate(p, "abc123", []policy.Evidence{ev("builder-a", "cloud-a", "sha256:abc")})
	if got.Decision != policy.DecisionInsufficient {
		t.Fatalf("expected INSUFFICIENT_EVIDENCE, got %s", got.Decision)
	}
}

// Duplicate builder counted once.
func TestDuplicateBuilder(t *testing.T) {
	p := basePolicy()
	dup := ev("builder-a", "cloud-a", "sha256:abc")
	got := policy.Evaluate(p, "abc123", []policy.Evidence{dup, dup})
	if len(got.ExcludedEvidence) == 0 {
		t.Fatalf("duplicate evidence must be excluded with reason")
	}
}

// Malformed evidence (empty id/group/digest) is excluded with a reason and
// can never satisfy a quorum — found by FuzzConflictVisibility.
func TestMalformedEvidenceExcluded(t *testing.T) {
	p := basePolicy()
	got := policy.Evaluate(p, "abc123", []policy.Evidence{
		{BuilderID: "", IndependenceGroup: "cloud-a", SourceCommit: "abc123", ArtifactDigest: "sha256:abc", SignatureValid: true, VerificationSource: "builder"},
		{BuilderID: "builder-b", IndependenceGroup: "", SourceCommit: "abc123", ArtifactDigest: "sha256:abc", SignatureValid: true, VerificationSource: "builder"},
		{BuilderID: "builder-c", IndependenceGroup: "cloud-c", SourceCommit: "abc123", ArtifactDigest: "", SignatureValid: true, VerificationSource: "builder"},
	})
	if got.Decision == policy.DecisionVerified || got.Decision == policy.DecisionVerifiedConflict {
		t.Fatalf("malformed evidence must never verify, got %s", got.Decision)
	}
	if len(got.CountedEvidence)+len(got.ExcludedEvidence) != 3 {
		t.Fatalf("all malformed inputs must be accounted for: %+v", got)
	}
}

// Tiebreak: equal group counts resolve to the lowest digest (deterministic).
// Kills the `>` -> `>=` mutant which would hand victory to the last digest.
func TestTiebreakLowestDigestWins(t *testing.T) {
	p := basePolicy()
	p.MinBuilders = 4
	d1, d2 := "sha256:aaa", "sha256:zzz"
	got := policy.Evaluate(p, "abc123", []policy.Evidence{
		ev("builder-a", "cloud-a", d1),
		ev("builder-b", "cloud-b", d1),
		ev("builder-c", "cloud-c", d2),
		ev("builder-d", "cloud-d", d2),
	})
	if len(got.CountedEvidence) != 2 || got.CountedEvidence[0].ArtifactDigest != d1 {
		t.Fatalf("tie must resolve to lowest digest, got %+v", got.CountedEvidence)
	}
	if len(got.Conflicts) != 1 || got.Conflicts[0].Digest != d2 {
		t.Fatalf("loser must surface as conflict, got %+v", got.Conflicts)
	}
}

// Zero conflicts with tolerance 0 is plain VERIFIED, not WITH_CONFLICT.
// Kills the `<=` -> `<` mutant on the no-conflict branch.
func TestZeroConflictToleranceZeroIsVerified(t *testing.T) {
	p := basePolicy()
	p.ConflictTolerance = 0
	got := policy.Evaluate(p, "abc123", []policy.Evidence{
		ev("builder-a", "cloud-a", "sha256:abc"),
		ev("builder-b", "cloud-b", "sha256:abc"),
	})
	if got.Decision != policy.DecisionVerified {
		t.Fatalf("expected VERIFIED, got %s (%v)", got.Decision, got.Reasons)
	}
}
