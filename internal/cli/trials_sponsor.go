// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/cliutil"
	"openpayments-pp-cli/internal/op"
)

// sponsorTrialPaymentsSQL joins one NCT ID to local research payments and
// the paid principal investigators on those payment records. Arguments: the
// NCT ID three times.
const sponsorTrialPaymentsSQL = `SELECT COUNT(*) payments, ROUND(COALESCE(SUM(p.amount),0),2) research_total,
(SELECT COUNT(DISTINCT COALESCE(i.npi,i.name)) FROM research_investigators i JOIN payments_research q ON q.record_id=i.record_id AND q.program_year=i.program_year WHERE q.nct_id = ?) paid_pis,
COALESCE((SELECT GROUP_CONCAT(DISTINCT i.name || ' (' || COALESCE(i.city,'') || ' ' || COALESCE(i.state,'') || ')') FROM research_investigators i JOIN payments_research q ON q.record_id=i.record_id AND q.program_year=i.program_year WHERE q.nct_id = ?), '') pi_names
FROM payments_research p WHERE p.nct_id = ?`

func newNovelTrialsSponsorCmd(flags *rootFlags) *cobra.Command {
	var status, states string
	var aliases []string
	var limit, maxPages int
	cmd := &cobra.Command{
		Use:   "sponsor <company>",
		Short: "List a sponsor's ClinicalTrials.gov trials with the sites and PIs it already pays",
		Long: `Lists the company's trials from ClinicalTrials.gov (default: active
statuses) and joins each NCT ID to local Open Payments research payments:
dollars, paid PI count and paid PIs. --state narrows to trials with a site in
those states.

Use this command for one sponsor's trial portfolio with paid PIs. Do NOT use
it for a company's general-payment footprint; use 'company' instead. Do NOT
use it for trials missing local PIs; use 'trials gaps' instead.`,
		Example: `  openpayments-pp-cli trials sponsor Medtronic --agent
  openpayments-pp-cli trials sponsor "Boston Scientific" --state PA,NJ`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "trials sponsor")
			}
			if len(args) == 0 {
				return usageErr(fmt.Errorf("a sponsor/company name is required"))
			}
			if flags.dataSource == "local" {
				return usageErr(fmt.Errorf("trials sponsor searches ClinicalTrials.gov live; there is no local-only equivalent (drop --data-source local)"))
			}
			sponsor := strings.Join(args, " ")
			ctx, cancel, db, err := trialsCtx(cmd, flags)
			if err != nil {
				return err
			}
			defer cancel()
			if cliutil.IsDogfoodEnv() {
				maxPages = 1
			}
			trials, err := newCTGov(flags).Search(ctx, op.SearchParams{Sponsor: sponsor, Statuses: splitCSVFlag(status), MaxPages: maxPages})
			if err != nil {
				return apiErr(err)
			}
			if err := op.CacheTrials(db, trials); err != nil {
				return err
			}
			companies, err := researchCompanies(ctx, db)
			if err != nil {
				return err
			}
			matched := op.MatchCompanies(sponsor, companies, parseAliases(aliases))
			region := map[string]bool{}
			for _, s := range upperList(splitCSVFlag(states)) {
				region[s] = true
			}
			if matched == nil {
				matched = []string{}
			}
			out := make([]map[string]any, 0)
			for _, t := range trials {
				if !op.SponsorMatches(sponsor, t.Sponsor) {
					continue // collaborator-only hits
				}
				var sites []string
				for _, l := range t.Locations {
					if len(region) == 0 || region[l.State] {
						sites = append(sites, l.City+" "+l.State)
					}
				}
				if len(region) > 0 && len(sites) == 0 {
					continue
				}
				rows, err := queryArgs(ctx, db, sponsorTrialPaymentsSQL, t.NCTID, t.NCTID, t.NCTID)
				if err != nil {
					return err
				}
				row := map[string]any{"nct_id": t.NCTID, "title": t.Title, "status": t.Status, "phase": t.Phase, "sponsor": t.Sponsor, "site_count": len(sites)}
				if len(rows) > 0 {
					for k, v := range rows[0] {
						row[k] = v
					}
				}
				out = append(out, row)
				if limit > 0 && len(out) >= limit {
					break
				}
			}
			if len(out) == 0 {
				return notFoundErr(fmt.Errorf("no match for sponsor %q among ClinicalTrials.gov lead sponsors with the requested statuses", sponsor))
			}
			return flags.printJSON(cmd, map[string]any{"sponsor": sponsor, "matched_open_payments_names": matched, "trials": out})
		},
	}
	cmd.Flags().StringVar(&status, "status", "RECRUITING,NOT_YET_RECRUITING,ACTIVE_NOT_RECRUITING,ENROLLING_BY_INVITATION", "ClinicalTrials.gov overall statuses")
	cmd.Flags().StringVar(&states, "state", "", "Only trials with a site in these states")
	cmd.Flags().StringArrayVar(&aliases, "sponsor-alias", nil, "Map a ClinicalTrials.gov sponsor to an Open Payments name: \"CT name=OP name\"")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum trials")
	cmd.Flags().IntVar(&maxPages, "max-pages", 3, "ClinicalTrials.gov result pages (100 studies each)")
	return cmd
}
