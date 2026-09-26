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

func newNovelNearCmd(flags *rootFlags) *cobra.Command {
	var flagZip string
	var flagMiles string
	var flagType string

	cmd := &cobra.Command{
		Use:         "near",
		Short:       "Find paid clinicians within N miles of a ZIP code.",
		Example:     "  openpayments-pp-cli near --zip 19002 --miles 25 --type research --agent",
		Annotations: map[string]string{"mcp:read-only": "false", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "near")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "near")
		},
	}
	cmd.Flags().StringVar(&flagZip, "zip", "", "TODO: describe --zip")
	cmd.Flags().StringVar(&flagMiles, "miles", "", "TODO: describe --miles")
	cmd.Flags().StringVar(&flagType, "type", "", "TODO: describe --type")
	return cmd
}
