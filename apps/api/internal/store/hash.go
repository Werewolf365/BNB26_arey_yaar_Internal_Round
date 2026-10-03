package store

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/quorum/quorum/services/canonical"
)

// hashRecord mirrors the CLI audit chain: H(prev || canonical(payload+type)).
func hashRecord(prevHash, eventType string, payload map[string]any) (string, error) {
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

// VerifyChain checks linkage for a slice ordered oldest-first.
func VerifyChain(records []AuditRecord) (bool, int, string) {
	prev := "GENESIS"
	for i, r := range records {
		if r.PreviousHash != prev {
			return false, i, "previous hash mismatch"
		}
		recomputed, err := hashRecord(r.PreviousHash, r.EventType, r.Payload)
		if err != nil || recomputed != r.RecordHash {
			return false, i, "payload modified"
		}
		prev = r.RecordHash
	}
	return true, -1, ""
}
