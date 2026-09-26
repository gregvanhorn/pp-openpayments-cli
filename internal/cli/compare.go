// Copyright 2026 Claude and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNovelCompareCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	cmd := &cobra.Command{
		Use:   "compare <npi> <npi> [<npi>...]",
		Short: "Put two or more clinicians side by side by year, company and nature of payment",
		Long: `Side-by-side totals for 2+ recipients: general and research totals, dollars
per year (pivoted with one column per NPI), top companies and nature mix.

Use this command to put 2+ specific recipients side by side. Do NOT use it
for one recipient's full history; use 'dossier' instead. Do NOT use it for a
long NPI list; use 'roster' instead.`,
		Example:     "  openpayments-pp-cli compare 1234567890 1987654321 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "compare")
			}
			if len(args) < 2 {
				return usageErr(fmt.Errorf("compare needs at least two NPIs or names"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			fw, fa, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			var people []map[string]any
			pivot := map[string]map[string]any{}
			for _, arg := range args {
				rec, err := pickRecipient(ctx, db, arg)
				if err != nil {
					return err
				}
				pred, pa := recipientPredicate("p", rec)
				a := append(append([]any{}, pa...), fa...)
				w := pred + " AND " + fw
				summary := map[string]any{"recipient": rec}
				for _, s := range []struct{ k, q string }{
					{"general", `SELECT COUNT(*) payments, SUM(amount) total, COUNT(DISTINCT company) companies, MIN(program_year) first_year, MAX(program_year) last_year, SUM(CASE WHEN dispute='Yes' THEN 1 ELSE 0 END) disputed FROM payments_general p WHERE ` + w},
					{"research", `SELECT COUNT(*) payments, SUM(amount) total, COUNT(DISTINCT nct_id) studies FROM payments_research p WHERE ` + w},
					{"top_companies", `SELECT company, SUM(amount) total FROM payments_general p WHERE ` + w + ` GROUP BY company ORDER BY total DESC LIMIT 5`},
					{"by_nature", `SELECT nature, SUM(amount) total FROM payments_general p WHERE ` + w + ` GROUP BY nature ORDER BY total DESC`},
				} {
					rows, err := queryArgs(ctx, db, s.q, a...)
					if err != nil {
						return err
					}
					if s.k == "general" || s.k == "research" {
						if len(rows) > 0 {
							summary[s.k] = rows[0]
						}
					} else {
						summary[s.k] = rows
					}
				}
				years, err := queryArgs(ctx, db, `SELECT program_year, SUM(amount) total FROM payments_general p WHERE `+w+` GROUP BY program_year`, a...)
				if err != nil {
					return err
				}
				key := fmt.Sprint(rec["npi"])
				for _, y := range years {
					yk := fmt.Sprint(y["program_year"])
					if pivot[yk] == nil {
						pivot[yk] = map[string]any{"program_year": y["program_year"]}
					}
					pivot[yk][key] = y["total"]
				}
				people = append(people, summary)
			}
			var byYear []map[string]any
			for y := 2013; y <= 2035; y++ {
				if row, ok := pivot[itoa(y)]; ok {
					byYear = append(byYear, row)
				}
			}
			return flags.printJSON(cmd, map[string]any{"recipients": people, "by_year": byYear})
		},
	}
	addFilterFlags(cmd, &f, 5)
	return cmd
}
