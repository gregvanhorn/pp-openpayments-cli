// Copyright 2026 Claude and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelRosterCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var npis, file string
	cmd := &cobra.Command{
		Use:   "roster",
		Short: "Summarize payments for a whole list of NPIs in one pass, including disputed counts",
		Long: `One row per NPI: synced general and research totals, first and last year,
top three companies, number of distinct natures and disputed-payment count.
NPIs with no synced payments are listed with found=false so gaps are visible;
sync them with 'sync --years 2019-2025 --npi <list>'.

Use this command for a list of many NPIs at once. Do NOT use it for one
recipient in depth; use 'dossier' instead. Do NOT use it for a small
side-by-side; use 'compare' instead.`,
		Example: `  openpayments-pp-cli roster --npi 1234567890,1987654321 --agent
  openpayments-pp-cli roster --file faculty-npis.txt --year 2024 --csv`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "roster")
			}
			list := splitCSVFlag(npis)
			list = append(list, args...)
			if file != "" {
				fh, err := os.Open(file)
				if err != nil {
					return usageErr(err)
				}
				sc := bufio.NewScanner(fh)
				for sc.Scan() {
					for _, t := range strings.FieldsFunc(sc.Text(), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
						list = append(list, t)
					}
				}
				fh.Close()
			}
			if len(list) == 0 {
				return usageErr(fmt.Errorf("pass NPIs with --npi, as arguments, or with --file"))
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
			var out []map[string]any
			for _, npi := range list {
				npi = strings.TrimSpace(npi)
				if npi == "" {
					continue
				}
				aa := append([]any{npi}, a...)
				rows, err := queryArgs(ctx, db, `SELECT MAX(recipient_name) name, MAX(specialty) specialty, MAX(state) state, COUNT(*) payments, ROUND(SUM(amount),2) general_total,
					MIN(program_year) first_year, MAX(program_year) last_year, COUNT(DISTINCT nature) natures, SUM(CASE WHEN dispute='Yes' THEN 1 ELSE 0 END) disputed,
					MAX(dataset_modified) dataset_modified FROM payments_general p WHERE p.npi = ? AND `+w, aa...)
				if err != nil {
					return err
				}
				row := map[string]any{"npi": npi, "found": false}
				if len(rows) > 0 && toInt(rows[0]["payments"]) > 0 {
					for k, v := range rows[0] {
						row[k] = v
					}
					row["found"] = true
				}
				var research float64
				_ = db.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount),0) FROM payments_research p WHERE (p.npi = ? OR EXISTS (SELECT 1 FROM research_investigators i WHERE i.record_id=p.record_id AND i.program_year=p.program_year AND i.npi = ?)) AND `+w, append([]any{npi, npi}, a...)...).Scan(&research)
				row["research_total"] = roundCents(research)
				if research > 0 {
					row["found"] = true
				}
				top, err := queryArgs(ctx, db, `SELECT company FROM payments_general p WHERE p.npi = ? AND `+w+` GROUP BY company ORDER BY SUM(amount) DESC LIMIT 3`, aa...)
				if err != nil {
					return err
				}
				var cos []string
				for _, t := range top {
					cos = append(cos, fmt.Sprint(t["company"]))
				}
				row["top_companies"] = strings.Join(cos, "; ")
				out = append(out, row)
			}
			return flags.printJSON(cmd, out)
		},
	}
	addFilterFlags(cmd, &f, 0)
	cmd.Flags().StringVar(&npis, "npi", "", "Comma-separated NPIs")
	cmd.Flags().StringVar(&file, "file", "", "File with NPIs (one per line or comma-separated)")
	return cmd
}
