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

func newNovelAskCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "ask",
		Short:       "Print the local schema, a plain-word glossary and worked SQL examples for any question.",
		Example:     "  openpayments-pp-cli ask 'top companies paying PA pain doctors'",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:novel-scaffold": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "ask")
			}
			// validate required flags here
			return fmt.Errorf("TODO: implement novel feature %q", "ask")
		},
	}
	return cmd
}
