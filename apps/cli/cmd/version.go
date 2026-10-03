package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI version",
		RunE: func(cmd *cobra.Command, args []string) error {
			if useJSON(cmd) {
				return emit(cmd, "", map[string]string{"version": version})
			}
			if !useQuiet(cmd) {
				fmt.Fprintf(cmd.OutOrStdout(), "quorum %s\n", version)
			}
			return nil
		},
	}
}
