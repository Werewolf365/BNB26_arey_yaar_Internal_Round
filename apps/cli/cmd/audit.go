package cmd

import (
	"fmt"

	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/quorum/quorum/internal/runner"
	"github.com/spf13/cobra"
)

func newAuditCmd() *cobra.Command {
	a := &cobra.Command{Use: "audit", Short: "Verify tamper-evident audit chains"}
	v := &cobra.Command{
		Use:   "verify [--chain FILE | --demo] [--tamper]",
		Short: "Verify an audit chain file (exit 0 valid, 1 tampered, 5 bad input)",
		RunE: func(cmd *cobra.Command, args []string) error {
			chainPath, _ := cmd.Flags().GetString("chain")
			demo, _ := cmd.Flags().GetBool("demo")
			tamper, _ := cmd.Flags().GetBool("tamper")
			var chain []runner.AuditRecord
			var err error
			switch {
			case demo:
				chain, err = runner.DemoChain()
			case chainPath != "":
				chain, err = runner.LoadAuditChain(chainPath)
			default:
				return &exitErr{code: exitcodes.InvalidInput}
			}
			if err != nil {
			emitErr(cmd, err.Error())
				return &exitErr{code: exitcodes.InvalidInput}
			}
			if tamper && len(chain) > 1 {
				chain[1].Payload["builderId"] = "builder-EVIL"
			}
			ok, at, reason := runner.VerifyChain(chain)
			payload := map[string]any{"ok": ok, "records": len(chain), "brokenAt": at, "reason": reason}
			human := fmt.Sprintf("audit chain valid (%d records)\n", len(chain))
			if !ok {
				human = fmt.Sprintf("AUDIT CHAIN INVALID\nbroken at record: %d\nreason: %s\n", at, reason)
			}
			if err := emit(cmd, human, payload); err != nil {
				return &exitErr{code: exitcodes.Operational}
			}
			if !ok {
				return &exitErr{code: exitcodes.Rejected}
			}
			return nil
		},
	}
	v.Flags().String("chain", "", "audit chain JSON file")
	v.Flags().Bool("demo", false, "verify a built-in demo chain")
	v.Flags().Bool("tamper", false, "TEST ONLY: mutate record 1 before verifying")
	a.AddCommand(v)
	return a
}
