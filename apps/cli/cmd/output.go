package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// flags reads the shared output flags.
func useJSON(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

func useQuiet(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("quiet")
	return v
}

// emit prints v as JSON when --json is set, the human text otherwise,
// or nothing when --quiet (exit code carries the decision).
func emit(cmd *cobra.Command, human string, v any) error {
	if useJSON(cmd) {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}
	if useQuiet(cmd) {
		return nil
	}
	fmt.Fprint(cmd.OutOrStdout(), human)
	return nil
}

// exitErr carries a process exit code through Cobra's error path.
type exitErr struct{ code int }

func (e *exitErr) Error() string { return fmt.Sprintf("exit %d", e.code) }
