// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelChangedCmd(flags *rootFlags) *cobra.Command {
	var sinceLast, cms bool
	var run int
	var limit int
	cmd := &cobra.Command{
		Use:   "changed",
		Short: "List records added, corrected or removed since the previous sync",
		Long: `Lists per-record changes the sync engine detected: 'added' (new record in a
scope synced before), 'amended' (amount or dispute status changed) and
'removed' (no longer returned in that scope: deleted by CMS or moved out of it). --since-last-sync shows the most
recent run that touched data; --run picks a specific run; --cms instead
lists rows CMS itself flags with change_type NEW/CHANGED in the synced data.`,
		Example: `  openpayments-pp-cli changed --since-last-sync --agent
  openpayments-pp-cli changed --cms --limit 50`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "changed")
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "")
			if err != nil {
				return err
			}
			defer cancel()
			if cms {
				return runLocal(ctx, cmd, flags, db, `SELECT 'general' payment_type, record_id, program_year, change_type, npi, recipient_name, company, amount, dataset_modified FROM payments_general WHERE change_type NOT IN ('UNCHANGED','')
					UNION ALL SELECT 'research', record_id, program_year, change_type, npi, recipient_name, company, amount, dataset_modified FROM payments_research WHERE change_type NOT IN ('UNCHANGED','')
					UNION ALL SELECT 'ownership', record_id, program_year, change_type, npi, recipient_name, company, amount_invested, dataset_modified FROM payments_ownership WHERE change_type NOT IN ('UNCHANGED','')
					ORDER BY program_year DESC LIMIT ?`, limit)
			}
			runs, err := queryArgs(ctx, db, `SELECT sync_run, started_at, finished_at, rows_seen, added, amended, deleted, status, args FROM sync_runs ORDER BY sync_run DESC LIMIT 20`)
			if err != nil {
				return err
			}
			target := run
			if target == 0 && sinceLast {
				_ = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(sync_run),0) FROM sync_changes`).Scan(&target)
			}
			if target == 0 && len(runs) > 0 {
				target = int(toInt(runs[0]["sync_run"]))
			}
			var summary []map[string]any
			for _, r := range runs {
				if int(toInt(r["sync_run"])) == target {
					summary = append(summary, r)
				}
			}
			counts, err := queryArgs(ctx, db, `SELECT change, payment_type, COUNT(*) records, ROUND(SUM(COALESCE(new_amount,0) - COALESCE(old_amount,0)),2) net_amount_change FROM sync_changes WHERE sync_run = ? GROUP BY change, payment_type`, target)
			if err != nil {
				return err
			}
			rows, err := queryArgs(ctx, db, `SELECT change, payment_type, record_id, program_year, npi, recipient_name, company, old_amount, new_amount, cms_change_type FROM sync_changes WHERE sync_run = ? ORDER BY ABS(COALESCE(new_amount,0) - COALESCE(old_amount,0)) DESC LIMIT ?`, target, limit)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, map[string]any{"sync_run": target, "run": summary, "counts": counts, "changes": rows, "note": "first sync of a scope records no per-row 'added' entries; re-syncs after a CMS refresh do"})
		},
	}
	cmd.Flags().BoolVar(&sinceLast, "since-last-sync", true, "Show the most recent sync run that changed data")
	cmd.Flags().IntVar(&run, "run", 0, "Show a specific sync run id")
	cmd.Flags().BoolVar(&cms, "cms", false, "List rows CMS flags as NEW/CHANGED instead")
	cmd.Flags().IntVar(&limit, "limit", 100, "Maximum change rows")
	return cmd
}
