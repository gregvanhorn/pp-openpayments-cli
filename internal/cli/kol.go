// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// kolNatureSQL is the fixed, published definition of speaking + consulting.
const kolNatureSQL = `(nature = 'Consulting Fee' OR nature LIKE 'Compensation for%' OR nature = 'Honoraria')`

func newNovelKolCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var sortBy string
	var physiciansOnly bool
	cmd := &cobra.Command{
		Use:   "kol",
		Short: "Rank physicians in a specialty and region by speaking and consulting dollars",
		Long: `Ranks recipients by speaking + consulting dollars (natures: Consulting Fee,
every "Compensation for..." speaker/faculty category, Honoraria), with the
number of distinct paying companies and years active shown alongside. No
hidden composite score: pick the sort key with --sort. Reports published
dollars only; it does not characterize any relationship.

Use this command to rank physicians by speaking and consulting activity. Do
NOT use it for all-nature dollar leaderboards; use 'top' instead.`,
		Example: `  openpayments-pp-cli kol --specialty "Physical Medicine" --state PA --agent
  openpayments-pp-cli kol --specialty "Orthopaedic Surgery" --state PA,NJ --sort companies --limit 25`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "kol")
			}
			order := map[string]string{"dollars": "speaking_consulting_total", "companies": "companies", "years": "years_active"}[sortBy]
			if order == "" {
				return usageErr(fmt.Errorf("--sort must be dollars, companies or years"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			w, a, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			if physiciansOnly {
				w += " AND p.recipient_type LIKE '%Physician' AND p.recipient_type NOT LIKE '%Non-Physician%'"
			}
			// Aggregate on covered columns (ix_gen_cover); name/specialty/city
			// come from the derived recipients table.
			q := `SELECT g.npi, r.name, r.specialty, r.city, g.state, g.speaking_consulting_total, g.consulting_total, g.speaking_total,
				g.companies, g.years_active, g.all_general_total, g.first_year, g.last_year FROM (
				SELECT npi, MAX(state) state,
				ROUND(SUM(CASE WHEN ` + kolNatureSQL + ` THEN amount ELSE 0 END),2) speaking_consulting_total,
				ROUND(SUM(CASE WHEN nature = 'Consulting Fee' THEN amount ELSE 0 END),2) consulting_total,
				ROUND(SUM(CASE WHEN nature LIKE 'Compensation for%' OR nature = 'Honoraria' THEN amount ELSE 0 END),2) speaking_total,
				COUNT(DISTINCT CASE WHEN ` + kolNatureSQL + ` THEN company END) companies,
				COUNT(DISTINCT CASE WHEN ` + kolNatureSQL + ` THEN program_year END) years_active,
				ROUND(SUM(amount),2) all_general_total, MIN(program_year) first_year, MAX(program_year) last_year
				FROM payments_general p WHERE ` + w + ` AND npi IS NOT NULL GROUP BY npi
				HAVING speaking_consulting_total > 0 ORDER BY ` + order + ` DESC, speaking_consulting_total DESC LIMIT ?) g
				LEFT JOIN recipients r ON r.recipient_key = g.npi ORDER BY g.` + order + ` DESC, g.speaking_consulting_total DESC`
			return runLocal(ctx, cmd, flags, db, q, append(a, f.limit)...)
		},
	}
	addFilterFlags(cmd, &f, 25)
	cmd.Flags().StringVar(&sortBy, "sort", "dollars", "Sort key: dollars, companies or years")
	cmd.Flags().BoolVar(&physiciansOnly, "physicians-only", false, "Exclude non-physician practitioners")
	return cmd
}
