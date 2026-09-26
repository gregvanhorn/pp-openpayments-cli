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

func newNovelRisingCmd(flags *rootFlags) *cobra.Command {
	var flagState string
	var flagYear string
	var flagLimit string

	cmd := &cobra.Command{
		Use:         "rising",
		Short:       "Find recipient-company pairs whose dollars grew the most year over year.",
		Example:     "  openpayments-pp-cli rising --state PA --year 2025 --limit 20 --agent",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "rising")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "rising")
		},
	}
	cmd.Flags().StringVar(&flagState, "state", "", "TODO: describe --state")
	cmd.Flags().StringVar(&flagYear, "year", "", "TODO: describe --year")
	cmd.Flags().StringVar(&flagLimit, "limit", "", "TODO: describe --limit")
	return cmd
}
