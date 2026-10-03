package runner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/quorum/quorum/internal/runner"
)

func TestHashRecordDeterministic(t *testing.T) {
	h1, err := runner.HashRecord("GENESIS", "release.created", map[string]any{"releaseId": "r1"})
	if err != nil || h1 == "" {
		t.Fatal("must hash")
	}
	h2, _ := runner.HashRecord("GENESIS", "release.created", map[string]any{"releaseId": "r1"})
	if h1 != h2 {
		t.Fatal("must be deterministic")
	}
	h3, _ := runner.HashRecord("GENESIS", "release.created", map[string]any{"releaseId": "r2"})
	if h1 == h3 {
		t.Fatal("payload change must change hash")
	}
	h4, _ := runner.HashRecord("OTHER", "release.created", map[string]any{"releaseId": "r1"})
	if h1 == h4 {
		t.Fatal("prev-hash change must change hash")
	}
}

func TestVerifyChainEdges(t *testing.T) {
	if ok, _, _ := runner.VerifyChain(nil); !ok {
		t.Fatal("empty chain is vacuously valid")
	}
	chain, err := runner.DemoChain()
	if err != nil {
		t.Fatal(err)
	}
	if ok, at, _ := runner.VerifyChain(chain); !ok || at != -1 {
		t.Fatal("demo chain must verify")
	}
	// Empty event type rejected.
	bad := append([]runner.AuditRecord(nil), chain...)
	bad[0].EventType = ""
	if ok, at, _ := runner.VerifyChain(bad); ok || at != 0 {
		t.Fatal("empty event type must fail at 0")
	}
	// Reordered records break links.
	swapped := []runner.AuditRecord{chain[1], chain[0], chain[2]}
	if ok, _, _ := runner.VerifyChain(swapped); ok {
		t.Fatal("reordered chain must fail")
	}
}

func TestLoadAuditChainRoundTrip(t *testing.T) {
	dir := t.TempDir()
	chain, _ := runner.DemoChain()
	// Write via JSON manually to prove the file format is stable.
	p := filepath.Join(dir, "chain.json")
	f, _ := os.Create(p)
	f.WriteString(`[`)
	for i, r := range chain {
		if i > 0 {
			f.WriteString(`,`)
		}
		// Minimal record encoding the loader must accept.
		f.WriteString(`{"index":` + itoa(r.Index) + `,"eventType":` + q(r.EventType) + `,"payload":{"releaseId":"r1"},"previousHash":` + q(r.PreviousHash) + `,"recordHash":` + q(r.RecordHash) + `}`)
	}
	f.WriteString(`]`)
	f.Close()
	loaded, err := runner.LoadAuditChain(p)
	if err != nil {
		t.Fatal(err)
	}
	// Payloads differ per record in the demo chain; just check linkage loads.
	if len(loaded) != 3 {
		t.Fatalf("want 3 records, got %d", len(loaded))
	}
	if _, err := runner.LoadAuditChain(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing file must error")
	}
	os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{oops`), 0o600)
	if _, err := runner.LoadAuditChain(filepath.Join(dir, "bad.json")); err == nil {
		t.Fatal("malformed JSON must error")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

func q(s string) string { return `"` + s + `"` }
