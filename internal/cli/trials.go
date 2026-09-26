// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/op"
)

func newNovelTrialsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trials",
		Short: "Join ClinicalTrials.gov studies with Open Payments research PIs",
		Long: `ClinicalTrials.gov (live, cached 7 days) joined on NCT ID and sponsor with
the local Open Payments research payments. Sponsor-to-manufacturer matching
is by normalized name and every result prints the matched pairs; use
--sponsor-alias "CT name=OP name" to correct a miss.`,
		Example:     "  openpayments-pp-cli trials gaps --condition 'low back pain' --state PA,NJ --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newTrialsSitesCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelTrialsGapsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelTrialsSponsorCmd(flags))
	return cmd
}

func newTrialsSitesCmd(flags *rootFlags) *cobra.Command {
	var nct string
	var refresh bool
	cmd := &cobra.Command{
		Use:   "sites",
		Short: "Show a trial's status, phase, sponsor and locations plus every Open Payments PI paid for it",
		Long: `Fetches one study from ClinicalTrials.gov and lists its locations beside the
principal investigators Open Payments shows were paid for that NCT ID
(from the local store). paid_pi_nearby marks locations whose city and state
match a paid PI.

Use this command for where one trial runs and who was paid on it. Do NOT use
it to rank sites across trials; use 'research-sites' instead.`,
		Example:     "  openpayments-pp-cli trials sites --nct NCT04280705 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && nct == "" {
				nct = args[0]
			}
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "trials sites")
			}
			if nct == "" {
				return usageErr(fmt.Errorf("--nct is required"))
			}
			nct = strings.ToUpper(nct)
			ctx, cancel, db, err := trialsCtx(cmd, flags)
			if err != nil {
				return err
			}
			defer cancel()
			t, ok := op.CachedTrial(db, nct, trialCacheAge)
			if !ok || refresh {
				if flags.dataSource == "local" {
					if !ok {
						return notFoundErr(fmt.Errorf("%s is not cached; drop --data-source local to fetch it", nct))
					}
				} else {
					t, err = newCTGov(flags).Study(ctx, nct)
					if err != nil {
						return apiErr(err)
					}
					if err := op.CacheTrials(db, []op.Trial{t}); err != nil {
						return err
					}
				}
			}
			pis, err := queryArgs(ctx, db, `SELECT MAX(i.npi) npi, MAX(i.name) name, MAX(i.specialty) specialty, MAX(i.city) city, MAX(i.state) state,
				ROUND(SUM(p.amount),2) total, GROUP_CONCAT(DISTINCT p.company) sponsors, MIN(p.program_year) first_year, MAX(p.program_year) last_year
				FROM research_investigators i JOIN payments_research p ON p.record_id=i.record_id AND p.program_year=i.program_year
				WHERE p.nct_id = ? GROUP BY COALESCE(i.npi, i.name) ORDER BY total DESC`, nct)
			if err != nil {
				return err
			}
			paid := map[string]bool{}
			for _, p := range pis {
				paid[strings.ToLower(fmt.Sprint(p["city"]))+"|"+fmt.Sprint(p["state"])] = true
			}
			var locs []map[string]any
			for _, l := range t.Locations {
				locs = append(locs, map[string]any{"facility": l.Facility, "city": l.City, "state": l.State, "zip5": l.Zip5, "country": l.Country, "status": l.Status,
					"paid_pi_nearby": paid[strings.ToLower(l.City)+"|"+l.State]})
			}
			return flags.printJSON(cmd, map[string]any{"trial": map[string]any{"nct_id": t.NCTID, "title": t.Title, "status": t.Status, "phase": t.Phase, "sponsor": t.Sponsor, "sponsor_class": t.SponsorClass, "conditions": t.Conditions, "start_date": t.StartDate},
				"locations": locs, "open_payments_pis": pis})
		},
	}
	cmd.Flags().StringVar(&nct, "nct", "", "ClinicalTrials.gov identifier (NCT########)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Re-fetch from ClinicalTrials.gov even if cached")
	return cmd
}
