package server

import (
	"errors"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	if (Config{}).maxBody() != DefaultMaxBodyBytes {
		t.Fatal("default body cap")
	}
	if (Config{MaxBodyBytes: 16}).maxBody() != 16 {
		t.Fatal("custom body cap")
	}
	if (Config{}).burst() != 10 {
		t.Fatal("default burst")
	}
	if (Config{RateBurst: 3}).burst() != 3 {
		t.Fatal("custom burst")
	}
	if got := requestID(httptest.NewRequest("GET", "/", nil)); got != "" {
		t.Fatalf("bare request must have no id, got %q", got)
	}
	plain := httptest.NewRequest("GET", "/", nil)
	plain.RemoteAddr = "nodomain"
	if got := clientIP(plain); got != "nodomain" {
		t.Fatalf("portless addr must pass through, got %q", got)
	}
}

func TestLimiterInternals(t *testing.T) {
	now := time.Now()
	l := newRateLimiter(1000, 2)
	// Overflow refill clamps at burst, never above.
	l.buckets["ip"] = &bucket{tokens: 0, lastSeen: now.Add(-time.Second)}
	if !l.allow("ip", now) {
		t.Fatal("refilled bucket must allow")
	}
	if l.buckets["ip"].tokens > 2 {
		t.Fatalf("tokens must clamp at burst: %v", l.buckets["ip"].tokens)
	}
	// Cleanup evicts minute-old buckets once the map is oversized.
	l2 := newRateLimiter(1, 1)
	l2.buckets["old"] = &bucket{tokens: 1, lastSeen: now.Add(-2 * time.Minute)}
	for i := 0; i < 4096; i++ {
		l2.buckets["k"+strconv.Itoa(i)] = &bucket{tokens: 1, lastSeen: now}
	}
	l2.allow("fresh", now)
	if _, ok := l2.buckets["old"]; ok {
		t.Fatal("stale bucket must be evicted by cleanup")
	}
}

// TestClassifyContract locks the docs/api.md error-code mapping, including
// codes no current handler emits (kept for the contract).
func TestClassifyContract(t *testing.T) {
	cases := []struct {
		err    error
		code   string
		status int
	}{
		{errors.New("INVALID_INPUT: x"), "INVALID_INPUT", 400},
		{errors.New("INVALID_POLICY: x"), "INVALID_POLICY", 400},
		{errors.New("UNAUTHENTICATED: x"), "UNAUTHENTICATED", 401},
		{errors.New("FORBIDDEN: x"), "FORBIDDEN", 403},
		{errors.New("RELEASE_NOT_FOUND: x"), "RELEASE_NOT_FOUND", 404},
		{errors.New("EVIDENCE_NOT_FOUND: x"), "EVIDENCE_NOT_FOUND", 404},
		{errors.New("POLICY_CONFLICT: x"), "POLICY_CONFLICT", 409},
		{errors.New("JOB_CONFLICT: x"), "JOB_CONFLICT", 409},
		{errors.New("RATE_LIMITED: x"), "RATE_LIMITED", 429},
		{errors.New("INSUFFICIENT_EVIDENCE: x"), "INSUFFICIENT_EVIDENCE", 422},
		{errors.New("POLICY_VIOLATION: x"), "POLICY_VIOLATION", 422},
		{errors.New("OSS_REBUILD_UNAVAILABLE: x"), "OSS_REBUILD_UNAVAILABLE", 502},
		{errors.New("BLOCKCHAIN_UNAVAILABLE: x"), "BLOCKCHAIN_UNAVAILABLE", 502},
		{errors.New("BUILDER_TIMEOUT: x"), "BUILDER_TIMEOUT", 504},
		{errors.New("ARTIFACT_OVERSIZED: x"), "ARTIFACT_OVERSIZED", 422},
		{errors.New("ATTESTATION_INVALID: x"), "ATTESTATION_INVALID", 422},
		{errors.New("SIGNATURE_INVALID: x"), "SIGNATURE_INVALID", 422},
		{errors.New("SIGNER_NOT_TRUSTED: x"), "SIGNER_NOT_TRUSTED", 422},
		{errors.New("BUILDER_FAILED: x"), "BUILDER_FAILED", 422},
		{errors.New("AUDIT_TAMPERED: x"), "AUDIT_TAMPERED", 422},
		{errors.New("BLOCKCHAIN_REVERTED: x"), "BLOCKCHAIN_REVERTED", 422},
		{errors.New("OSS_REBUILD_UNSUPPORTED: x"), "OSS_REBUILD_UNSUPPORTED", 422},
		{errors.New("boom"), "INTERNAL_ERROR", 500},
	}
	for _, tc := range cases {
		code, msg, status := classify(tc.err)
		if code != tc.code || status != tc.status || msg == "" {
			t.Fatalf("%v: got %s/%d/%q", tc.err, code, status, msg)
		}
	}
}
