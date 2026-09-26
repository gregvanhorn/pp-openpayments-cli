// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

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
			// Prior synced years come from the sync bookkeeping, not a scan.
			prior, err := queryArgs(ctx, db, `SELECT DISTINCT program_year FROM sync_scopes WHERE payment_type IN ('general','research') AND program_year < ? ORDER BY program_year`, year)
			if err != nil {
				return err
			}
			if len(prior) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "hint: no synced years before %d; every recipient looks new. Sync earlier years for the same scope first.\n", year)
			}
			var years []string
			for _, r := range prior {
				years = append(years, fmt.Sprint(r["program_year"]))
			}
			w, a, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			y := itoa(year)
			// New = first payment year in scope is --year: one grouped pass
			// with HAVING MIN(program_year), over the general_pairs rollup
			// unless a per-payment --min-amount needs the raw rows.
			general := `SELECT npi, company, program_year, amount, 1 n, state FROM payments_general p WHERE ` + w + ` AND npi IS NOT NULL AND program_year <= ` + y
			var rolled int
			_ = db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM general_pairs)`).Scan(&rolled)
			if f.minAmount == 0 && rolled == 1 {
				gw, ga, err := f.where("p", "specialties", "")
				if err != nil {
					return err
				}
				w, a = gw, ga
				general = `SELECT npi, company, program_year, total amount, n, state FROM general_pairs p WHERE ` + gw + ` AND program_year <= ` + y
			}
			rw, ra, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			q := `SELECT g.npi, r.name, r.specialty, r.city, g.state, g.companies, g.payments, ROUND(g.total,2) total, ` + y + ` program_year, ? prior_years_checked FROM (
				WITH a AS (` + general + `
				UNION ALL SELECT npi, company, program_year, amount, 1, state FROM payments_research p WHERE ` + rw + ` AND npi IS NOT NULL AND program_year <= ` + y + `)
				SELECT npi, MAX(state) state, SUM(n) payments, SUM(amount) total, GROUP_CONCAT(DISTINCT company) companies FROM a
				GROUP BY npi HAVING MIN(program_year) = ` + y + ` ORDER BY total DESC LIMIT ?) g
				LEFT JOIN recipients r ON r.recipient_key = g.npi ORDER BY g.total DESC`
			var priorArg any
			if len(years) > 0 {
				priorArg = strings.Join(years, ",")
			}
			args2 := append(append(append([]any{priorArg}, a...), ra...), f.limit)
			return runLocal(ctx, cmd, flags, db, q, args2...)
		},
	}
	addFilterFlags(cmd, &f, 50)
	return cmd
}
