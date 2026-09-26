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

func newNovelRelationshipsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "relationships",
		Short:       "Show every company that paid a clinician with first year, last year, total and trend.",
		Example:     "  openpayments-pp-cli relationships 1234567890 --agent",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "relationships")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "relationships")
		},
	}
	return cmd
}
