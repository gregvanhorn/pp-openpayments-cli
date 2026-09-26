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

func newNovelRosterCmd(flags *rootFlags) *cobra.Command {
	var flagNpi string

	cmd := &cobra.Command{
		Use:         "roster",
		Short:       "Summarize payments for a whole list of NPIs in one pass, including disputed counts.",
		Example:     "  openpayments-pp-cli roster --npi 1234567890,1987654321 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "roster")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "roster")
		},
	}
	cmd.Flags().StringVar(&flagNpi, "npi", "", "TODO: describe --npi")
	return cmd
}
