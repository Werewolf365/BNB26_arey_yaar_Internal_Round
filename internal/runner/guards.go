// Package runner holds the CLI's testable business logic (no Cobra here).
package runner

import (
	"fmt"
	"net/url"
	"strings"
)

var allowedRepoHosts = map[string]bool{
	"github.com": true,
	"gitlab.com": true,
	"npmjs.com":  true,
	"pypi.org":   true,
	"crates.io":  true,
}

// ValidateRepoURL enforces https + allowlisted host (SSRF protection).
func ValidateRepoURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("INVALID_INPUT: repository URL must be https (got %q)", raw)
	}
	host := u.Hostname()
	if !allowedRepoHosts[host] {
		return fmt.Errorf("INVALID_INPUT: repository host %q is not allowlisted", host)
	}
	if u.User != nil {
		return fmt.Errorf("INVALID_INPUT: repository URL must not embed credentials")
	}
	return nil
}

// ValidateArtifactPath rejects NUL/control characters, shell metacharacters,
// and absurd lengths. Absolute paths and `..` segments are allowed: the CLI
// opens the path directly (no base-dir join), so `..` cannot escape anything.
// (Archive-extraction traversal is a separate builder-side guard, tested later.)
func ValidateArtifactPath(p string) error {
	if p == "" || len(p) > 512 {
		return fmt.Errorf("INVALID_INPUT: artifact path must be 1-512 chars")
	}
	for _, r := range p {
		if r == 0 || (r < 0x20) {
			return fmt.Errorf("INVALID_INPUT: artifact path has control characters")
		}
		switch r {
		case '"', '*', '?', '<', '>', '|', ';', '&', '$', '`':
			return fmt.Errorf("INVALID_INPUT: artifact path has unsafe characters")
		}
	}
	return nil
}

// CheckSize enforces an explicit artifact size cap.
func CheckSize(sizeBytes, maxBytes int64) error {
	if sizeBytes < 0 {
		return fmt.Errorf("INVALID_INPUT: negative artifact size")
	}
	if sizeBytes > maxBytes {
		return fmt.Errorf("ARTIFACT_OVERSIZED: %d bytes exceeds cap of %d", sizeBytes, maxBytes)
	}
	return nil
}

// ParsePackageRef splits ecosystem:name@version; ok=false when not a ref.
func ParsePackageRef(s string) (ecosystem, name, version string, ok bool) {
	for _, prefix := range []string{"npm:", "pypi:", "cargo:"} {
		if !strings.HasPrefix(s, prefix) {
			continue
		}
		rest := strings.TrimPrefix(s, prefix)
		at := strings.LastIndex(rest, "@")
		if at <= 0 || at == len(rest)-1 {
			return "", "", "", false
		}
		return strings.TrimSuffix(prefix, ":"), rest[:at], rest[at+1:], true
	}
	return "", "", "", false
}

// OssFixture is the deterministic fixture-mode OSS Rebuild lookup.
// Live mode is intentionally NOT implemented here: real OSS Rebuild calls
// need ADC credentials and belong to test-external, never to the CLI default path.
func OssFixture(pkgRef string) (state, detail string) {
	if pkgRef == "pypi:quorum-tiny@1.0.0" {
		return "SUPPORTED_AND_VERIFIED", "fixture: deterministic tiny package"
	}
	if eco, _, _, ok := ParsePackageRef(pkgRef); ok {
		_ = eco
		return "UNSUPPORTED", "fixture: package not in OSS Rebuild rebuilt subset"
	}
	return "NOT_FOUND", "unknown ecosystem prefix; want npm:|pypi:|cargo:"
}
