package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/cliutil"
	"openpayments-pp-cli/internal/op"
	"openpayments-pp-cli/internal/store"
)

// Open Payments scoped sync. The framework sync command walks generic list
// resources; CMS data needs year × type × filter scoping instead, so this
// hook adds the domain flags to `sync` and routes to op.Sync when any is set.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		syncCmd, _, err := root.Find([]string{"sync"})
		if err != nil || syncCmd == nil || syncCmd.Name() != "sync" {
			return
		}
		var years, types, states, specialty, npis, companies, typ string
		var year int
		var bulk, force bool
		syncCmd.Flags().StringVar(&years, "years", "", "Program years to sync, e.g. 2024, 2019-2025 or 2023,2025")
		syncCmd.Flags().IntVar(&year, "year", 0, "Single program year (shorthand for --years; used with --full --type for bulk CSV)")
		syncCmd.Flags().StringVar(&types, "types", "", "Payment types: general,research,ownership (default all three)")
		syncCmd.Flags().StringVar(&typ, "type", "", "Single payment type (used with --full for bulk CSV)")
		syncCmd.Flags().StringVar(&states, "states", "", "Recipient states to sync, e.g. PA,NJ")
		syncCmd.Flags().StringVar(&specialty, "specialty", "", "Specialty substrings, e.g. \"Pain Medicine,Orthopaedic Surgery\"")
		syncCmd.Flags().StringVar(&npis, "npi", "", "Recipient NPIs (comma list); research also matches PI slots 1-5")
		syncCmd.Flags().StringVar(&companies, "company", "", "Paying company names; loose names like 'stryker' resolve to CMS spellings")
		syncCmd.Flags().BoolVar(&bulk, "bulk", false, "With --full: stream the CMS bulk CSV instead of paging the API")
		syncCmd.Flags().BoolVar(&force, "refresh", false, "Re-sync scopes even when the CMS dataset modified date is unchanged")
		syncCmd.Long += `

Open Payments scoping (recommended; national years exceed 15M rows):
  sync --years 2019-2025 --types general,research,ownership --states PA,NJ
  sync --years 2024 --specialty "Pain Medicine,Orthopaedic Surgery"
  sync --years 2019-2025 --npi 1234567890
  sync --years 2024 --company "Stryker Corporation"
  sync --full --year 2024 --type research        # bulk CSV, not the API

Scopes whose CMS dataset 'modified' date is unchanged are skipped; pass
--refresh to force. Every run records added/amended/deleted rows for
'changed --since-last-sync'.`
		orig := syncCmd.RunE
		syncCmd.RunE = func(cmd *cobra.Command, args []string) error {
			domain := years != "" || year != 0 || types != "" || typ != "" || states != "" || specialty != "" || npis != "" || companies != ""
			if !domain {
				if orig != nil {
					return orig(cmd, args)
				}
				return cmd.Help()
			}
			full, _ := cmd.Flags().GetBool("full")
			concurrency, _ := cmd.Flags().GetInt("concurrency")
			maxPages, _ := cmd.Flags().GetInt("max-pages")
			if year != 0 && years == "" {
				years = strconv.Itoa(year)
			}
			if typ != "" && types == "" {
				types = typ
			}
			yl, err := op.ParseYears(years)
			if err != nil {
				return usageErr(err)
			}
			if len(yl) == 0 {
				return usageErr(fmt.Errorf("--years is required for a scoped sync (e.g. --years 2024)"))
			}
			tl, err := op.ParseTypes(types)
			if err != nil {
				return usageErr(err)
			}
			scope := op.Scope{Years: yl, Types: tl, States: upperList(splitCSVFlag(states)), Specialties: splitCSVFlag(specialty), NPIs: splitCSVFlag(npis)}
			if full && (bulk || scope.Empty()) {
				return runBulkSync(cmd, flags, scope)
			}
			if scope.Empty() {
				return usageErr(fmt.Errorf("refusing an unscoped API sync (15M+ rows per year): add --states, --specialty, --npi or --company, or use --full --bulk for the CMS bulk CSV"))
			}
			if dryRunOK(flags) {
				flags.dryRun = true
			}
			ctx, cancel := boundSyncCtx(cmd, flags)
			defer cancel()
			st, db, err := openOPStore(ctx)
			if err != nil {
				return err
			}
			reg, err := resolveRegistry(ctx, flags, db, false)
			if err != nil {
				return err
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if cs := splitCSVFlag(companies); len(cs) > 0 {
				if err := op.EnsureCompanyProfiles(ctx, c, db, reg, false); err != nil {
					return apiErr(fmt.Errorf("loading company profiles: %w", err))
				}
				resolved, matches, err := op.ResolveCompanies(db, cs)
				if err != nil {
					return err
				}
				for term, names := range matches {
					fmt.Fprintf(os.Stderr, "company %q → %d CMS name(s): %v\n", term, len(names), names)
				}
				scope.Companies = resolved
			}
			if cliutil.IsDogfoodEnv() && maxPages == 0 {
				maxPages = 1
			}
			progress := os.Stderr
			if flags.quiet {
				progress = nil
			}
			opts := op.SyncOptions{Concurrency: concurrency, Force: force || full, MaxPages: maxPages, DryRun: flags.dryRun}
			if progress != nil {
				opts.Progress = progress
			}
			report, err := op.Sync(ctx, c, db, reg, scope, opts)
			if err != nil {
				return apiErr(err)
			}
			recordFrameworkSyncState(st, report)
			if err := flags.printJSON(cmd, report); err != nil {
				return err
			}
			if report.Failed > 0 {
				return apiErr(fmt.Errorf("%d scope(s) failed; re-run the same command to resume (completed scopes are skipped)", report.Failed))
			}
			return nil
		}
		if syncCmd.Annotations == nil {
			syncCmd.Annotations = map[string]string{}
		}
	})
}

// boundSyncCtx gives syncs a generous overall deadline unless --timeout was set.
func boundSyncCtx(cmd *cobra.Command, flags *rootFlags) (context.Context, context.CancelFunc) {
	if flags.timeoutExplicit {
		return boundCtx(cmd.Context(), flags)
	}
	return context.WithTimeout(cmd.Context(), 12*time.Hour)
}

// recordFrameworkSyncState mirrors domain syncs into the framework sync_state
// table so generated stale/unsynced hints work for local commands.
func recordFrameworkSyncState(st *store.Store, report *op.SyncReport) {
	if st == nil || report == nil {
		return
	}
	counts := map[string]int{}
	for _, s := range report.Scopes {
		if s.Status == "synced" || s.Status == "unchanged" {
			counts["payments_"+s.Type] += s.Rows
		}
	}
	for res, n := range counts {
		_ = st.SaveSyncState(res, "", n)
	}
}
