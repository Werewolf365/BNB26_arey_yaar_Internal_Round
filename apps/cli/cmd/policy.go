package cmd

import (
	"fmt"

	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/quorum/quorum/internal/runner"
	"github.com/spf13/cobra"
)

func newPolicyCmd() *cobra.Command {
	p := &cobra.Command{Use: "policy", Short: "Validate and test quorum policies"}

	validate := &cobra.Command{
		Use:   "validate --policy-file FILE",
		Short: "Validate a policy file (exit 0 valid, 5 invalid)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("policy-file")
			if path == "" {
				return &exitErr{code: exitcodes.InvalidInput}
			}
			pol, err := runner.LoadPolicyFile(path)
			if err != nil {
				if !useJSON(cmd) {
					fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
				}
				return &exitErr{code: exitcodes.InvalidInput}
			}
			hash, _ := runner.PolicyHash(pol)
			if err := emit(cmd, fmt.Sprintf("policy %s (%s) valid\npolicy hash: %s\n", pol.PolicyID, pol.Version, hash),
				map[string]string{"policyId": pol.PolicyID, "version": pol.Version, "policyHash": hash}); err != nil {
				return &exitErr{code: exitcodes.Operational}
			}
			return nil
		},
	}
	validate.Flags().String("policy-file", "", "policy JSON file")

	test := &cobra.Command{
		Use:   "test --policy-file FILE --evidence-file FILE",
		Short: "Evaluate a policy against an evidence file",
		RunE: func(cmd *cobra.Command, args []string) error {
			pf, _ := cmd.Flags().GetString("policy-file")
			ef, _ := cmd.Flags().GetString("evidence-file")
			if pf == "" || ef == "" {
				return &exitErr{code: exitcodes.InvalidInput}
			}
			res, pol, err := runner.RunPolicyTest(pf, ef)
			if err != nil {
				if !useJSON(cmd) {
					fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
				}
				return &exitErr{code: exitcodes.InvalidInput}
			}
			human := fmt.Sprintf("decision: %s\nrequired: %d satisfied: %d\n", res.Decision, res.Required, res.Satisfied)
			for _, r := range res.Reasons {
				human += "reason: " + r + "\n"
			}
			for _, c := range res.Conflicts {
				human += fmt.Sprintf("conflict: %s from %v\n", c.Digest, c.Builders)
			}
			_ = pol
			if err := emit(cmd, human, res); err != nil {
				return &exitErr{code: exitcodes.Operational}
			}
			if code := exitcodes.ForDecision(res.Decision); code != 0 {
				return &exitErr{code: code}
			}
			return nil
		},
	}
	test.Flags().String("policy-file", "", "policy JSON file")
	test.Flags().String("evidence-file", "", "evidence JSON file {expectedSource, evidence[]}")

	p.AddCommand(validate, test)
	return p
}
