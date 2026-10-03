package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/quorum/quorum/services/canonical"
)

// AuditRecord is one hash-chained record: recordHash = H(prev || canonical(payload)).
type AuditRecord struct {
	Index        int            `json:"index"`
	EventType    string         `json:"eventType"`
	Payload      map[string]any `json:"payload"`
	PreviousHash string         `json:"previousHash"`
	RecordHash   string         `json:"recordHash"`
}

// HashRecord computes the record hash.
func HashRecord(prevHash string, eventType string, payload map[string]any) (string, error) {
	envelope := map[string]any{"eventType": eventType}
	for k, v := range payload {
		envelope[k] = v
	}
	canon, err := canonical.Marshal(envelope)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(prevHash))
	h.Write(canon)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyChain checks the whole chain; returns (ok, brokenIndex, reason).
func VerifyChain(chain []AuditRecord) (bool, int, string) {
	prev := "GENESIS"
	for i, r := range chain {
		if r.PreviousHash != prev {
			return false, i, fmt.Sprintf("previous hash mismatch at record %d (want %s, got %s)", i, prev, r.PreviousHash)
		}
		if r.EventType == "" {
			return false, i, fmt.Sprintf("record %d has empty event type", i)
		}
		recomputed, err := HashRecord(r.PreviousHash, r.EventType, r.Payload)
		if err != nil {
			return false, i, fmt.Sprintf("record %d unhashable: %v", i, err)
		}
		if recomputed != r.RecordHash {
			return false, i, fmt.Sprintf("payload modified at record %d", i)
		}
		prev = r.RecordHash
	}
	return true, -1, ""
}

// LoadAuditChain reads a JSON array chain file.
func LoadAuditChain(path string) ([]AuditRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("INVALID_INPUT: cannot read audit file: %v", err)
	}
	var chain []AuditRecord
	if err := json.Unmarshal(data, &chain); err != nil {
		return nil, fmt.Errorf("INVALID_INPUT: malformed audit JSON: %v", err)
	}
	return chain, nil
}

// DemoChain builds a small valid chain for `audit verify --demo`.
func DemoChain() ([]AuditRecord, error) {
	events := []struct {
		typ     string
		payload map[string]any
	}{
		{"release.created", map[string]any{"releaseId": "r1"}},
		{"builder.completed", map[string]any{"builderId": "builder-a"}},
		{"quorum.evaluated", map[string]any{"decision": "VERIFIED"}},
	}
	var chain []AuditRecord
	prev := "GENESIS"
	for i, e := range events {
		h, err := HashRecord(prev, e.typ, e.payload)
		if err != nil {
			return nil, err
		}
		chain = append(chain, AuditRecord{Index: i, EventType: e.typ, Payload: e.payload, PreviousHash: prev, RecordHash: h})
		prev = h
	}
	return chain, nil
}
