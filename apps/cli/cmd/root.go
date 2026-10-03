package cmd

import (
	"github.com/spf13/cobra"
)

var version = "0.2.0-slice2"

// NewRoot builds the full command tree (exported for tests).
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "quorum",
		Short: "Don't trust the binary. Trust the builders.",
		Long: `Quorum independently verifies software releases: rebuild evidence, signed
attestations, configurable quorum policy, tamper-evident audit, and optional
EVM anchoring. Quorum reports verification evidence and policy decisions —
never proof that software is safe.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().Bool("json", false, "machine-readable JSON output")
	root.PersistentFlags().Bool("quiet", false, "no output; exit code carries the decision")
	root.PersistentFlags().Bool("verbose", false, "diagnostic output")

	root.AddCommand(newVerifyCmd())
	root.AddCommand(newPolicyCmd())
	root.AddCommand(newBuildersCmd())
	root.AddCommand(newEvidenceCmd())
	root.AddCommand(newAuditCmd())
	root.AddCommand(newBlockchainCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newInspectCmd())
	root.AddCommand(newVersionCmd())
	return root
}

// Execute runs the CLI; main() maps exitErr to the process exit code.
func Execute() int {
	root := NewRoot()
	if err := root.Execute(); err != nil {
		if e, ok := err.(*exitErr); ok {
			return e.code
		}
		return 4
	}
	return 0
}
