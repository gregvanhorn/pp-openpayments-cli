// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelConcentrationCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var by string
	cmd := &cobra.Command{
		Use:   "concentration <company>",
		Short: "How concentrated a company's spend is: top-10 share, HHI and recipients to reach 50%/80%",
		Long: `Arithmetic over a company's synced general payments grouped by --by
(recipient, state, specialty or product): total, number of groups, top-10
share, Herfindahl-Hirschman index (sum of squared percentage shares, 0-10000)
and how many groups account for 50% and 80% of dollars.

Use this command to measure how concentrated one company's spend is. Do NOT
use it to list who a company pays; use 'company' instead.`,
		Example: `  openpayments-pp-cli concentration "Stryker Corporation" --year 2024 --agent
  openpayments-pp-cli concentration stryker --state PA --by specialty`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "concentration")
			}
			if len(args) == 0 {
				return usageErr(fmt.Errorf("a company name is required"))
			}
			group := map[string]string{"recipient": "COALESCE(p.npi, p.teaching_hospital_ccn)", "state": "p.state", "specialty": "p.specialty", "product": "pr.name"}[by]
			if group == "" {
				return usageErr(fmt.Errorf("--by must be recipient, state, specialty or product"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			f.companies = strings.Join(args, " ")
			w, a, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			from := "payments_general p"
			if by == "product" {
				from += " JOIN products pr ON pr.payment_type='general' AND pr.record_id=p.record_id AND pr.program_year=p.program_year"
			}
			label := group
			if by == "recipient" {
				label = "COALESCE(MAX(p.recipient_name), MAX(p.teaching_hospital_name))"
			}
			rows, err := queryArgs(ctx, db, `SELECT `+group+` k, `+label+` label, SUM(p.amount) total FROM `+from+` WHERE `+w+` GROUP BY k ORDER BY total DESC`, a...)
			if err != nil {
				return err
			}
			var total float64
			for _, r := range rows {
				total += toFloat(r["total"])
			}
			if total <= 0 {
				return notFoundErr(fmt.Errorf("no match for %q in synced payments (check the name or sync that company's scope)", f.companies))
			}
			var hhi, cum, top10 float64
			n50, n80 := 0, 0
			for i, r := range rows {
				share := toFloat(r["total"]) / total
				hhi += (share * 100) * (share * 100)
				cum += share
				if i < 10 {
					top10 += share
				}
				if n50 == 0 && cum >= 0.5 {
					n50 = i + 1
				}
				if n80 == 0 && cum >= 0.8 {
					n80 = i + 1
				}
				r["share_pct"] = roundCents(share * 100)
			}
			top := rows
			if len(top) > f.limit {
				top = top[:f.limit]
			}
			return flags.printJSON(cmd, map[string]any{
				"company": f.companies, "by": by, "total": roundCents(total), "groups": len(rows),
				"top10_share_pct": roundCents(top10 * 100), "hhi": roundCents(hhi),
				"groups_for_50pct": n50, "groups_for_80pct": n80, "top": top,
			})
		},
	}
	addFilterFlags(cmd, &f, 10)
	cmd.Flags().StringVar(&by, "by", "recipient", "Group by recipient, state, specialty or product")
	return cmd
}
