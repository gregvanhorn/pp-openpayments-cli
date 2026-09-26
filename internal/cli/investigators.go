// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelInvestigatorsCmd(flags *rootFlags) *cobra.Command {
	var nct string
	var f opFilter
	cmd := &cobra.Command{
		Use:   "investigators",
		Short: "List every principal investigator paid for one ClinicalTrials.gov study, deduplicated",
		Long: `Flattens principal investigator slots 1-5 on every synced research payment
that cites --nct into one row per PI with dollars, sponsors, years and
location.

Use this command to list who Open Payments shows was paid as PI on one trial.
Do NOT use it for where the trial runs per ClinicalTrials.gov; use 'trials
sites' instead. Do NOT use it for raw payment rows; use 'research' instead.`,
		Example:     "  openpayments-pp-cli investigators --nct NCT04280705 --agent\n  openpayments-pp-cli investigators --nct NCT05665088 --state NJ",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && nct == "" {
				nct = args[0]
			}
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "investigators")
			}
			if nct == "" {
				return usageErr(fmt.Errorf("--nct is required (e.g. --nct NCT04280705)"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_research")
			if err != nil {
				return err
			}
			defer cancel()
			iw, ia, err := opFilter{states: f.states, specialties: f.specialties, years: f.years}.where("i", "specialty", "")
			if err != nil {
				return err
			}
			iw = strings.Replace(iw, "i.program_year", "p.program_year", 1)
			pw, pa, err := opFilter{companies: f.companies, minAmount: f.minAmount}.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			iw += " AND " + pw
			ia = append(ia, pa...)
			q := `SELECT MAX(i.npi) npi, MAX(i.name) name, MAX(i.specialty) specialty, MAX(i.city) city, MAX(i.state) state, MAX(i.zip5) zip5,
				COUNT(*) payments, ROUND(SUM(p.amount),2) total, GROUP_CONCAT(DISTINCT p.company) sponsors,
				GROUP_CONCAT(DISTINCT UPPER(COALESCE(p.teaching_hospital_name, p.noncovered_entity, p.recipient_name))) paid_entities,
				MIN(p.program_year) first_year, MAX(p.program_year) last_year, MAX(p.name_of_study) name_of_study, MAX(p.dataset_modified) dataset_modified
				FROM research_investigators i JOIN payments_research p ON p.record_id = i.record_id AND p.program_year = i.program_year
				WHERE p.nct_id = ? AND ` + iw + ` GROUP BY COALESCE(i.npi, i.profile_id, i.name) ORDER BY total DESC LIMIT ?`
			return runLocal(ctx, cmd, flags, db, q, append(append([]any{strings.ToUpper(nct)}, ia...), f.limit)...)
		},
	}
	cmd.Flags().StringVar(&nct, "nct", "", "ClinicalTrials.gov identifier (NCT########)")
	addFilterFlags(cmd, &f, 100)
	return cmd
}
