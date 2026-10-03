// Package ossrebuild is the OSS Rebuild evidence adapter (prompt section 6).
//
// It shells the real `oss-rebuild` CLI (install: go install
// github.com/google/oss-rebuild/cmd/oss-rebuild@latest; ecosystems on the
// wire: npm, pypi, cratesio) and normalizes results into Quorum states.
// Reads need no credentials (verified live); signature verification is on by
// default inside the CLI. Deterministic fixture mode stays the default for
// tests and offline runs; live mode is explicit and env-gated.
package ossrebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// States (adapter contract; also mirrored in services/oss-rebuild/adapter.mjs).
const (
	StateVerified    = "SUPPORTED_AND_VERIFIED"
	StateFailed       = "SUPPORTED_BUT_FAILED"
	StateUnsupported  = "UNSUPPORTED"
	StateNotFound     = "NOT_FOUND"
	StateUnavailable  = "UNAVAILABLE"
	StateMalformed    = "MALFORMED_EVIDENCE"
)

// Result is the normalized lookup outcome. Raw holds the untouched CLI
// payload bytes for forensic preservation.
type Result struct {
	State          string `json:"state"`
	Ecosystem      string `json:"ecosystem"`
	Package        string `json:"package"`
	Version        string `json:"version"`
	Artifact       string `json:"artifact"`
	UpstreamDigest string `json:"upstreamDigest"` // sha256:<hex> of the upstream artifact
	SourceCommit   string `json:"sourceCommit"`   // pinned by the rebuild Dockerfile, if found
	RebuiltAt      string `json:"rebuiltAt"`
	Detail         string `json:"detail"`
	Raw            []byte `json:"-"`
}

// Provider shells the CLI. Bin is the executable path; Timeout bounds each
// call; MaxAttempts allows one safe retry (reads are idempotent).
type Provider struct {
	Bin         string
	Timeout     time.Duration
	MaxAttempts int
}

// DefaultProvider uses PATH lookup with sane bounds.
func DefaultProvider() Provider {
	return Provider{Bin: "oss-rebuild", Timeout: 90 * time.Second, MaxAttempts: 2}
}

// MapRef converts npm:/pypi:/cargo: refs to CLI ecosystems.
func MapRef(pkgRef string) (ecosystem, name, version string, err error) {
	var eco string
	switch {
	case strings.HasPrefix(pkgRef, "npm:"):
		eco, pkgRef = "npm", strings.TrimPrefix(pkgRef, "npm:")
	case strings.HasPrefix(pkgRef, "pypi:"):
		eco, pkgRef = "pypi", strings.TrimPrefix(pkgRef, "pypi:")
	case strings.HasPrefix(pkgRef, "cargo:"):
		eco, pkgRef = "cratesio", strings.TrimPrefix(pkgRef, "cargo:")
	default:
		return "", "", "", fmt.Errorf("UNSUPPORTED: unknown ecosystem in %q (want npm:|pypi:|cargo:)", pkgRef)
	}
	at := strings.LastIndex(pkgRef, "@")
	if at <= 0 || at == len(pkgRef)-1 {
		return "", "", "", fmt.Errorf("INVALID_INPUT: bad package ref (want name@version)")
	}
	return eco, pkgRef[:at], pkgRef[at+1:], nil
}

var (
	digestRe = regexp.MustCompile(`Upstream target digest:\s*\{"sha256":"([0-9a-f]{64})"\}`)
	commitRe = regexp.MustCompile(`git checkout --force '([0-9a-f]{5,128})'`)
	builtRe  = regexp.MustCompile(`Rebuilt at:\s*"([^"]+)"`)
	noteRe   = regexp.MustCompile(`artifact is being inferred as "([^"]+)"`)
)

// run executes the CLI with timeout + one safe retry. Both streams are
// returned: human markers live on stderr ("Rebuild found!", inference
// NOTEs), machine payloads on stdout. Callers pick the right one.
func (p Provider) run(ctx context.Context, args ...string) (stdout, stderr string, err error) {
	if p.Timeout <= 0 {
		p.Timeout = 90 * time.Second
	}
	attempts := p.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for i := 0; i < attempts; i++ {
		cctx, cancel := context.WithTimeout(ctx, p.Timeout)
		cmd := exec.CommandContext(cctx, p.Bin, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		cancel()
		if err == nil {
			return stdout.String(), stderr.String(), nil
		}
		if ctx.Err() != nil {
			return "", "", fmt.Errorf("UNAVAILABLE: %v", ctx.Err())
		}
		last = fmt.Errorf("%v: %s", err, strings.TrimSpace(stdout.String()+"\n"+stderr.String()))
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	// Raw error (no UNAVAILABLE prefix): only timeouts carry that taxonomy,
	// so Lookup can still distinguish FAILED from UNAVAILABLE.
	return "", "", last
}

// Probe checks the binary executes (exit 0 on --help).
func (p Provider) Probe(ctx context.Context) error {
	if _, _, err := p.run(ctx, "--help"); err != nil {
		return fmt.Errorf("UNAVAILABLE: oss-rebuild binary missing or broken: %v", err)
	}
	return nil
}

// Lookup resolves one package version into a normalized Result.
func (p Provider) Lookup(ctx context.Context, pkgRef string) (Result, error) {
	eco, name, version, err := MapRef(pkgRef)
	if err != nil {
		msg := err.Error()
		if strings.HasPrefix(msg, "UNSUPPORTED") {
			return Result{State: StateUnsupported, Detail: msg}, nil
		}
		return Result{State: StateMalformed, Detail: msg}, nil
	}
	res := Result{Ecosystem: eco, Package: name, Version: version}
	stdout, stderr, err := p.run(ctx, "get", eco, name, version, "--output=summary")
	if err != nil {
		text := err.Error()
		switch {
		case strings.Contains(text, "file does not exist"):
			res.State = StateNotFound
			res.Detail = "no rebuild attestation for this version (package may exist, version not rebuilt)"
		case strings.Contains(text, "deadline") || strings.Contains(text, "timeout") ||
			strings.Contains(text, "UNAVAILABLE") || strings.Contains(text, "executable file not found") ||
			strings.Contains(text, "not found in %PATH%") || strings.Contains(text, "not found in $PATH"):
			res.State = StateUnavailable
			res.Detail = "oss-rebuild service unreachable: " + text
		default:
			res.State = StateFailed
			res.Detail = "rebuild lookup failed: " + text
		}
		return res, nil
	}
	// Markers and human fields may live on either stream; machine payloads
	// below use stdout only.
	out := stdout + "\n" + stderr
	if !strings.Contains(out, "Rebuild found!") {
		res.State = StateFailed
		res.Detail = "unexpected summary output (no 'Rebuild found!' marker)"
		return res, nil
	}
	res.State = StateVerified // CLI verifies attestation signatures by default
	if m := noteRe.FindStringSubmatch(out); m != nil {
		res.Artifact = m[1]
	}
	if m := digestRe.FindStringSubmatch(out); m != nil {
		res.UpstreamDigest = "sha256:" + m[1]
	}
	if m := commitRe.FindStringSubmatch(out); m != nil {
		res.SourceCommit = m[1]
	}
	if m := builtRe.FindStringSubmatch(out); m != nil {
		res.RebuiltAt = m[1]
	}
	res.Detail = "rebuild attestation retrieved and signature-verified by oss-rebuild CLI"
	// Preserve the full signed payload for forensics (stdout only: stderr
	// carries human NOTEs that would poison JSON parsing). Best-effort;
	// absence does not invalidate the verified summary above.
	if payload, _, perr := p.run(ctx, "get", eco, name, version, "--output=payload"); perr == nil {
		if IsProvenancePayload(payload) {
			res.Raw = []byte(payload)
		}
	}
	if res.UpstreamDigest == "" {
		res.State = StateMalformed
		res.Detail = "verified summary carried no upstream digest"
	}
	return res, nil
}

// IsProvenancePayload sanity-checks that the FIRST JSON document in the
// preserved bytes is an in-toto statement with an SLSA provenance predicate.
// (The CLI appends extra top-level values after the statement; a whole-blob
// Unmarshal would falsely reject. Never blind-store: Raw is kept only when
// the lead document validates.)
func IsProvenancePayload(payload string) bool {
	var doc struct {
		Type          string `json:"_type"`
		PredicateType string `json:"predicateType"`
	}
	dec := json.NewDecoder(strings.NewReader(payload))
	if err := dec.Decode(&doc); err != nil {
		return false
	}
	return doc.Type == "https://in-toto.io/Statement/v1" &&
		doc.PredicateType == "https://slsa.dev/provenance/v1"
}
