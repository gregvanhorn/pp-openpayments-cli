// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelOverlapCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	cmd := &cobra.Command{
		Use:   "overlap <company> <company>",
		Short: "Recipients two companies both pay, and who only one of them pays",
		Long: `Set comparison of the recipients (by NPI) two companies paid in the synced
scope, with each company's dollars per shared recipient. Company names are
case-insensitive substrings.

Use this command to compare two companies' recipient sets. Do NOT use it for
one company's footprint; use 'company' instead. Do NOT use it to compare
doctors; use 'compare' instead.`,
		Example:     `  openpayments-pp-cli overlap "Stryker Corporation" "Zimmer Biomet" --state PA --agent`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "overlap")
			}
			if len(args) != 2 {
				return usageErr(fmt.Errorf("overlap takes exactly two company names"))
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
			sets := `SELECT npi, MAX(recipient_name) name, MAX(specialty) specialty, MAX(city) city, MAX(state) state, SUM(CASE WHEN LOWER(company) LIKE ? THEN amount ELSE 0 END) a_total, SUM(CASE WHEN LOWER(company) LIKE ? THEN amount ELSE 0 END) b_total
				FROM payments_general p WHERE ` + w + ` AND npi IS NOT NULL AND (LOWER(company) LIKE ? OR LOWER(company) LIKE ?) GROUP BY npi`
			ta, tb := "%"+strings.ToLower(args[0])+"%", "%"+strings.ToLower(args[1])+"%"
			sa := append(append([]any{ta, tb}, a...), ta, tb)
			counts, err := queryArgs(ctx, db, `WITH s AS (`+sets+`) SELECT COALESCE(SUM(a_total>0 AND b_total>0),0) both_count, COALESCE(SUM(a_total>0 AND b_total=0),0) only_a, COALESCE(SUM(a_total=0 AND b_total>0),0) only_b FROM s`, sa...)
			if err != nil {
				return err
			}
			shared, err := queryArgs(ctx, db, `WITH s AS (`+sets+`) SELECT s.npi, s.name, s.specialty, s.city, s.state, ROUND(a_total,2) a_total, ROUND(b_total,2) b_total
				FROM s WHERE a_total > 0 AND b_total > 0 ORDER BY a_total + b_total DESC LIMIT ?`, append(sa, f.limit)...)
			if err != nil {
				return err
			}
			onlyA, err := queryArgs(ctx, db, `WITH s AS (`+sets+`) SELECT s.npi, s.name, s.specialty, s.state, ROUND(a_total,2) a_total FROM s WHERE a_total > 0 AND b_total = 0 ORDER BY a_total DESC LIMIT ?`, append(sa, f.limit)...)
			if err != nil {
				return err
			}
			onlyB, err := queryArgs(ctx, db, `WITH s AS (`+sets+`) SELECT s.npi, s.name, s.specialty, s.state, ROUND(b_total,2) b_total FROM s WHERE b_total > 0 AND a_total = 0 ORDER BY b_total DESC LIMIT ?`, append(sa, f.limit)...)
			if err != nil {
				return err
			}
			out := map[string]any{"company_a": args[0], "company_b": args[1], "shared": shared, "only_a": onlyA, "only_b": onlyB}
			if len(counts) > 0 {
				out["counts"] = counts[0]
			}
			return flags.printJSON(cmd, out)
		},
	}
	addFilterFlags(cmd, &f, 25)
	return cmd
}
