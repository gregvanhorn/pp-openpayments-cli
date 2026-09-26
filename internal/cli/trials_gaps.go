// Copyright 2026 Claude and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNovelTrialsGapsCmd(flags *rootFlags) *cobra.Command {
	var flagCondition string
	var flagState string

	cmd := &cobra.Command{
		Use:         "gaps",
		Short:       "List recruiting trials in your region whose sponsor pays no local principal investigator.",
		Example:     "  openpayments-pp-cli trials gaps --condition 'low back pain' --state PA,NJ --agent",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "trials gaps")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "trials gaps")
		},
	}
	cmd.Flags().StringVar(&flagCondition, "condition", "", "TODO: describe --condition")
	cmd.Flags().StringVar(&flagState, "state", "", "TODO: describe --state")
	return cmd
}
