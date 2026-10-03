package canonical_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/quorum/quorum/services/canonical"
)

func loadVectors(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	// services/canonical -> ../.. is the repo root.
	path := filepath.Join("..", "..", "packages", "fixtures", "conformance.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot load conformance vectors: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestCanonicalVectors(t *testing.T) {
	doc := loadVectors(t)
	var cases []struct {
		Input any    `json:"input"`
		Want  string `json:"want"`
	}
	if err := json.Unmarshal(doc["canonicalCases"], &cases); err != nil {
		t.Fatal(err)
	}
	for i, tc := range cases {
		got, err := canonical.Marshal(tc.Input)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if string(got) != tc.Want {
			t.Fatalf("case %d: want %s, got %s", i, tc.Want, got)
		}
		// Idempotence: canonical(canonical(x)) == canonical(x).
		var reparsed any
		if err := json.Unmarshal(got, &reparsed); err != nil {
			t.Fatal(err)
		}
		again, _ := canonical.Marshal(reparsed)
		if string(again) != tc.Want {
			t.Fatalf("case %d not idempotent", i)
		}
	}
}
