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

func newNovelTrialsSponsorCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "sponsor",
		Short:       "Show a sponsor's ClinicalTrials.gov trials with the sites it already pays.",
		Example:     "  openpayments-pp-cli trials sponsor Medtronic --agent",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "trials sponsor")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "trials sponsor")
		},
	}
	return cmd
}
