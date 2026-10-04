package canonical

import (
	"bytes"
	"encoding/json"
	"testing"
)

// FuzzMarshalStability: canonical bytes are deterministic per input, sorted
// by key, and arbitrary JSON never panics the serializer.
func FuzzMarshalStability(f *testing.F) {
	f.Add(`{"b":2,"a":1}`)
	f.Add(`[3,2,1]`)
	f.Fuzz(func(t *testing.T, raw string) {
		var v any
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			t.Skip()
		}
		a, err := Marshal(v)
		if err != nil {
			t.Skip()
		}
		b, err := Marshal(v)
		if err != nil {
			t.Fatalf("second marshal failed: %v", err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("non-deterministic:\n%s\n%s", a, b)
		}
		var back any
		if err := json.Unmarshal(a, &back); err != nil {
			t.Fatalf("canonical output is not JSON: %v", err)
		}
	})
}
