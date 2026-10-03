package cmd

import (
	"fmt"

	"github.com/quorum/quorum/internal/exitcodes"
	"github.com/quorum/quorum/internal/runner"
	"github.com/spf13/cobra"
)

func newBuildersCmd() *cobra.Command {
	b := &cobra.Command{Use: "builders", Short: "Manage the local builder registry"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List registered builders",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, _ := cmd.Flags().GetString("registry")
			regs, err := runner.LoadRegistry(reg)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
				return &exitErr{code: exitcodes.Operational}
			}
			human := ""
			if len(regs) == 0 {
				human = "no builders registered\n"
			}
			for _, r := range regs {
				human += fmt.Sprintf("%s\tgroup=%s\tendpoint=%s\n", r.ID, r.IndependenceGroup, r.Endpoint)
			}
			return emit(cmd, human, regs)
		},
	}
	list.Flags().String("registry", "builders.json", "registry file")

	register := &cobra.Command{
		Use:   "register --id ID --group GROUP",
		Short: "Register a builder (independence group required)",
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, _ := cmd.Flags().GetString("registry")
			id, _ := cmd.Flags().GetString("id")
			group, _ := cmd.Flags().GetString("group")
			name, _ := cmd.Flags().GetString("name")
			endpoint, _ := cmd.Flags().GetString("endpoint")
			err := runner.Register(reg, runner.BuilderRegistration{
				ID: id, DisplayName: name, IndependenceGroup: group, Endpoint: endpoint,
			})
			if err != nil {
				if !useJSON(cmd) {
					fmt.Fprintf(cmd.ErrOrStderr(), "error: %v\n", err)
				}
				return &exitErr{code: exitcodes.InvalidInput}
			}
			return emit(cmd, fmt.Sprintf("registered %s (group %s)\n", id, group),
				map[string]string{"id": id, "independenceGroup": group})
		},
	}
	register.Flags().String("registry", "builders.json", "registry file")
	register.Flags().String("id", "", "builder id")
	register.Flags().String("group", "", "independence group (required)")
	register.Flags().String("name", "", "display name")
	register.Flags().String("endpoint", "", "endpoint")

	b.AddCommand(list, register)
	return b
}
