package cosign

import (
	"testing"
	"time"
)

// Zero values must fall back to safe defaults (never unbounded/zero).
func TestZeroValueDefaults(t *testing.T) {
	var p Provider
	if p.timeout() <= 0 || p.attempts() != 1 {
		t.Fatalf("provider defaults: %v %d", p.timeout(), p.attempts())
	}
	p.Timeout = -time.Second
	p.MaxAttempts = -3
	if p.timeout() <= 0 || p.attempts() != 1 {
		t.Fatal("negative values must fall back to defaults")
	}
	var r RekorClient
	if r.timeout() <= 0 {
		t.Fatal("rekor default timeout")
	}
	if r.client() == nil {
		t.Fatal("rekor default transport")
	}
	// Exported constructors exist so callers never hand-roll timeouts.
	if DefaultProvider().Bin != "cosign" {
		t.Fatal("default cosign bin")
	}
	if DefaultRekor().BaseURL != "https://rekor.sigstore.dev" {
		t.Fatal("default rekor url")
	}
}
