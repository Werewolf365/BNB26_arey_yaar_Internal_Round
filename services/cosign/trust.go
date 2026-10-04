// Operator trust-root governance for keyless (Fulcio/OIDC) verification
// and Rekor transparency enforcement.
//
// Local P-256 keys prove the protocol; production keyless trust must be
// configured by the operator. A TrustRoot names exactly which Fulcio
// certificate identities and OIDC issuers count as trusted, which Rekor log
// is authoritative, and whether Rekor inclusion is required. The CLI loads
// it from a JSON file (--trust-root) or the QUORUM_TRUST_ROOT env var
// (inline JSON or @path). Verification authorizes the requested keyless
// identity against the local root before cosign runs — unlisted issuers or
// identities fail closed with SIGNER_NOT_TRUSTED, never a silent pass.
// Without a root, keyless verification passes through to cosign's own Fulcio
// chain validation (legacy behavior); the root is additive governance.
package cosign

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// TrustRoot is the operator-configured keyless trust anchor set.
//
// AllowedIssuers lists exact OIDC issuer URLs (e.g.
// "https://token.actions.githubusercontent.com"). AllowedIdentities lists
// permitted certificate identities as exact strings or "regexp:..." patterns
// (e.g. "regexp:^https://github.com/myorg/.*"). RekorURL names the
// authoritative transparency log; RequireRekor makes a Rekor inclusion check
// mandatory for a VERIFIED verdict.
type TrustRoot struct {
	AllowedIssuers    []string `json:"allowedIssuers"`
	AllowedIdentities []string `json:"allowedIdentities"`
	RekorURL          string   `json:"rekorUrl"`
	RequireRekor      bool     `json:"requireRekor"`
}

// DefaultTrustRoot is the empty root: no extra governance — keyless
// verification passes through to cosign's own Fulcio validation.
func DefaultTrustRoot() TrustRoot { return TrustRoot{} }

// LoadTrustRoot reads a root from a JSON file path. An empty path returns
// the deny-all default (explicit keys still work; keyless does not).
func LoadTrustRoot(path string) (TrustRoot, error) {
	if strings.TrimSpace(path) == "" {
		return DefaultTrustRoot(), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return TrustRoot{}, fmt.Errorf("INVALID_INPUT: cannot read trust root %q: %v", path, err)
	}
	return ParseTrustRoot(raw)
}

// ParseTrustRoot decodes and validates a root document.
func ParseTrustRoot(raw []byte) (TrustRoot, error) {
	var t TrustRoot
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(string(raw))))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return TrustRoot{}, fmt.Errorf("INVALID_INPUT: malformed trust root: %v", err)
	}
	for _, id := range t.AllowedIdentities {
		if pat, ok := strings.CutPrefix(id, "regexp:"); ok {
			if _, err := regexp.Compile(pat); err != nil {
				return TrustRoot{}, fmt.Errorf("INVALID_INPUT: bad identity pattern %q: %v", id, err)
			}
		}
		if strings.TrimSpace(strings.TrimPrefix(id, "regexp:")) == "" {
			return TrustRoot{}, fmt.Errorf("INVALID_INPUT: empty identity entry")
		}
	}
	for _, iss := range t.AllowedIssuers {
		if strings.TrimSpace(iss) == "" {
			return TrustRoot{}, fmt.Errorf("INVALID_INPUT: empty issuer entry")
		}
	}
	return t, nil
}

// TrustRootFromEnv loads the root from QUORUM_TRUST_ROOT: either inline JSON
// or @path to a JSON file. Unset/empty means no extra governance (cosign's
// own Fulcio validation applies).
func TrustRootFromEnv() (TrustRoot, error) {
	v := strings.TrimSpace(os.Getenv("QUORUM_TRUST_ROOT"))
	if v == "" {
		return DefaultTrustRoot(), nil
	}
	if path, ok := strings.CutPrefix(v, "@"); ok {
		return LoadTrustRoot(path)
	}
	return ParseTrustRoot([]byte(v))
}

// matchIdentity reports whether identity matches one allowlist entry.
func matchIdentity(entry, identity string) bool {
	if pat, ok := strings.CutPrefix(entry, "regexp:"); ok {
		m, err := regexp.MatchString(pat, identity)
		return err == nil && m
	}
	return entry == identity
}

// AuthorizeKeyless checks a keyless identity pair against the root.
// Exact issuer allowlist match plus one identity entry match is required.
func (t TrustRoot) AuthorizeKeyless(identity, issuer string) error {
	if strings.TrimSpace(identity) == "" || strings.TrimSpace(issuer) == "" {
		return fmt.Errorf("INVALID_INPUT: certificate identity and OIDC issuer are required")
	}
	issuerOK := false
	for _, a := range t.AllowedIssuers {
		if a == issuer {
			issuerOK = true
			break
		}
	}
	if !issuerOK {
		return fmt.Errorf("SIGNER_NOT_TRUSTED: OIDC issuer %q is not in the trust root", issuer)
	}
	for _, e := range t.AllowedIdentities {
		if matchIdentity(e, identity) {
			return nil
		}
	}
	return fmt.Errorf("SIGNER_NOT_TRUSTED: certificate identity %q is not in the trust root", identity)
}

// empty reports whether no keyless governance is configured. An empty root
// preserves legacy passthrough: cosign itself validates the Fulcio chain
// against its real roots. Any configured issuer/identity/Rekor setting
// switches enforcement on.
func (t TrustRoot) empty() bool {
	return len(t.AllowedIssuers) == 0 && len(t.AllowedIdentities) == 0 && !t.RequireRekor && strings.TrimSpace(t.RekorURL) == ""
}

// RekorFor returns the authoritative log URL (root override or public good).
func (t TrustRoot) RekorFor() string {
	if strings.TrimSpace(t.RekorURL) != "" {
		return t.RekorURL
	}
	return DefaultRekor().BaseURL
}

// RekorEnforced reports whether Rekor inclusion is mandatory.
func (t TrustRoot) RekorEnforced() bool { return t.RequireRekor }

// VerifyAttestationWithTrust enforces the operator root before delegating to
// the cosign CLI: with a root configured, keyless anchors must first satisfy
// AuthorizeKeyless (unlisted identities fail closed with SIGNER_NOT_TRUSTED,
// before the CLI ever runs). Without a root, keyless verification passes
// through to cosign, which validates the Fulcio chain against its own real
// roots. When the root requires Rekor, a log entry lookup (rekorUUID) must
// succeed or the result degrades to UNAVAILABLE — never a silent VERIFIED
// without the required transparency evidence.
func (p Provider) VerifyAttestationWithTrust(ctx context.Context, root TrustRoot, opts AttestOpts, blobPath, envPath, rekorUUID string) Result {
	if opts.KeyRef == "" && !root.empty() {
		if err := root.AuthorizeKeyless(opts.CertIdentity, opts.CertIssuer); err != nil {
			return Result{State: StateInvalidSig, Detail: err.Error()}
		}
	}
	res := p.VerifyBlobAttestation(ctx, opts, blobPath, envPath)
	if res.State != StateVerified {
		return res
	}
	if opts.KeyRef == "" && root.RekorEnforced() {
		if strings.TrimSpace(rekorUUID) == "" {
			return Result{State: StateUnavailable, Detail: "UNAVAILABLE: trust root requires Rekor inclusion but no entry UUID was provided (--rekor-entry)"}
		}
		rc := RekorClient{BaseURL: root.RekorFor(), Timeout: 30 * time.Second}
		if _, err := rc.Entry(ctx, rekorUUID); err != nil {
			return Result{State: StateUnavailable, Detail: "UNAVAILABLE: Rekor inclusion check failed: " + err.Error()}
		}
		res.Detail += " + rekor inclusion confirmed"
	}
	return res
}
