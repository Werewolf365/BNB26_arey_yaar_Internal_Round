// Package policy implements the deterministic Quorum evaluation engine.
//
// Same evidence + same policy = same decision. No trust scores, only
// transparent predicates. Builders sharing an independence_group are never
// counted as independent.
package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// Decisions per prompt section 5.
const (
	DecisionVerified         = "VERIFIED"
	DecisionVerifiedConflict = "VERIFIED_WITH_CONFLICT"
	DecisionInsufficient     = "INSUFFICIENT_EVIDENCE"
	DecisionRejected         = "REJECTED"
	DecisionInvestigate      = "INVESTIGATE"
	DecisionError            = "ERROR"
)

// Policy mirrors packages/schemas/policy.json.
type Policy struct {
	PolicyID                   string   `json:"policyId"`
	Version                    string   `json:"version"`
	MinBuilders                int      `json:"minBuilders"`
	RequiredAgreement          int      `json:"requiredAgreement"`
	RequiredIndependentGroups  int      `json:"requiredIndependentGroups"`
	RequireSourceMatch         bool     `json:"requireSourceMatch"`
	RequireAllSignaturesValid  bool     `json:"requireAllSignaturesValid"`
	ConflictTolerance          int      `json:"conflictTolerance"`
	AllowedVerificationSources []string `json:"allowedVerificationSources"`
}

// Evidence mirrors packages/schemas/evidence.json.
type Evidence struct {
	BuilderID          string `json:"builderId"`
	IndependenceGroup  string `json:"independenceGroup"`
	SourceCommit       string `json:"sourceCommit"`
	ArtifactDigest     string `json:"artifactDigest"`
	SignatureValid     bool   `json:"signatureValid"`
	VerificationSource string `json:"verificationSource"`
}

// Conflict describes one minority digest group.
type Conflict struct {
	Digest   string   `json:"digest"`
	Builders []string `json:"builders"`
}

// ExcludedItem is one evidence input the evaluator did not count, with the
// exact reason. Never silently dropped: every exclusion is reported.
type ExcludedItem struct {
	Evidence Evidence `json:"evidence"`
	Reason   string   `json:"reason"`
}

// Result mirrors packages/schemas/decision.json.
type Result struct {
	Decision         string         `json:"decision"`
	Required         int            `json:"required"`
	Satisfied        int            `json:"satisfied"`
	Conflicts        []Conflict     `json:"conflicts"`
	CountedEvidence  []Evidence     `json:"countedEvidence"`
	ExcludedEvidence []ExcludedItem `json:"excludedEvidence"`
	Reasons          []string       `json:"reasons"`
}

// Validate the policy itself (never trust an invalid policy).
func (p Policy) Validate() error {
	if p.MinBuilders < 1 {
		return fmt.Errorf("INVALID_POLICY: minBuilders must be >= 1")
	}
	if p.RequiredAgreement < 1 {
		return fmt.Errorf("INVALID_POLICY: requiredAgreement must be >= 1")
	}
	if p.RequiredIndependentGroups < 1 {
		return fmt.Errorf("INVALID_POLICY: requiredIndependentGroups must be >= 1")
	}
	if p.RequiredAgreement > p.MinBuilders {
		return fmt.Errorf("INVALID_POLICY: requiredAgreement cannot exceed minBuilders")
	}
	if p.ConflictTolerance < 0 {
		return fmt.Errorf("INVALID_POLICY: conflictTolerance cannot be negative")
	}
	return nil
}

// Evaluate is deterministic: sort inputs, group by digest, enforce
// independence-group uniqueness, source match, signature validity.
func Evaluate(policy Policy, expectedSource string, evidence []Evidence) Result {
	res := Result{Required: policy.RequiredAgreement, Conflicts: []Conflict{}, CountedEvidence: []Evidence{}, Reasons: []string{}}
	if err := policy.Validate(); err != nil {
		res.Decision = DecisionError
		res.Reasons = append(res.Reasons, err.Error())
		return res
	}
	allowed := map[string]bool{}
	for _, s := range policy.AllowedVerificationSources {
		allowed[s] = true
	}
	// Deterministic order.
	sorted := append([]Evidence(nil), evidence...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].ArtifactDigest != sorted[j].ArtifactDigest {
			return sorted[i].ArtifactDigest < sorted[j].ArtifactDigest
		}
		return sorted[i].BuilderID < sorted[j].BuilderID
	})
	// Filter + dedupe: one vote per independence group per digest handled below.
	seenBuilder := map[string]bool{}
	var eligible []Evidence
	for _, e := range sorted {
		if seenBuilder[e.BuilderID] {
			res.ExcludedEvidence = append(res.ExcludedEvidence, ExcludedItem{e, "duplicate builder id: evidence not double-counted"})
			continue
		}
		seenBuilder[e.BuilderID] = true
		if len(allowed) > 0 && !allowed[e.VerificationSource] {
			res.ExcludedEvidence = append(res.ExcludedEvidence, ExcludedItem{e, "verification source not allowed by policy"})
			continue
		}
		if policy.RequireSourceMatch && e.SourceCommit != expectedSource {
			res.ExcludedEvidence = append(res.ExcludedEvidence, ExcludedItem{e, "source commit mismatch"})
			continue
		}
		if policy.RequireAllSignaturesValid && !e.SignatureValid {
			res.ExcludedEvidence = append(res.ExcludedEvidence, ExcludedItem{e, "invalid signature (policy requires all signatures valid)"})
			continue
		}
		eligible = append(eligible, e)
	}
	// Group by digest; within each digest keep one vote per independence group.
	type groupKey struct{ digest, group string }
	used := map[groupKey]string{} // groupKey -> builderId
	digestGroups := map[string][]Evidence{}
	digestGroupSet := map[string]map[string]bool{}
	for _, e := range eligible {
		k := groupKey{e.ArtifactDigest, e.IndependenceGroup}
		if prev, ok := used[k]; ok {
			res.ExcludedEvidence = append(res.ExcludedEvidence, ExcludedItem{e, fmt.Sprintf("same independence_group '%s' already counted by '%s': not independent", e.IndependenceGroup, prev)})
			continue
		}
		used[k] = e.BuilderID
		digestGroups[e.ArtifactDigest] = append(digestGroups[e.ArtifactDigest], e)
		if digestGroupSet[e.ArtifactDigest] == nil {
			digestGroupSet[e.ArtifactDigest] = map[string]bool{}
		}
		digestGroupSet[e.ArtifactDigest][e.IndependenceGroup] = true
	}
	if len(eligible) < policy.MinBuilders {
		res.Decision = DecisionInsufficient
		res.Reasons = append(res.Reasons, fmt.Sprintf("only %d eligible builders, need %d", len(eligible), policy.MinBuilders))
		return res
	}
	// Find best digest by independent-group count (deterministic tiebreak: digest asc).
	digests := make([]string, 0, len(digestGroups))
	for d := range digestGroups {
		digests = append(digests, d)
	}
	sort.Strings(digests)
	best := ""
	bestCount := 0
	for _, d := range digests {
		if c := len(digestGroupSet[d]); c > bestCount {
			bestCount = c
			best = d
		}
	}
	res.Satisfied = bestCount
	// Surface minority digests as conflicts (never hide disagreement).
	for _, d := range digests {
		if d == best {
			continue
		}
		builders := []string{}
		for _, e := range digestGroups[d] {
			builders = append(builders, e.BuilderID)
		}
		sort.Strings(builders)
		res.Conflicts = append(res.Conflicts, Conflict{Digest: d, Builders: builders})
	}
	if best != "" {
		res.CountedEvidence = digestGroups[best]
		sort.Slice(res.CountedEvidence, func(i, j int) bool {
			return res.CountedEvidence[i].BuilderID < res.CountedEvidence[j].BuilderID
		})
	}
	numConflicts := len(res.Conflicts)
	switch {
	case bestCount >= policy.RequiredAgreement && bestCount >= policy.RequiredIndependentGroups && numConflicts <= policy.ConflictTolerance && numConflicts == 0:
		res.Decision = DecisionVerified
		res.Reasons = append(res.Reasons, fmt.Sprintf("%d independent groups agree on %s", bestCount, shortDigest(best)))
	case bestCount >= policy.RequiredAgreement && bestCount >= policy.RequiredIndependentGroups && numConflicts <= policy.ConflictTolerance:
		res.Decision = DecisionVerifiedConflict
		res.Reasons = append(res.Reasons, fmt.Sprintf("%d independent groups agree on %s with %d conflicting group(s) tolerated", bestCount, shortDigest(best), numConflicts))
	case bestCount < policy.RequiredAgreement || bestCount < policy.RequiredIndependentGroups:
		// Distinguish conflict-driven investigation from plain insufficiency.
		if numConflicts > 0 && bestCount > 0 {
			res.Decision = DecisionInvestigate
			res.Reasons = append(res.Reasons, "no digest reaches required agreement; conflicting evidence requires investigation")
		} else {
			res.Decision = DecisionInsufficient
			res.Reasons = append(res.Reasons, fmt.Sprintf("best agreement %d below required %d", bestCount, policy.RequiredAgreement))
		}
	default:
		res.Decision = DecisionInvestigate
		res.Reasons = append(res.Reasons, fmt.Sprintf("conflicts (%d) exceed tolerance (%d)", numConflicts, policy.ConflictTolerance))
	}
	return res
}

func shortDigest(d string) string {
	if len(d) > 16 {
		return d[:16] + "..."
	}
	return d
}

// PolicyHash is a transparent content hash of the canonical policy (for anchoring).
func PolicyHash(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:])
}
