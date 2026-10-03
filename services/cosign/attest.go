// DSSE attest-blob verification path with keyless (Fulcio/OIDC) support.
//
// Flag surface verified against the cosign main-branch CLI reference
// (doc/cosign_attest-blob.md, doc/cosign_verify-blob-attestation.md,
// fetched Oct 2026): both commands are bundle-based. attest-blob writes the
// DSSE envelope with --bundle <out> (there is no --output-attestation flag
// upstream); verify-blob-attestation reads it back with --bundle <env> plus
// either --key <keyRef> or the keyless Fulcio identity pair
// (--certificate-identity + --certificate-oidc-issuer). Upstream
// verify-blob-attestation has no --rekor-url flag (trust is configured via
// --trusted-root / signing-config in current releases); RekorURL is kept as
// an opt-in passthrough for older CLI versions and is omitted when empty so
// the default path always matches current upstream.
//
// Keyless signing (AttestBlob with keyRef == "") is passed straight through
// to the CLI: it needs interactive OIDC (browser / ambient token) and will
// block or fail in headless CI. Keyless VERIFY is non-interactive and fully
// supported: it only checks the Fulcio certificate identity against local
// trust roots.
//
// State taxonomy and UNAVAILABLE rules mirror Provider.VerifyBlob: exit 0
// plus a "verified" marker => VERIFIED; a CLI that ran and refused =>
// SIGNATURE_INVALID; missing binary / timeout => UNAVAILABLE with an install
// hint (never a faked verdict); exit 0 without a marker => MALFORMED.
package cosign

import (
	"context"
	"strings"
)

// AttestOpts selects the trust anchor for blob-attestation verification:
// either an explicit key reference (KeyRef) or the keyless Fulcio identity
// pair (CertIdentity + CertIssuer). RekorURL is an opt-in passthrough for
// older cosign CLIs; it is omitted when empty.
type AttestOpts struct {
	KeyRef       string
	CertIdentity string
	CertIssuer   string
	RekorURL     string
}

// HasAnchor reports whether at least one trust anchor is configured.
func (o AttestOpts) HasAnchor() bool {
	if o.KeyRef != "" {
		return true
	}
	return o.CertIdentity != "" && o.CertIssuer != ""
}

// AttestBlob creates a DSSE attestation over a blob:
// cosign attest-blob --predicate <predicatePath> --type <predicateType>
// [--key <keyRef>] --bundle <outPath> --yes <blobPath>.
// keyRef may be empty for keyless (Fulcio/OIDC) signing, which is
// interactive: the CLI opens a browser / needs ambient OIDC credentials.
// predicateType defaults to "custom" when empty (the cosign default).
func (p Provider) AttestBlob(ctx context.Context, keyRef, blobPath, predicatePath, predicateType, outPath string) Result {
	if blobPath == "" || predicatePath == "" || outPath == "" {
		return Result{State: StateInvalidInput, Detail: "INVALID_INPUT: blob, predicate and output are required"}
	}
	if predicateType == "" {
		predicateType = "custom"
	}
	args := []string{"attest-blob", "--predicate", predicatePath, "--type", predicateType}
	if keyRef != "" {
		args = append(args, "--key", keyRef)
	}
	args = append(args, "--bundle", outPath, "--yes", blobPath)
	stdout, stderr, err := p.run(ctx, args...)
	out := stdout + "\n" + stderr
	if err != nil {
		text := err.Error() + "\n" + out
		for _, marker := range []string{
			"executable file not found", "not found in %PATH%", "not found in $PATH",
			"deadline exceeded", "context canceled", "signal: killed",
		} {
			if strings.Contains(text, marker) {
				return Result{State: StateUnavailable, Detail: err.Error() + " (install: https://docs.sigstore.dev/cosign/installation/)", Raw: []byte(out)}
			}
		}
		// The CLI ran and refused (bad key, missing predicate, OIDC
		// failure): operational, never a signature verdict.
		return Result{State: StateUnavailable, Detail: "UNAVAILABLE: attest-blob failed: " + strings.TrimSpace(text), Raw: []byte(out)}
	}
	return Result{State: StateVerified, Detail: "blob attested (" + strings.TrimSpace(out) + ")", Raw: []byte(out)}
}

// VerifyBlobAttestation verifies a DSSE blob attestation:
// cosign verify-blob-attestation --bundle <envPath>
// [--key <keyRef> | --certificate-identity <id> --certificate-oidc-issuer <issuer>]
// [--rekor-url <url>] <blobPath>.
// At least one trust anchor (explicit key or the full keyless identity pair)
// is required, else INVALID_INPUT. Exit 0 => VERIFIED; a CLI refusal =>
// SIGNATURE_INVALID; missing binary/timeout => UNAVAILABLE; exit 0 without a
// verification marker => MALFORMED_EVIDENCE (never silent success).
func (p Provider) VerifyBlobAttestation(ctx context.Context, opts AttestOpts, blobPath, envPath string) Result {
	if blobPath == "" || envPath == "" {
		return Result{State: StateInvalidInput, Detail: "INVALID_INPUT: blob and attestation bundle are required"}
	}
	if !opts.HasAnchor() {
		return Result{State: StateInvalidInput, Detail: "INVALID_INPUT: one trust anchor is required: --key or --certificate-identity plus --certificate-oidc-issuer"}
	}
	args := []string{"verify-blob-attestation", "--bundle", envPath}
	if opts.KeyRef != "" {
		args = append(args, "--key", opts.KeyRef)
	} else {
		args = append(args, "--certificate-identity", opts.CertIdentity, "--certificate-oidc-issuer", opts.CertIssuer)
	}
	if opts.RekorURL != "" {
		args = append(args, "--rekor-url", opts.RekorURL)
	}
	args = append(args, blobPath)
	stdout, stderr, err := p.run(ctx, args...)
	out := stdout + "\n" + stderr
	if err != nil {
		text := err.Error() + "\n" + out
		// The CLI never ran (binary missing, timeout, context cancel):
		// UNAVAILABLE with a hint, never a faked verdict.
		for _, marker := range []string{
			"executable file not found", "not found in %PATH%", "not found in $PATH",
			"deadline exceeded", "context canceled", "signal: killed",
		} {
			if strings.Contains(text, marker) {
				return Result{State: StateUnavailable, Detail: err.Error() + " (install: https://docs.sigstore.dev/cosign/installation/)", Raw: []byte(out)}
			}
		}
		// The CLI ran and refused: attestation invalid.
		return Result{State: StateInvalidSig, Detail: "SIGNATURE_INVALID: " + strings.TrimSpace(text), Raw: []byte(out)}
	}
	if !strings.Contains(strings.ToLower(out), "verified") {
		// Exit 0 but no confirmation marker: treat as malformed evidence,
		// never silent success.
		return Result{State: StateMalformed, Detail: "MALFORMED_EVIDENCE: cosign exited 0 without a verification marker", Raw: []byte(out)}
	}
	return Result{State: StateVerified, Detail: "blob attestation verified by cosign", Raw: []byte(out)}
}
