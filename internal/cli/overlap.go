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

func newNovelOverlapCmd(flags *rootFlags) *cobra.Command {
	var flagState string

	cmd := &cobra.Command{
		Use:         "overlap",
		Short:       "See which recipients two companies both pay, and who only one pays.",
		Example:     "  openpayments-pp-cli overlap 'Stryker Corporation' 'Zimmer Biomet' --state PA --agent",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "overlap")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "overlap")
		},
	}
	cmd.Flags().StringVar(&flagState, "state", "", "TODO: describe --state")
	return cmd
}
