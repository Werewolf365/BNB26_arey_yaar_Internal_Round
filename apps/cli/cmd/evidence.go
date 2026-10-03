package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/quorum/quorum/internal/runner"
	"github.com/spf13/cobra"
)

func newEvidenceCmd() *cobra.Command {
	var o runner.VerifyOptions
	var output string
	c := &cobra.Command{
		Use:   "evidence",
		Short: "Export evidence bundles",
	}
	exp := &cobra.Command{
		Use:   "export [--repo URL --commit SHA --artifact FILE | PKGREF]",
		Short: "Run verification and write the evidence bundle to --output",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				o.PkgRef = args[0]
			} else if len(args) > 1 {
				return &exitErr{code: exitcodes.InvalidInput}
			}
			if output == "" {
				return &exitErr{code: exitcodes.InvalidInput}
			}
			verbose, _ := cmd.Flags().GetBool("verbose")
			o.Verbose = verbose
			res, code, _, err := runner.RunVerify(o)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
				return &exitErr{code: exitcodes.Operational}
			}
			bundle := map[string]any{
				"exportedAt": time.Now().UTC().Format(time.RFC3339),
				"result":     res,
			}
			data, err := json.MarshalIndent(bundle, "", "  ")
			if err != nil {
				return &exitErr{code: exitcodes.Operational}
			}
			if err := os.WriteFile(output, data, 0o600); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
				return &exitErr{code: exitcodes.Operational}
			}
			if err := emit(cmd, fmt.Sprintf("evidence bundle written to %s (decision %s)\n", output, res.Decision), bundle); err != nil {
				return &exitErr{code: exitcodes.Operational}
			}
			if code != 0 {
				return &exitErr{code: code}
			}
			return nil
		},
	}
	exp.Flags().StringVar(&o.Repo, "repo", "", "source repository URL")
	exp.Flags().StringVar(&o.Commit, "commit", "", "pinned source commit")
	exp.Flags().StringVar(&o.Artifact, "artifact", "", "artifact file")
	exp.Flags().BoolVar(&o.FixtureTiny, "fixture-tiny", false, "use built-in tiny artifact")
	exp.Flags().StringVar(&o.ExpectedDigest, "expected-digest", "", "expected sha256:<hex>")
	exp.Flags().StringVar(&o.PolicyFile, "policy-file", "", "policy JSON file")
	exp.Flags().StringVar(&output, "output", "", "bundle output path (required)")
	c.AddCommand(exp)
	return c
}
