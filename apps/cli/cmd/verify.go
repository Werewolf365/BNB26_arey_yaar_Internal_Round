package cmd

import (
	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/quorum/quorum/internal/runner"
	"github.com/spf13/cobra"
)

func newVerifyCmd() *cobra.Command {
	var o runner.VerifyOptions
	c := &cobra.Command{
		Use:   "verify [--repo URL --commit SHA --artifact FILE | PKGREF]",
		Short: "Verify a release against independent builder evidence",
		Long: `Verify a release: pin source, hash artifact, collect builder evidence,
evaluate quorum policy, and report the release decision.

Exit codes: 0=verified 1=rejected 2=insufficient evidence
3=conflict/investigation 4=operational error 5=invalid input`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				o.PkgRef = args[0]
			} else if len(args) > 1 {
				return &exitErr{code: exitcodes.InvalidInput}
			}
			verbose, _ := cmd.Flags().GetBool("verbose")
			o.Verbose = verbose
			res, code, human, err := runner.RunVerify(o)
			if err != nil {
				if code == exitcodes.InvalidInput {
					emitErr(cmd, err.Error())
					return &exitErr{code: code}
				}
				emitErr(cmd, "operational error: "+err.Error())
				return &exitErr{code: exitcodes.Operational}
			}
			payload := res
			if useJSON(cmd) {
				if err := emit(cmd, "", payload); err != nil {
					return &exitErr{code: exitcodes.Operational}
				}
			} else if err := emit(cmd, human, nil); err != nil {
				return &exitErr{code: exitcodes.Operational}
			}
			if code != 0 {
				return &exitErr{code: code}
			}
			return nil
		},
	}
	c.Flags().StringVar(&o.Repo, "repo", "", "source repository URL (https, allowlisted host)")
	c.Flags().StringVar(&o.Commit, "commit", "", "pinned source commit")
	c.Flags().StringVar(&o.Artifact, "artifact", "", "artifact file to verify")
	c.Flags().BoolVar(&o.FixtureTiny, "fixture-tiny", false, "use the built-in deterministic tiny artifact")
	c.Flags().StringVar(&o.ExpectedDigest, "expected-digest", "", "expected sha256:<hex>; mismatch rejects")
	c.Flags().StringVar(&o.InjectConflictBuilder, "inject-conflict", "", "TEST ONLY: simulate disagreement from this builder id")
	c.Flags().BoolVar(&o.InjectTamper, "inject-tamper", false, "TEST ONLY: append one byte before hashing")
	c.Flags().IntVar(&o.MinBuilders, "min-builders", 0, "override policy minBuilders (0 = default)")
	c.Flags().IntVar(&o.RequiredAgree, "required-agreement", 0, "override required agreement (0 = default)")
	c.Flags().IntVar(&o.RequiredGroups, "required-groups", 0, "override required independent groups (0 = default)")
	c.Flags().IntVar(&o.ConflictTolerance, "tolerance", -1, "override conflict tolerance (-1 = default)")
	c.Flags().StringVar(&o.PolicyFile, "policy-file", "", "full JSON policy file (overrides scalar flags)")
	c.Flags().StringVar(&o.OssMode, "oss-mode", "fixture", "oss-rebuild evidence: fixture (deterministic label) or live (shells the real CLI)")
	c.Flags().StringVar(&o.OssBin, "oss-bin", "", "oss-rebuild executable (default: PATH lookup)")
	return c
}
