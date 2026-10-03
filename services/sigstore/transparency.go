// Package sigstore transparency: a minimal hash-chained log interface with a
// file-backed DEV implementation. This is a development stand-in for
// Rekor-style transparency: it proves inclusion mechanics and is NEVER
// presented as production transparency (no quorum of witnesses, no
// consistency proofs — see docs/attestations.md).
package sigstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Entry is one transparency record binding a statement hash to a key id.
type Entry struct {
	Index         int    `json:"index"`
	StatementHash string `json:"statementHash"`
	KeyID         string `json:"keyId"`
	PrevHash      string `json:"prevHash"`
	EntryHash     string `json:"entryHash"`
}

// TransparencyLog is the production seam (Rekor adapter plugs in here).
type TransparencyLog interface {
	Append(statementHash, keyID string) (Entry, error)
	Verify(e Entry) error
}

// FileLog appends JSONL entries to a file (dev only).
type FileLog struct {
	mu   sync.Mutex
	path string
}

func NewFileLog(path string) *FileLog { return &FileLog{path: path} }

func (f *FileLog) head() (Entry, bool) {
	raw, err := os.ReadFile(f.path)
	if err != nil || len(raw) == 0 {
		return Entry{}, false
	}
	lines := splitLines(raw)
	var last Entry
	for _, ln := range lines {
		var e Entry
		if json.Unmarshal([]byte(ln), &e) == nil && e.EntryHash != "" {
			last = e
		}
	}
	if last.EntryHash == "" {
		return Entry{}, false
	}
	return last, true
}

func splitLines(raw []byte) []string {
	var out []string
	cur := ""
	for _, c := range raw {
		if c == '\n' {
			out = append(out, cur)
			cur = ""
		} else if c != '\r' {
			cur += string(c)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (f *FileLog) Append(statementHash, keyID string) (Entry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prev := "GENESIS"
	index := 0
	if last, ok := f.head(); ok {
		prev = last.EntryHash
		index = last.Index + 1
	}
	sum := sha256.Sum256([]byte(prev + "\n" + statementHash + "\n" + keyID))
	e := Entry{Index: index, StatementHash: statementHash, KeyID: keyID, PrevHash: prev, EntryHash: hex.EncodeToString(sum[:])}
	raw, _ := json.Marshal(e)
	fh, err := os.OpenFile(f.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Entry{}, fmt.Errorf("OPERATIONAL: %v", err)
	}
	defer fh.Close()
	if _, err := fh.Write(append(raw, '\n')); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// Verify recomputes the entry hash and its linkage against file history.
func (f *FileLog) Verify(e Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	sum := sha256.Sum256([]byte(e.PrevHash + "\n" + e.StatementHash + "\n" + e.KeyID))
	if hex.EncodeToString(sum[:]) != e.EntryHash {
		return fmt.Errorf("AUDIT_TAMPERED: entry hash mismatch at index %d", e.Index)
	}
	if e.Index == 0 {
		if e.PrevHash != "GENESIS" {
			return fmt.Errorf("AUDIT_TAMPERED: bad genesis link")
		}
		return nil
	}
	raw, err := os.ReadFile(f.path)
	if err != nil {
		return err
	}
	for _, ln := range splitLines(raw) {
		var prev Entry
		if json.Unmarshal([]byte(ln), &prev) == nil && prev.Index == e.Index-1 {
			if prev.EntryHash != e.PrevHash {
				return fmt.Errorf("AUDIT_TAMPERED: broken link at index %d", e.Index)
			}
			return nil
		}
	}
	return fmt.Errorf("AUDIT_TAMPERED: predecessor %d not found", e.Index-1)
}
