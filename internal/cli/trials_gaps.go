// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/cliutil"
	"openpayments-pp-cli/internal/op"
)

func newNovelTrialsGapsCmd(flags *rootFlags) *cobra.Command {
	var condition, states, status string
	var aliases []string
	var allSponsors, includeCovered bool
	var limit, maxPages int
	cmd := &cobra.Command{
		Use:   "gaps",
		Short: "Recruiting trials in your region whose sponsor pays no local principal investigator",
		Long: `Searches ClinicalTrials.gov for trials matching --condition with a location in
--state, then checks the local Open Payments research data: a trial is a gap
when its lead sponsor (matched to Open Payments manufacturer names) paid no
principal investigator located in those states in any synced year. Industry
sponsors only by default, since non-industry sponsors do not report to Open
Payments. Accuracy depends on your synced research scope; sync research for
the region first.

Use this command to find recruiting trials that lack a paid local PI. Do NOT
use it for ranking existing sites; use 'research-sites' instead. Do NOT use
it for one trial's locations; use 'trials sites' instead.`,
		Example: `  openpayments-pp-cli trials gaps --condition "low back pain" --state PA,NJ --agent
  openpayments-pp-cli trials gaps --condition "spinal cord stimulation" --state PA --include-covered`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "trials gaps")
			}
			region := upperList(splitCSVFlag(states))
			if condition == "" || len(region) == 0 {
				return usageErr(fmt.Errorf("--condition and --state are required"))
			}
			ctx, cancel, db, err := trialsCtx(cmd, flags)
			if err != nil {
				return err
			}
			defer cancel()
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			ct := newCTGov(flags)
			seen := map[string]op.Trial{}
			for _, st := range region {
				name := op.StateName(st)
				if name == "" {
					return usageErr(fmt.Errorf("unknown state code %q", st))
				}
				trials, err := ct.Search(ctx, op.SearchParams{Condition: condition, Location: name, Statuses: splitCSVFlag(status), MaxPages: maxPages})
				if err != nil {
					return apiErr(err)
				}
				for _, t := range trials {
					seen[t.NCTID] = t
				}
			}
			var all []op.Trial
			for _, t := range seen {
				all = append(all, t)
			}
			if err := op.CacheTrials(db, all); err != nil {
				return err
			}
			companies, err := researchCompanies(ctx, db)
			if err != nil {
				return err
			}
			alias := parseAliases(aliases)
			inRegion := map[string]bool{}
			for _, s := range region {
				inRegion[s] = true
			}
			var out []map[string]any
			for _, t := range all {
				if !allSponsors && t.SponsorClass != "INDUSTRY" {
					continue
				}
				var sites []string
				for _, l := range t.Locations {
					if inRegion[l.State] && (l.Status == "" || l.Status == "RECRUITING" || l.Status == "NOT_YET_RECRUITING") {
						sites = append(sites, fmt.Sprintf("%s (%s, %s)", l.Facility, l.City, l.State))
					}
				}
				if len(sites) == 0 {
					continue
				}
				matched := op.MatchCompanies(t.Sponsor, companies, alias)
				localPIs, trialPIs := 0, 0
				if len(matched) > 0 {
					args := []any{}
					for _, m := range matched {
						args = append(args, m)
					}
					for _, s := range region {
						args = append(args, s)
					}
					_ = db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT COALESCE(i.npi, i.name)) FROM research_investigators i JOIN payments_research p ON p.record_id=i.record_id AND p.program_year=i.program_year
						WHERE p.company IN (`+qmarks(len(matched))+`) AND i.state IN (`+qmarks(len(region))+`)`, args...).Scan(&localPIs)
				}
				regionArgs := []any{t.NCTID}
				for _, s := range region {
					regionArgs = append(regionArgs, s)
				}
				_ = db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT COALESCE(i.npi, i.name)) FROM research_investigators i JOIN payments_research p ON p.record_id=i.record_id AND p.program_year=i.program_year
					WHERE p.nct_id = ? AND i.state IN (`+qmarks(len(region))+`)`, regionArgs...).Scan(&trialPIs)
				gap := localPIs == 0
				if !gap && !includeCovered {
					continue
				}
				out = append(out, map[string]any{
					"nct_id": t.NCTID, "title": t.Title, "status": t.Status, "phase": t.Phase, "sponsor": t.Sponsor, "sponsor_class": t.SponsorClass,
					"conditions": t.Conditions, "in_region_sites": strings.Join(sites, "; "), "in_region_site_count": len(sites),
					"matched_open_payments_names": strings.Join(matched, "; "), "sponsor_matched": len(matched) > 0,
					"local_pis_paid_by_sponsor": localPIs, "local_pis_paid_for_this_trial": trialPIs, "gap": gap,
				})
			}
			sort.SliceStable(out, func(i, j int) bool {
				return toInt(out[i]["in_region_site_count"]) > toInt(out[j]["in_region_site_count"])
			})
			if limit > 0 && len(out) > limit {
				out = out[:limit]
			}
			return printRows(cmd, flags, out)
		},
	}
	cmd.Flags().StringVar(&condition, "condition", "", "Condition or disease, e.g. \"low back pain\"")
	cmd.Flags().StringVar(&states, "state", "", "Region as USPS codes, e.g. PA,NJ")
	cmd.Flags().StringVar(&status, "status", "RECRUITING,NOT_YET_RECRUITING", "ClinicalTrials.gov overall statuses")
	cmd.Flags().StringArrayVar(&aliases, "sponsor-alias", nil, "Map a ClinicalTrials.gov sponsor to an Open Payments name: \"CT name=OP name\"")
	cmd.Flags().BoolVar(&allSponsors, "all-sponsors", false, "Include non-industry sponsors (they never appear in Open Payments)")
	cmd.Flags().BoolVar(&includeCovered, "include-covered", false, "Also list trials whose sponsor already pays a local PI")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum trials")
	cmd.Flags().IntVar(&maxPages, "max-pages", 5, "ClinicalTrials.gov result pages per state (100 studies each)")
	return cmd
}
