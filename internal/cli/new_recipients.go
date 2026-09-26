// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNovelNewRecipientsCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	cmd := &cobra.Command{
		Use:   "new-recipients",
		Short: "List clinicians paid this year for the first time in any synced year",
		Long: `Recipients (by NPI) with general or research payments in --year (default:
latest synced year) and none in any earlier synced year. With --company,
"new" means new to that company. Results are only as deep as your synced
history: prior_years_checked lists the years compared.

Use this command for first-time relationships in a year. Do NOT use it for
existing relationships that grew; use 'rising' instead.`,
		Example: `  openpayments-pp-cli new-recipients --year 2025 --state PA --agent
  openpayments-pp-cli new-recipients --year 2025 --company stryker --specialty "Orthopaedic Surgery"`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "new-recipients")
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			year, err := targetYear(ctx, db, &f)
			if err != nil {
				return err
			}
			prior, err := queryArgs(ctx, db, `SELECT DISTINCT program_year FROM payments_general WHERE program_year < ? ORDER BY program_year`, year)
			if err != nil {
				return err
			}
			if len(prior) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: no synced years before %d; every recipient looks new. Sync earlier years for the same scope first.\n", year)
			}
			w, a, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			all := `SELECT npi, company, program_year, amount, recipient_name name, specialty, city, state FROM payments_general p WHERE ` + w + ` AND npi IS NOT NULL
				UNION ALL SELECT npi, company, program_year, amount, recipient_name, specialty, city, state FROM payments_research p WHERE ` + w + ` AND npi IS NOT NULL`
			q := `WITH a AS (` + all + `)
				SELECT cur.npi, cur.name, cur.specialty, cur.city, cur.state, cur.companies, cur.payments, ROUND(cur.total,2) total, ? program_year,
				  (SELECT GROUP_CONCAT(DISTINCT program_year) FROM payments_general WHERE program_year < ?) prior_years_checked
				FROM (SELECT npi, MAX(name) name, MAX(specialty) specialty, MAX(city) city, MAX(state) state, COUNT(*) payments, SUM(amount) total, GROUP_CONCAT(DISTINCT company) companies FROM a WHERE program_year = ? GROUP BY npi) cur
				WHERE NOT EXISTS (SELECT 1 FROM a prev WHERE prev.npi = cur.npi AND prev.program_year < ?)
				ORDER BY total DESC LIMIT ?`
			args2 := append(append(append([]any{}, a...), a...), year, year, year, year, f.limit)
			return runLocal(ctx, cmd, flags, db, q, args2...)
		},
	}
	addFilterFlags(cmd, &f, 50)
	return cmd
}
