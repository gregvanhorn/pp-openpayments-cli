// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/op"
)

// liveConcentrationGroups caps server-side groups fetched for live mode.
const liveConcentrationGroups = op.MaxPageSize

func newNovelConcentrationCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var by string
	cmd := &cobra.Command{
		Use:   "concentration <company>",
		Short: "Measure how concentrated a company's spend is: top-10 share, HHI and recipients to reach 50%/80%",
		Long: `Arithmetic over a company's general payments grouped by --by (recipient,
state, specialty or product; a payment linked to several products is split
evenly across them): total, number of groups, top-10 share,
Herfindahl-Hirschman index (sum of squared percentage shares, 0-10000) and how
many groups account for 50% and 80% of dollars.

Reads the local store. With --data-source auto (default) and no synced
payments for the company, it computes the answer live from CMS server-side
summary for one program year (--year, default the latest published): exact
company total and recipient count, HHI over the top 500 recipients, --by
recipient only (coverage_note says so).

Use this command to measure how concentrated one company's spend is. Do NOT
use it to list who a company pays; use 'company' instead.`,
		Example: `  openpayments-pp-cli concentration "Stryker Corporation" --year 2024 --agent
  openpayments-pp-cli concentration stryker --state PA --by specialty`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:happy-args": "<company>=Stryker Corporation;--year=2024"},
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
			if by != "recipient" && by != "state" && by != "specialty" && by != "product" {
				return usageErr(fmt.Errorf("--by must be recipient, state, specialty or product"))
			}
			company := strings.Join(args, " ")
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			st, db, err := openOPStore(ctx)
			if err != nil {
				return err
			}
			f.companies = company
			w, a, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			var localRows int
			if flags.dataSource != "live" {
				_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT 1 FROM payments_general p WHERE `+w+` LIMIT 1)`, a...).Scan(&localRows)
			}
			var rows []map[string]any
			var liveTotal float64
			liveGroups := 0
			source, note := "local", ""
			if localRows > 0 {
				emitOPHints(cmd, flags, st, "payments_general")
				rows, err = localConcentration(ctx, db, by, w, a)
			} else {
				if flags.dataSource == "local" {
					return notFoundErr(fmt.Errorf("no match for %q in synced payments (check the name or sync that company's scope)", company))
				}
				if by == "product" {
					return usageErr(fmt.Errorf("--by product needs synced data for %q (sync --years <Y> --company %q)", company, company))
				}
				source = "live"
				rows, liveTotal, liveGroups, note, err = liveConcentration(ctx, flags, db, company, by, f)
			}
			if err != nil {
				return err
			}
			var total float64
			for _, r := range rows {
				total += toFloat(r["total"])
			}
			groupCount := len(rows)
			if source == "live" {
				total, groupCount = liveTotal, liveGroups
			}
			if total <= 0 {
				return notFoundErr(fmt.Errorf("no match for %q in %s payments (check the name or sync that company's scope)", company, source))
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
				r["total"] = roundCents(toFloat(r["total"]))
				r["share_pct"] = roundCents(share * 100)
			}
			top := rows
			if len(top) > f.limit {
				top = top[:f.limit]
			}
			out := map[string]any{
				"company": company, "by": by, "source": source, "total": roundCents(total), "groups": groupCount,
				"top10_share_pct": roundCents(top10 * 100), "hhi": roundCents(hhi),
				"groups_for_50pct": n50, "groups_for_80pct": n80, "top": top,
			}
			if note != "" {
				out["coverage_note"] = note
			}
			if source == "live" {
				cmd.Annotations["pp:data-source"] = "live"
			}
			return flags.printJSON(cmd, out)
		},
	}
	addFilterFlags(cmd, &f, 10)
	cmd.Flags().StringVar(&by, "by", "recipient", "Group by recipient, state, specialty or product")
	return cmd
}

func localConcentration(ctx context.Context, db *sql.DB, by, w string, a []any) ([]map[string]any, error) {
	group := map[string]string{"recipient": "COALESCE(p.npi, p.teaching_hospital_ccn)", "state": "p.state", "specialty": "p.specialty", "product": "pr.name"}[by]
	from := "payments_general p"
	amount := "p.amount"
	if by == "product" {
		from += " CROSS JOIN products pr ON pr.payment_type='general' AND pr.record_id=p.record_id AND pr.program_year=p.program_year"
		// Split each payment evenly across its product slots so shares sum to 100%.
		amount = "p.amount / (SELECT COUNT(*) FROM products x WHERE x.payment_type='general' AND x.record_id=p.record_id AND x.program_year=p.program_year)"
	}
	label := group
	if by == "recipient" {
		label = "COALESCE(MAX(p.recipient_name), MAX(p.teaching_hospital_name))"
	}
	return queryArgs(ctx, db, `SELECT `+group+` k, `+label+` label, SUM(`+amount+`) total FROM `+from+` WHERE `+w+` GROUP BY k ORDER BY total DESC`, a...)
}

// liveConcentration answers from CMS's pre-grouped "payments grouped by
// covered recipient and reporting entities" table for one program year. It is
// indexed on the company id, so exact totals come back in about a second
// (the 15M-row detail table needs ~30 s per aggregate).
func liveConcentration(ctx context.Context, flags *rootFlags, db *sql.DB, company, by string, f opFilter) ([]map[string]any, float64, int, string, error) {
	if by != "recipient" || f.states != "" || f.specialties != "" || f.minAmount > 0 {
		return nil, 0, 0, "", notFoundErr(fmt.Errorf("no synced payments for %q; live mode supports --by recipient without state/specialty/amount filters — sync the scope first (sync --years <Y> --company %q)", company, company))
	}
	reg, err := resolveRegistry(ctx, flags, db, false)
	if err != nil {
		return nil, 0, 0, "", err
	}
	const slug = "payments-grouped-by-covered-recipient-and-reporting-entities"
	year := 0
	if f.years != "" {
		ys, err := op.ParseYears(f.years)
		if err != nil || len(ys) != 1 {
			return nil, 0, 0, "", usageErr(fmt.Errorf("live concentration takes one --year"))
		}
		year = ys[0]
	} else {
		for _, d := range reg {
			if d.Type == slug && d.Year > year {
				year = d.Year
			}
		}
	}
	ds, ok := op.Find(reg, year, slug)
	if !ok || year == 0 {
		return nil, 0, 0, "", notFoundErr(fmt.Errorf("no grouped summary dataset for program year %d", year))
	}
	c, err := flags.newClient()
	if err != nil {
		return nil, 0, 0, "", err
	}
	if err := op.EnsureCompanyProfiles(ctx, c, db, reg, false); err != nil {
		return nil, 0, 0, "", apiErr(err)
	}
	ids, err := queryArgs(ctx, db, `SELECT company_id, name FROM company_profiles WHERE lower(name) = lower(?) UNION SELECT company_id, name FROM company_profiles WHERE lower(name) LIKE lower(?) OR lower(alt_names) LIKE lower(?) LIMIT 10`, company, "%"+company+"%", "%"+company+"%")
	if err != nil {
		return nil, 0, 0, "", err
	}
	if len(ids) == 0 {
		return nil, 0, 0, "", notFoundErr(fmt.Errorf("no match for %q among CMS reporting entities", company))
	}
	var idList, names []string
	for _, r := range ids {
		idList = append(idList, fmt.Sprint(r["company_id"]))
		names = append(names, fmt.Sprint(r["name"]))
	}
	base := []op.Condition{{Property: "amgpo_id", Operator: "in", Value: idList}, {Property: "payment_type", Operator: "=", Value: "General"}}
	sum := op.Aggregate{Operator: "sum", Property: "total_amount", Alias: "total"}
	totResp, err := op.RunQuery(ctx, c, ds.DatasetID, op.Query{Conditions: base, Properties: []string{"payment_type"}, Groupings: []string{"payment_type"},
		Aggregates: []op.Aggregate{sum, {Operator: "count", Property: "recipient_id", Alias: "n"}}, Limit: 5})
	if err != nil {
		return nil, 0, 0, "", apiErr(err)
	}
	var total float64
	groups := 0
	for _, r := range totResp.Results {
		row := op.Row(r)
		t, _ := row.F("total").(float64)
		total += t
		if n, ok := row.I("n").(int); ok {
			groups += n
		}
	}
	cols := []string{"recipient_id", "covered_recipient_npi", "covered_recipient_profile_last_name", "covered_recipient_profile_first_name", "teaching_hospital_name"}
	resp, err := op.RunQuery(ctx, c, ds.DatasetID, op.Query{Conditions: base, Properties: cols, Groupings: cols,
		Aggregates: []op.Aggregate{sum}, Sorts: []op.Sort{{Property: "total", Desc: true}}, Limit: liveConcentrationGroups})
	if err != nil {
		return nil, 0, 0, "", apiErr(err)
	}
	rows := make([]map[string]any, 0, len(resp.Results))
	for _, r := range resp.Results {
		row := op.Row(r)
		label := op.PersonName(row.S("covered_recipient_profile_first_name"), "", row.S("covered_recipient_profile_last_name"), "")
		if label == "" {
			label = row.S("teaching_hospital_name")
		}
		key := row.S("covered_recipient_npi")
		if key == "" {
			key = row.S("recipient_id")
		}
		t, _ := row.F("total").(float64)
		rows = append(rows, map[string]any{"k": key, "label": label, "total": t, "program_year": year, "dataset_modified": ds.Modified})
	}
	note := fmt.Sprintf("live: CMS %d grouped summary (dataset modified %s) for %v; total and group count are exact, HHI covers the top %d recipients", year, ds.Modified, names, len(rows))
	return rows, total, groups, note, nil
}
