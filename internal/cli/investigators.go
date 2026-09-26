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

func newNovelInvestigatorsCmd(flags *rootFlags) *cobra.Command {
	var flagNct string

	cmd := &cobra.Command{
		Use:         "investigators",
		Short:       "List every principal investigator paid for one ClinicalTrials.gov study, deduplicated.",
		Example:     "  openpayments-pp-cli investigators --nct NCT04280705 --agent",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "investigators")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "investigators")
		},
	}
	cmd.Flags().StringVar(&flagNct, "nct", "", "TODO: describe --nct")
	return cmd
}
