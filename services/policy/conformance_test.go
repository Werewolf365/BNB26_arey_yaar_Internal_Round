package policy_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/quorum/quorum/services/policy"
)

// TestConformanceVectors asserts the shared fixtures in
// packages/fixtures/conformance.json. The Node mirror asserts the same file;
// any behavioral drift between implementations fails one of the suites.
func TestConformanceVectors(t *testing.T) {
	path := filepath.Join("..", "..", "packages", "fixtures", "conformance.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot load conformance vectors: %v", err)
	}
	// Decode with explicit field names matching the fixture file.
	var raw struct {
		QuorumCases []struct {
			Name           string           `json:"name"`
			Policy         policy.Policy    `json:"policy"`
			ExpectedSource string           `json:"expectedSource"`
			Evidence       []policy.Evidence `json:"evidence"`
			WantDecision   string           `json:"wantDecision"`
			WantSatisfied  int              `json:"wantSatisfied"`
			WantConflicts  int              `json:"wantConflicts"`
		} `json:"quorumCases"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw.QuorumCases) == 0 {
		t.Fatal("no quorum cases loaded")
	}
	for _, tc := range raw.QuorumCases {
		got := policy.Evaluate(tc.Policy, tc.ExpectedSource, tc.Evidence)
		if got.Decision != tc.WantDecision {
			t.Errorf("%s: decision want %s got %s (%v)", tc.Name, tc.WantDecision, got.Decision, got.Reasons)
		}
		if got.Satisfied != tc.WantSatisfied {
			t.Errorf("%s: satisfied want %d got %d", tc.Name, tc.WantSatisfied, got.Satisfied)
		}
		if len(got.Conflicts) != tc.WantConflicts {
			t.Errorf("%s: conflicts want %d got %d", tc.Name, tc.WantConflicts, len(got.Conflicts))
		}
		// Determinism under input permutation.
		rev := append([]policy.Evidence(nil), tc.Evidence...)
		for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
			rev[i], rev[j] = rev[j], rev[i]
		}
		again := policy.Evaluate(tc.Policy, tc.ExpectedSource, rev)
		if again.Decision != got.Decision || again.Satisfied != got.Satisfied {
			t.Errorf("%s: not order-independent", tc.Name)
		}
	}
}
