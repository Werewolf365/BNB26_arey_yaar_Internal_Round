package policy

import (
	"encoding/json"
	"testing"
)

// FuzzEvaluateDeterminism: same evidence + same policy = same decision,
// and arbitrary inputs never panic.
func FuzzEvaluateDeterminism(f *testing.F) {
	f.Add(`{"policyId":"p","version":"v1","minBuilders":2,"requiredAgreement":2,"requiredIndependentGroups":2,"requireSourceMatch":true,"requireAllSignaturesValid":true,"conflictTolerance":1,"allowedVerificationSources":["builder"]}`,
		"abc123",
		`[{"builderId":"a","independenceGroup":"ga","sourceCommit":"abc123","artifactDigest":"sha256:aa","signatureValid":true,"verificationSource":"builder"}]`)
	f.Fuzz(func(t *testing.T, policyJSON, src, evidenceJSON string) {
		var p Policy
		if err := json.Unmarshal([]byte(policyJSON), &p); err != nil {
			t.Skip()
		}
		var ev []Evidence
		if err := json.Unmarshal([]byte(evidenceJSON), &ev); err != nil {
			t.Skip()
		}
		if err := p.Validate(); err != nil {
			t.Skip()
		}
		r1 := Evaluate(p, src, ev)
		r2 := Evaluate(p, src, ev)
		b1, _ := json.Marshal(r1)
		b2, _ := json.Marshal(r2)
		if string(b1) != string(b2) {
			t.Fatalf("non-deterministic:\n%s\n%s", b1, b2)
		}
	})
}

// FuzzIndependenceInvariant: two builders in the same independence group can
// never satisfy a policy requiring two independent groups.
func FuzzIndependenceInvariant(f *testing.F) {
	f.Add("aa", "abc123")
	f.Fuzz(func(t *testing.T, digest, src string) {
		p := Policy{PolicyID: "p", Version: "v1", MinBuilders: 2, RequiredAgreement: 2,
			RequiredIndependentGroups: 2, RequireSourceMatch: false,
			ConflictTolerance: 1, AllowedVerificationSources: []string{"builder"}}
		ev := []Evidence{
			{BuilderID: "a", IndependenceGroup: "same", SourceCommit: src, ArtifactDigest: digest, SignatureValid: true, VerificationSource: "builder"},
			{BuilderID: "b", IndependenceGroup: "same", SourceCommit: src, ArtifactDigest: digest, SignatureValid: true, VerificationSource: "builder"},
		}
		r := Evaluate(p, src, ev)
		if r.Decision == DecisionVerified {
			t.Fatalf("same-group pair satisfied 2-group policy: %+v", r)
		}
	})
}

// FuzzConflictVisibility: conflicting evidence must never disappear silently —
// total counted + excluded must account for every input.
func FuzzConflictVisibility(f *testing.F) {
	f.Add("abc123", `[{"builderId":"a","independenceGroup":"ga","sourceCommit":"abc123","artifactDigest":"sha256:aa","signatureValid":true,"verificationSource":"builder"}]`)
	f.Fuzz(func(t *testing.T, src, evidenceJSON string) {
		var ev []Evidence
		if err := json.Unmarshal([]byte(evidenceJSON), &ev); err != nil {
			t.Skip()
		}
		p := Policy{PolicyID: "p", Version: "v1", MinBuilders: 1, RequiredAgreement: 1,
			RequiredIndependentGroups: 1, ConflictTolerance: 100,
			AllowedVerificationSources: []string{"builder"}}
		r := Evaluate(p, src, ev)
		if len(r.CountedEvidence)+len(r.ExcludedEvidence) != len(ev) {
			t.Fatalf("evidence lost: in=%d counted=%d excluded=%d", len(ev), len(r.CountedEvidence), len(r.ExcludedEvidence))
		}
	})
}
