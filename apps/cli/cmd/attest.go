package cmd

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/quorum/quorum/internal/exitcodes"
	qcosign "github.com/quorum/quorum/services/cosign"
	"github.com/quorum/quorum/services/sigstore"
	"github.com/spf13/cobra"
)

func newAttestCmd() *cobra.Command {
	a := &cobra.Command{Use: "attest", Short: "Sign and verify standard DSSE attestations"}

	gen := &cobra.Command{
		Use:   "genkey --private PRIV.pem --public PUB.pem",
		Short: "Generate a P-256 signing keypair (private written 0600)",
		RunE: func(cmd *cobra.Command, args []string) error {
			privPath, _ := cmd.Flags().GetString("private")
			pubPath, _ := cmd.Flags().GetString("public")
			if privPath == "" || pubPath == "" {
				emitErr(cmd, "--private and --public are required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			priv, err := sigstore.GenerateKey()
			if err != nil {
				emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.Operational}
			}
			privPEM, _ := sigstore.PrivateToPEM(priv)
			pubPEM, _ := sigstore.PublicToPEM(&priv.PublicKey)
			if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
				emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.Operational}
			}
			if err := os.WriteFile(pubPath, pubPEM, 0o600); err != nil {
				emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.Operational}
			}
			kid, _ := sigstore.KeyID(&priv.PublicKey)
			return emit(cmd, fmt.Sprintf("key %s written\n", kid), map[string]string{"keyId": kid})
		},
	}
	gen.Flags().String("private", "", "private key output path")
	gen.Flags().String("public", "", "public key output path")

	sign := &cobra.Command{
		Use:   "sign --artifact FILE --commit SHA --builder ID --key PRIV.pem",
		Short: "Hash an artifact and sign a standard provenance statement",
		RunE: func(cmd *cobra.Command, args []string) error {
			artifact, _ := cmd.Flags().GetString("artifact")
			name, _ := cmd.Flags().GetString("artifact-name")
			commit, _ := cmd.Flags().GetString("commit")
			builder, _ := cmd.Flags().GetString("builder")
			buildType, _ := cmd.Flags().GetString("build-type")
			keyPath, _ := cmd.Flags().GetString("key")
			output, _ := cmd.Flags().GetString("output")
			transparency, _ := cmd.Flags().GetString("transparency")
			if artifact == "" || commit == "" || builder == "" || keyPath == "" {
				emitErr(cmd, "--artifact, --commit, --builder and --key are required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			if name == "" {
				name = artifact
			}
			data, err := os.ReadFile(artifact)
			if err != nil {
				emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.InvalidInput}
			}
			keyRaw, err := os.ReadFile(keyPath)
			if err != nil {
				emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.InvalidInput}
			}
			priv, err := sigstore.ParsePrivatePEM(keyRaw)
			if err != nil {
				emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.InvalidInput}
			}
			sum := sha256.Sum256(data)
			st := sigstore.ProvenanceStatement(name, hex.EncodeToString(sum[:]), commit, builder, orDefault(buildType, "https://quorum.example/buildtypes/local/v1"))
			env, err := sigstore.SignStatement(st, priv)
			if err != nil {
				emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.Operational}
			}
			raw, _ := json.MarshalIndent(env, "", "  ")
			if transparency != "" {
				body, _ := json.Marshal(map[string]any{"_type": st.Type, "subject": st.Subject})
				sh := sha256.Sum256(body)
				kid, _ := sigstore.KeyID(&priv.PublicKey)
				if _, err := sigstore.NewFileLog(transparency).Append(hex.EncodeToString(sh[:]), kid); err != nil {
					emitErr(cmd, err.Error())
					return &exitErr{code: exitcodes.Operational}
				}
			}
			if output != "" {
				if err := os.WriteFile(output, raw, 0o600); err != nil {
					emitErr(cmd, err.Error())
					return &exitErr{code: exitcodes.Operational}
				}
			}
			return emit(cmd, string(raw)+"\n", env)
		},
	}
	sign.Flags().String("artifact", "", "artifact file to hash")
	sign.Flags().String("artifact-name", "", "subject name (default: artifact path)")
	sign.Flags().String("commit", "", "pinned source commit")
	sign.Flags().String("builder", "", "builder id (URI)")
	sign.Flags().String("build-type", "", "build type URI")
	sign.Flags().String("key", "", "private key PEM")
	sign.Flags().String("output", "", "envelope output path")
	sign.Flags().String("transparency", "", "dev transparency log file (append)")

	verify := &cobra.Command{
		Use:   "verify --envelope ENV.json --key PUB.pem",
		Short: "Verify a DSSE envelope against policy (exit 0 valid, 1 invalid)",
		RunE: func(cmd *cobra.Command, args []string) error {
			envPath, _ := cmd.Flags().GetString("envelope")
			keyPaths, _ := cmd.Flags().GetStringSlice("key")
			expDigest, _ := cmd.Flags().GetString("expect-digest")
			expCommit, _ := cmd.Flags().GetString("expect-commit")
			allowed, _ := cmd.Flags().GetStringSlice("allow-builder")
			if envPath == "" || len(keyPaths) == 0 {
				emitErr(cmd, "--envelope and at least one --key are required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			raw, err := os.ReadFile(envPath)
			if err != nil {
				emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.InvalidInput}
			}
			var env sigstore.Envelope
			if err := json.Unmarshal(raw, &env); err != nil {
				emitErr(cmd, "malformed envelope: "+err.Error())
				return &exitErr{code: exitcodes.InvalidInput}
			}
			trusted := []*ecdsa.PublicKey{}
			for _, kp := range keyPaths {
				raw, err := os.ReadFile(kp)
				if err != nil {
					emitErr(cmd, err.Error())
					return &exitErr{code: exitcodes.InvalidInput}
				}
				pub, err := sigstore.ParsePublicPEM(raw)
				if err != nil {
					emitErr(cmd, err.Error())
					return &exitErr{code: exitcodes.InvalidInput}
				}
				trusted = append(trusted, pub)
			}
			st, err := sigstore.VerifyPolicy(env, trusted, expDigest, expCommit, allowed)
			if err != nil {
				emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.Rejected}
			}
			human := fmt.Sprintf("attestation valid: %s @ %s by %v\n", st.Subject[0].Name, st.Subject[0].Digest["sha256"], builderOf(st))
			return emit(cmd, human, map[string]any{"valid": true, "subject": st.Subject})
		},
	}
	verify.Flags().String("envelope", "", "envelope JSON path")
	verify.Flags().StringSlice("key", nil, "trusted public key PEM (repeatable)")
	verify.Flags().String("expect-digest", "", "required subject sha256 hex")
	verify.Flags().String("expect-commit", "", "required source commit")
	verify.Flags().StringSlice("allow-builder", nil, "allowlisted builder id (repeatable)")

	a.AddCommand(gen, sign, verify, newCosignSignCmd(), newCosignVerifyCmd(), newRekorGetCmd(), newCosignAttestBlobCmd(), newCosignVerifyAttestationCmd())
	return a
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func builderOf(st sigstore.Statement) string {
	if rd, ok := st.Predicate.RunDetails["builder"].(map[string]any); ok {
		if id, ok := rd["id"].(string); ok {
			return id
		}
	}
	return ""
}

// cosignExit maps adapter states onto the CLI exit contract (prompt
// section 40): 0 verified, 1 rejected (bad signature), 4 operational
// (binary missing / log unreachable), 5 invalid input.
func cosignExit(state string) int {
	switch state {
	case qcosign.StateVerified:
		return exitcodes.Verified
	case qcosign.StateInvalidSig:
		return exitcodes.Rejected
	case qcosign.StateUnavailable:
		return exitcodes.Operational
	default:
		return exitcodes.InvalidInput
	}
}

func newCosignSignCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "cosign-sign --artifact FILE --key KEYREF --output-signature SIG",
		Short: "Sign a blob with the real cosign CLI (explicit key, no keyless)",
		RunE: func(cmd *cobra.Command, args []string) error {
			artifact, _ := cmd.Flags().GetString("artifact")
			keyRef, _ := cmd.Flags().GetString("key")
			sigOut, _ := cmd.Flags().GetString("output-signature")
			bundleOut, _ := cmd.Flags().GetString("output-bundle")
			bin, _ := cmd.Flags().GetString("cosign-bin")
			if artifact == "" || keyRef == "" || sigOut == "" {
				emitErr(cmd, "--artifact, --key and --output-signature are required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			p := qcosign.DefaultProvider()
			if bin != "" {
				p.Bin = bin
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			res := p.SignBlob(ctx, keyRef, artifact, sigOut, bundleOut)
			if res.State != qcosign.StateVerified {
				emitErr(cmd, res.Detail)
				return &exitErr{code: cosignExit(res.State)}
			}
			return emit(cmd, "signed "+sigOut+"\n", map[string]string{"state": res.State, "signature": sigOut})
		},
	}
	c.Flags().String("artifact", "", "artifact file to sign")
	c.Flags().String("key", "", "cosign key reference (file path, k8s:// or KMS URI)")
	c.Flags().String("output-signature", "", "signature output path")
	c.Flags().String("output-bundle", "", "rekor bundle output path (optional, preserves transparency evidence)")
	c.Flags().String("cosign-bin", "", "cosign executable (default: PATH lookup)")
	return c
}

func newCosignVerifyCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "cosign-verify --artifact FILE --key PUB --signature SIG",
		Short: "Verify a blob signature with the real cosign CLI (exit 0 valid, 1 invalid)",
		RunE: func(cmd *cobra.Command, args []string) error {
			artifact, _ := cmd.Flags().GetString("artifact")
			keyRef, _ := cmd.Flags().GetString("key")
			sigPath, _ := cmd.Flags().GetString("signature")
			bundlePath, _ := cmd.Flags().GetString("bundle")
			bin, _ := cmd.Flags().GetString("cosign-bin")
			if artifact == "" || keyRef == "" || sigPath == "" {
				emitErr(cmd, "--artifact, --key and --signature are required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			p := qcosign.DefaultProvider()
			if bin != "" {
				p.Bin = bin
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			res := p.VerifyBlob(ctx, keyRef, artifact, sigPath, bundlePath)
			if res.State != qcosign.StateVerified {
				emitErr(cmd, res.Detail)
				return &exitErr{code: cosignExit(res.State)}
			}
			return emit(cmd, "cosign: signature verified\n", map[string]string{"state": res.State})
		},
	}
	c.Flags().String("artifact", "", "artifact file to verify")
	c.Flags().String("key", "", "trusted cosign public key / key reference")
	c.Flags().String("signature", "", "signature file")
	c.Flags().String("bundle", "", "rekor bundle file (optional, checks transparency inclusion)")
	c.Flags().String("cosign-bin", "", "cosign executable (default: PATH lookup)")
	return c
}

func newRekorGetCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "rekor-get --uuid ENTRY_UUID",
		Short: "Fetch a Rekor transparency entry by UUID (read-only, no credentials)",
		RunE: func(cmd *cobra.Command, args []string) error {
			uuid, _ := cmd.Flags().GetString("uuid")
			base, _ := cmd.Flags().GetString("rekor-url")
			if uuid == "" {
				emitErr(cmd, "--uuid is required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			r := qcosign.DefaultRekor()
			if base != "" {
				r.BaseURL = base
			}
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			body, err := r.Entry(ctx, uuid)
			if err != nil {
				msg := err.Error()
				code := exitcodes.Operational
				if len(msg) >= 9 && msg[:9] == "NOT_FOUND" {
					code = exitcodes.Rejected
				} else if len(msg) >= 13 && msg[:13] == "INVALID_INPUT" {
					code = exitcodes.InvalidInput
				}
				emitErr(cmd, msg)
				return &exitErr{code: code}
			}
			var v any
			_ = json.Unmarshal(body, &v)
			return emit(cmd, string(body)+"\n", v)
		},
	}
	c.Flags().String("uuid", "", "rekor entry UUID")
	c.Flags().String("rekor-url", "", "rekor base URL (default: https://rekor.sigstore.dev)")
	return c
}

// newCosignAttestBlobCmd creates a DSSE attestation over a blob with the real
// cosign CLI: attest-blob --predicate --type [--key] --bundle --yes. The
// --key flag is optional: when omitted the call is keyless (Fulcio/OIDC),
// which is interactive (browser / ambient token) and unsuitable for headless
// CI. --output-attestation is Quorum's flag name for the bundle file cosign
// writes (--bundle upstream).
func newCosignAttestBlobCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "cosign-attest-blob --artifact FILE --predicate FILE --type custom --output-attestation BUNDLE",
		Short: "Attest a blob with the real cosign CLI (explicit key, or keyless when --key is omitted)",
		RunE: func(cmd *cobra.Command, args []string) error {
			artifact, _ := cmd.Flags().GetString("artifact")
			predicate, _ := cmd.Flags().GetString("predicate")
			predType, _ := cmd.Flags().GetString("type")
			keyRef, _ := cmd.Flags().GetString("key")
			out, _ := cmd.Flags().GetString("output-attestation")
			bin, _ := cmd.Flags().GetString("cosign-bin")
			if artifact == "" || predicate == "" || out == "" {
				emitErr(cmd, "--artifact, --predicate and --output-attestation are required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			p := qcosign.DefaultProvider()
			if bin != "" {
				p.Bin = bin
			}
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			res := p.AttestBlob(ctx, keyRef, artifact, predicate, predType, out)
			if res.State != qcosign.StateVerified {
				emitErr(cmd, res.Detail)
				return &exitErr{code: cosignExit(res.State)}
			}
			return emit(cmd, "attested "+out+"\n", map[string]string{"state": res.State, "attestation": out})
		},
	}
	c.Flags().String("artifact", "", "artifact file to attest")
	c.Flags().String("predicate", "", "predicate JSON file")
	c.Flags().String("type", "custom", "predicate type (default: custom)")
	c.Flags().String("key", "", "cosign key reference (omit for keyless Fulcio/OIDC signing, interactive)")
	c.Flags().String("output-attestation", "", "attestation bundle output path (passed as --bundle to cosign)")
	c.Flags().String("cosign-bin", "", "cosign executable (default: PATH lookup)")
	return c
}

// newCosignVerifyAttestationCmd verifies a DSSE blob attestation with the
// real cosign CLI: verify-blob-attestation --bundle (--key | Fulcio identity
// pair). Exactly one trust anchor is required: --key for explicit keys, or
// --certificate-identity plus --certificate-oidc-issuer for keyless Fulcio
// certificates (non-interactive). --signature is Quorum's flag name for the
// bundle file cosign reads (--bundle upstream). --rekor-url is an opt-in
// passthrough for older cosign CLIs and is omitted when empty.
func newCosignVerifyAttestationCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "cosign-verify-attestation --artifact FILE --signature BUNDLE",
		Short: "Verify a blob attestation with the real cosign CLI (exit 0 valid, 1 invalid)",
		RunE: func(cmd *cobra.Command, args []string) error {
			artifact, _ := cmd.Flags().GetString("artifact")
			sigPath, _ := cmd.Flags().GetString("signature")
			keyRef, _ := cmd.Flags().GetString("key")
			identity, _ := cmd.Flags().GetString("certificate-identity")
			issuer, _ := cmd.Flags().GetString("certificate-oidc-issuer")
			rekorURL, _ := cmd.Flags().GetString("rekor-url")
			bin, _ := cmd.Flags().GetString("cosign-bin")
			if artifact == "" || sigPath == "" {
				emitErr(cmd, "--artifact and --signature are required")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			opts := qcosign.AttestOpts{KeyRef: keyRef, CertIdentity: identity, CertIssuer: issuer, RekorURL: rekorURL}
			if !opts.HasAnchor() {
				emitErr(cmd, "one trust anchor is required: --key or --certificate-identity plus --certificate-oidc-issuer")
				return &exitErr{code: exitcodes.InvalidInput}
			}
			p := qcosign.DefaultProvider()
			if bin != "" {
				p.Bin = bin
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			res := p.VerifyBlobAttestation(ctx, opts, artifact, sigPath)
			if res.State != qcosign.StateVerified {
				emitErr(cmd, res.Detail)
				return &exitErr{code: cosignExit(res.State)}
			}
			return emit(cmd, "cosign: attestation verified\n", map[string]string{"state": res.State})
		},
	}
	c.Flags().String("artifact", "", "artifact file the attestation covers")
	c.Flags().String("signature", "", "attestation bundle file (passed as --bundle to cosign)")
	c.Flags().String("key", "", "trusted cosign public key / key reference")
	c.Flags().String("certificate-identity", "", "Fulcio certificate identity for keyless verification")
	c.Flags().String("certificate-oidc-issuer", "", "Fulcio OIDC issuer for keyless verification")
	c.Flags().String("rekor-url", "", "rekor base URL passthrough for older cosign CLIs (default: omitted)")
	c.Flags().String("cosign-bin", "", "cosign executable (default: PATH lookup)")
	return c
}
