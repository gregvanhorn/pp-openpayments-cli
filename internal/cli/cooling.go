// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import "github.com/spf13/cobra"

func newNovelCoolingCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var minDelta float64
	cmd := &cobra.Command{
		Use:   "cooling",
		Short: "Find recipient-company pairs whose dollars fell or stopped year over year",
		Long: `Ranks recipient × company pairs by dollar decline from the prior program year
to --year (default: latest synced year), including pairs that dropped to $0
(status 'stopped').

Use this command for relationships whose dollars fell or stopped. Do NOT use
it for growth; use 'rising' instead.`,
		Example:     "  openpayments-pp-cli cooling --state PA --year 2025 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cooling")
			}
			return runYoY(cmd, flags, f, -1, false, minDelta)
		},
	}
	addFilterFlags(cmd, &f, 20)
	cmd.Flags().Float64Var(&minDelta, "min-delta", 0, "Only pairs that fell by at least this many USD")
	return cmd
}
