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

func newNovelResearchSitesCmd(flags *rootFlags) *cobra.Command {
	var flagSpecialty string
	var flagState string

	cmd := &cobra.Command{
		Use:         "research-sites",
		Short:       "Rank sites and principal investigators by research dollars, trials and sponsors for a specialty and region.",
		Example:     "  openpayments-pp-cli research-sites --specialty 'Pain Medicine' --state PA,NJ --agent",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "research-sites")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "research-sites")
		},
	}
	cmd.Flags().StringVar(&flagSpecialty, "specialty", "", "TODO: describe --specialty")
	cmd.Flags().StringVar(&flagState, "state", "", "TODO: describe --state")
	return cmd
}
