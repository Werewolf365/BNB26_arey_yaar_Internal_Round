package cmd

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/quorum/quorum/internal/exitcodes"
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

	a.AddCommand(gen, sign, verify)
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
