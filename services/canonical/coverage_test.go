package canonical_test

import (
	"testing"

	"github.com/quorum/quorum/services/canonical"
)

func TestMarshalUnencodable(t *testing.T) {
	if _, err := canonical.Marshal(func() {}); err == nil {
		t.Fatal("func values must fail canonical marshal")
	}
	if _, err := canonical.Marshal(make(chan int)); err == nil {
		t.Fatal("channels must fail canonical marshal")
	}
}

func TestMarshalNestedDeterminism(t *testing.T) {
	a, err := canonical.Marshal(map[string]any{"z": 1, "a": []any{3, 2, map[string]any{"k": "v"}}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonical.Marshal(map[string]any{"a": []any{3, 2, map[string]any{"k": "v"}}, "z": 1})
	if err != nil || string(a) != string(b) {
		t.Fatalf("nested order must not matter: %s %s", a, b)
	}
}
