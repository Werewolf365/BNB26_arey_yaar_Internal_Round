package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/quorum/quorum/internal/runner"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "inspect RESULT.json",
		Short: "Show a human summary of a verification result file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return &exitErr{code: exitcodes.InvalidInput}
			}
			data, err := os.ReadFile(args[0])
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
				return &exitErr{code: exitcodes.InvalidInput}
			}
			var r runner.VerifyResult
			if err := json.Unmarshal(data, &r); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: malformed result JSON: %v\n", err)
				return &exitErr{code: exitcodes.InvalidInput}
			}
			human := fmt.Sprintf("decision: %s\nartifact: %s\nsource: %s\npolicy: %s\noss: %s\n",
				r.Decision, r.Artifact, r.Source, r.PolicyID, r.OssState)
			for _, reason := range r.Reasons {
				human += "reason: " + reason + "\n"
			}
			if err := emit(cmd, human, r); err != nil {
				return &exitErr{code: exitcodes.Operational}
			}
			if code := exitcodes.ForDecision(r.Decision); code != 0 {
				return &exitErr{code: code}
			}
			return nil
		},
	}
	return c
}
