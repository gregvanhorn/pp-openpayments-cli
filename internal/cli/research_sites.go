// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelResearchSitesCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var by string
	var minSponsors int
	var generalOnly bool
	cmd := &cobra.Command{
		Use:   "research-sites",
		Short: "Rank principal investigators or sites by research dollars, trials and sponsors",
		Long: `Rolls research payments up by principal investigator (default) or by site
(teaching hospital / paid entity). --state and --specialty match the PI.
Each row shows research dollars, distinct trials (NCT IDs), distinct sponsors,
years and the NCT list. --min-sponsors keeps proven multi-sponsor sites.
--general-only instead lists clinicians with general payments but no research
payments in the scope (not yet trial sites).

Use this command to rank sites and PIs for a specialty and region. Do NOT
use it for the PIs of one known trial; use 'investigators' instead. Do NOT
use it for raw research payment rows; use 'research' instead.`,
		Example: `  openpayments-pp-cli research-sites --specialty "Pain Medicine" --state PA,NJ --agent
  openpayments-pp-cli research-sites --state PA --min-sponsors 3
  openpayments-pp-cli research-sites --state NJ --by site --specialty "Orthopaedic Surgery"`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "research-sites")
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_research")
			if err != nil {
				return err
			}
			defer cancel()
			if generalOnly {
				w, a, err := f.where("p", "specialties", "amount")
				if err != nil {
					return err
				}
				q := `SELECT p.npi, MAX(p.recipient_name) name, MAX(p.specialty) specialty, MAX(p.city) city, MAX(p.state) state, ROUND(SUM(p.amount),2) general_total, COUNT(DISTINCT p.company) companies
					FROM payments_general p WHERE ` + w + ` AND p.npi IS NOT NULL
					AND NOT EXISTS (SELECT 1 FROM research_investigators i WHERE i.npi = p.npi) AND NOT EXISTS (SELECT 1 FROM payments_research r WHERE r.npi = p.npi)
					GROUP BY p.npi ORDER BY general_total DESC LIMIT ?`
				return runLocal(ctx, cmd, flags, db, q, append(a, f.limit)...)
			}
			pf := f
			pf.states, pf.specialties = "", ""
			w, a, err := pf.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			iw, ia, err := opFilter{states: f.states, specialties: f.specialties}.where("i", "specialty", "")
			if err != nil {
				return err
			}
			var key, cols string
			switch by {
			case "pi":
				key = "COALESCE(i.npi, i.profile_id, i.name)"
				cols = "MAX(i.npi) npi, MAX(i.name) name, MAX(i.specialty) specialty, MAX(i.city) city, MAX(i.state) state, GROUP_CONCAT(DISTINCT UPPER(COALESCE(p.teaching_hospital_name, p.noncovered_entity, p.recipient_name))) sites"
			case "site":
				key = "COALESCE(p.teaching_hospital_ccn, p.noncovered_entity, p.npi, p.recipient_name)"
				cols = "MAX(COALESCE(p.teaching_hospital_name, p.noncovered_entity, p.recipient_name)) site, MAX(p.teaching_hospital_ccn) ccn, MAX(p.city) city, MAX(p.state) state, COUNT(DISTINCT i.npi) investigators"
			default:
				return usageErr(fmt.Errorf("--by must be pi or site"))
			}
			q := `SELECT ` + cols + `, ROUND(SUM(p.amount),2) research_total, COUNT(DISTINCT p.nct_id) trials, COUNT(DISTINCT p.company) sponsors,
				GROUP_CONCAT(DISTINCT p.company) sponsor_names, GROUP_CONCAT(DISTINCT p.nct_id) nct_ids, MIN(p.program_year) first_year, MAX(p.program_year) last_year, MAX(p.dataset_modified) dataset_modified
				FROM research_investigators i JOIN payments_research p ON p.record_id = i.record_id AND p.program_year = i.program_year
				WHERE ` + w + ` AND ` + iw + ` GROUP BY ` + key + ` HAVING sponsors >= ? ORDER BY research_total DESC LIMIT ?`
			return runLocal(ctx, cmd, flags, db, q, append(append(append([]any{}, a...), ia...), minSponsors, f.limit)...)
		},
	}
	addFilterFlags(cmd, &f, 25)
	cmd.Flags().StringVar(&by, "by", "pi", "Roll up by pi or site")
	cmd.Flags().IntVar(&minSponsors, "min-sponsors", 0, "Only PIs/sites paid by at least this many research sponsors")
	cmd.Flags().BoolVar(&generalOnly, "general-only", false, "List clinicians with general but no research payments instead")
	_ = strings.ToLower
	return cmd
}
