// Copyright 2026 Claude and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import "github.com/spf13/cobra"

func newNovelRisingCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var includeNew bool
	var minDelta float64
	cmd := &cobra.Command{
		Use:   "rising",
		Short: "Find recipient-company pairs whose dollars grew the most year over year",
		Long: `Ranks recipient × company pairs by dollar increase from the prior program
year to --year (default: latest synced year). Exact sums from the local store.

Use this command for relationships whose dollars grew year over year. Do NOT
use it for brand-new relationships; use 'new-recipients' instead. Do NOT use
it for declines; use 'cooling' instead.`,
		Example:     "  openpayments-pp-cli rising --state PA --year 2025 --limit 20 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "rising")
			}
			return runYoY(cmd, flags, f, 1, includeNew, minDelta)
		},
	}
	addFilterFlags(cmd, &f, 20)
	cmd.Flags().BoolVar(&includeNew, "include-new", false, "Also rank pairs with no prior-year payments")
	cmd.Flags().Float64Var(&minDelta, "min-delta", 0, "Only pairs that grew by at least this many USD")
	return cmd
}
