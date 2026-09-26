// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelRelationshipsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relationships <npi|name>",
		Short: "Every company that paid a clinician: first year, last year, total and trend",
		Long: `One row per paying company for a recipient: first and last program year,
years active, dollars per year, total, natures, and a trend label computed
from the last two synced years (new, growing, shrinking, steady, ended).

Use this command for how one recipient's company relationships evolved over
time. Do NOT use it for the full payment dossier with products and disputes;
use 'dossier' instead. Do NOT use it for comparing recipients; use 'compare'
instead.`,
		Example:     "  openpayments-pp-cli relationships 1234567890 --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "relationships")
			}
			if len(args) == 0 {
				return usageErr(fmt.Errorf("an NPI or name is required"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			rec, err := pickRecipient(ctx, db, strings.Join(args, " "))
			if err != nil {
				return err
			}
			pred, pargs := recipientPredicate("p", rec)
			rows, err := queryArgs(ctx, db, `SELECT company, program_year, SUM(amount) total, COUNT(*) payments, json_group_array(DISTINCT nature) natures FROM (
				SELECT company, program_year, amount, nature FROM payments_general p WHERE `+pred+`
				UNION ALL SELECT company, program_year, amount, 'Research' FROM payments_research p WHERE `+pred+`)
				GROUP BY company, program_year ORDER BY company, program_year`, append(append([]any{}, pargs...), pargs...)...)
			if err != nil {
				return err
			}
			var lastYear int
			_ = db.QueryRowContext(ctx, `SELECT COALESCE(MAX(program_year),0) FROM payments_general`).Scan(&lastYear)
			type rel struct {
				Company   string             `json:"company"`
				FirstYear int                `json:"first_year"`
				LastYear  int                `json:"last_year"`
				Years     int                `json:"years_active"`
				Total     float64            `json:"total"`
				Payments  int64              `json:"payments"`
				ByYear    map[string]float64 `json:"by_year"`
				Natures   []string           `json:"natures"`
				Trend     string             `json:"trend"`
			}
			byCo := map[string]*rel{}
			var order []string
			for _, r := range rows {
				co, _ := r["company"].(string)
				y := int(toInt(r["program_year"]))
				t := toFloat(r["total"])
				x := byCo[co]
				if x == nil {
					x = &rel{Company: co, FirstYear: y, ByYear: map[string]float64{}}
					byCo[co] = x
					order = append(order, co)
				}
				if y < x.FirstYear {
					x.FirstYear = y
				}
				if y > x.LastYear {
					x.LastYear = y
				}
				x.Years++
				x.Total = roundCents(x.Total + t)
				x.Payments += toInt(r["payments"])
				x.ByYear[itoa(y)] = t
				if n, ok := r["natures"].(string); ok {
					var ns []string
					_ = json.Unmarshal([]byte(n), &ns)
					for _, s := range ns {
						if !contains(x.Natures, s) {
							x.Natures = append(x.Natures, s)
						}
					}
				}
			}
			out := make([]*rel, 0, len(order))
			for _, co := range order {
				x := byCo[co]
				cur, prev := x.ByYear[itoa(lastYear)], x.ByYear[itoa(lastYear-1)]
				switch {
				case x.FirstYear == lastYear:
					x.Trend = "new"
				case cur == 0 && x.LastYear < lastYear:
					x.Trend = "ended"
				case prev > 0 && cur > prev*1.1:
					x.Trend = "growing"
				case prev > 0 && cur < prev*0.9:
					x.Trend = "shrinking"
				default:
					x.Trend = "steady"
				}
				out = append(out, x)
			}
			sort.SliceStable(out, func(i, j int) bool { return out[i].Total > out[j].Total })
			return flags.printJSON(cmd, map[string]any{"recipient": rec, "latest_synced_year": lastYear, "relationships": out})
		},
	}
	return cmd
}

func toInt(v any) int64 {
	switch t := v.(type) {
	case int64:
		return t
	case int:
		return int64(t)
	case float64:
		return int64(t)
	}
	return 0
}

func toFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	}
	return 0
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
