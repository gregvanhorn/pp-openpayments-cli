package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/cliutil"
	"openpayments-pp-cli/internal/op"
	"openpayments-pp-cli/internal/store"
)

// Open Payments scoped sync. The framework sync command walks generic list
// resources; CMS data needs year × type × filter scoping instead, so this
// hook adds the domain flags to `sync` and routes to op.Sync when any is set.
// opSyncFlags are the Open Payments scoping flags declared on `sync`.
type opSyncFlags struct {
	years, types, typ, states, specialty, npis, companies string
	year                                                  int
	bulk, refresh                                         bool
}

func (o *opSyncFlags) requested() bool {
	return o.years != "" || o.year != 0 || o.types != "" || o.typ != "" || o.states != "" || o.specialty != "" || o.npis != "" || o.companies != ""
}

// Open Payments scoping help, appended to the framework sync command.
func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		syncCmd, _, err := root.Find([]string{"sync"})
		if err != nil || syncCmd == nil || syncCmd.Name() != "sync" {
			return
		}
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
		syncCmd.Example = strings.Trim(`
  openpayments-pp-cli sync --years 2023-2025 --types general,research,ownership --states PA,NJ
  openpayments-pp-cli sync --years 2019-2025 --npi 1234567890
  openpayments-pp-cli sync --full --year 2024 --type ownership`, "\n")
	})
}

// runOPSync runs a scoped Open Payments sync.
func runOPSync(cmd *cobra.Command, flags *rootFlags, o *opSyncFlags) error {
	full, _ := cmd.Flags().GetBool("full")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	maxPages, _ := cmd.Flags().GetInt("max-pages")
	years, types := o.years, o.types
	if o.year != 0 && years == "" {
		years = strconv.Itoa(o.year)
	}
	if o.typ != "" && types == "" {
		types = o.typ
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
	scope := op.Scope{Years: yl, Types: tl, States: upperList(splitCSVFlag(o.states)), Specialties: splitCSVFlag(o.specialty), NPIs: splitCSVFlag(o.npis)}
	if full && (o.bulk || scope.Empty()) {
		return runBulkSync(cmd, flags, scope)
	}
	if scope.Empty() && len(splitCSVFlag(o.companies)) == 0 {
		return usageErr(fmt.Errorf("refusing an unscoped API sync (15M+ rows per year): add --states, --specialty, --npi or --company, or use --full --bulk for the CMS bulk CSV"))
	}
	if dryRunOK(flags) {
		return flags.printJSON(cmd, map[string]any{"dry_run": true, "action": "sync", "scope": scope})
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
	if cs := splitCSVFlag(o.companies); len(cs) > 0 {
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
	opts := op.SyncOptions{Concurrency: concurrency, Force: o.refresh || full, MaxPages: maxPages}
	if !flags.quiet {
		opts.Progress = os.Stderr
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
