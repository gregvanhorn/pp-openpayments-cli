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

func newNovelConcentrationCmd(flags *rootFlags) *cobra.Command {
	var flagYear string

	cmd := &cobra.Command{
		Use:         "concentration",
		Short:       "Measure how concentrated a company's spend is: top-10 share, HHI and recipients to reach 50% and 80%.",
		Example:     "  openpayments-pp-cli concentration 'Stryker Corporation' --year 2024 --agent",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "concentration")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "concentration")
		},
	}
	cmd.Flags().StringVar(&flagYear, "year", "", "TODO: describe --year")
	return cmd
}
