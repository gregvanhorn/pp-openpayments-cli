package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/op"
)

// targetYear resolves --year to one program year, defaulting to the latest
// year present in payments_general.
func targetYear(ctx context.Context, db *sql.DB, f *opFilter) (int, error) {
	if f.years != "" {
		ys, err := op.ParseYears(f.years)
		if err != nil || len(ys) != 1 {
			return 0, usageErr(fmt.Errorf("--year takes one program year here (e.g. --year 2025)"))
		}
		f.years = ""
		return ys[0], nil
	}
	var y sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(program_year) FROM payments_general`).Scan(&y); err != nil {
		return 0, err
	}
	if !y.Valid {
		return 0, notFoundErr(fmt.Errorf("no general payments synced yet"))
	}
	return int(y.Int64), nil
}

// runYoY ranks recipient × company pairs by dollar change between year-1
// and year. direction +1 = rising, -1 = cooling.
func runYoY(cmd *cobra.Command, flags *rootFlags, f opFilter, direction int, includeNew bool, minDelta float64) error {
	ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
	if err != nil {
		return err
	}
	defer cancel()
	year, err := targetYear(ctx, db, &f)
	if err != nil {
		return err
	}
	w, a, err := f.where("p", "specialties", "amount")
	if err != nil {
		return err
	}
	// One pass over both years with conditional sums (ct = year, pt = year-1),
	// ranked and limited before the recipients join.
	cy, py := itoa(year), itoa(year-1)
	pairs := `SELECT npi, company, SUM(CASE WHEN program_year = ` + cy + ` THEN amount END) ct, SUM(CASE WHEN program_year = ` + py + ` THEN amount END) pt,
		COUNT(CASE WHEN program_year = ` + cy + ` THEN 1 END) cn, MAX(state) state
		FROM payments_general p WHERE ` + w + ` AND npi IS NOT NULL AND program_year IN (` + py + `, ` + cy + `) GROUP BY npi, company`
	var rolled int
	_ = db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM general_pairs)`).Scan(&rolled)
	if f.minAmount == 0 && rolled == 1 {
		// general_pairs is the same aggregation precomputed per state; only a
		// per-payment --min-amount needs the raw rows.
		pairs = `SELECT npi, company, SUM(CASE WHEN program_year = ` + cy + ` THEN total END) ct, SUM(CASE WHEN program_year = ` + py + ` THEN total END) pt,
		SUM(CASE WHEN program_year = ` + cy + ` THEN n END) cn, MAX(state) state
		FROM general_pairs p WHERE ` + w + ` AND program_year IN (` + py + `, ` + cy + `) GROUP BY npi, company`
	}
	var q string
	if direction > 0 {
		cond := "pt > 0"
		if includeNew {
			cond = "1=1"
		}
		q = `SELECT g.npi, r.name, r.specialty, r.city, g.state, g.company, ` + py + ` AS prior_year, ROUND(COALESCE(g.pt,0),2) prior_total, ` + cy + ` AS year, ROUND(g.ct,2) year_total,
			  ROUND(g.ct - COALESCE(g.pt,0),2) delta, CASE WHEN COALESCE(g.pt,0) > 0 THEN ROUND(100.0*(g.ct - g.pt)/g.pt,1) END pct_change, g.cn payments
			FROM (` + pairs + ` HAVING ct IS NOT NULL AND ` + cond + ` AND ct - COALESCE(pt,0) >= ? ORDER BY ct - COALESCE(pt,0) DESC LIMIT ?) g
			LEFT JOIN recipients r ON r.recipient_key = g.npi ORDER BY delta DESC`
	} else {
		q = `SELECT g.npi, r.name, r.specialty, r.city, g.state, g.company, ` + py + ` AS prior_year, ROUND(g.pt,2) prior_total, ` + cy + ` AS year, ROUND(COALESCE(g.ct,0),2) year_total,
			  ROUND(COALESCE(g.ct,0) - g.pt,2) delta, ROUND(100.0*(COALESCE(g.ct,0) - g.pt)/g.pt,1) pct_change, CASE WHEN g.ct IS NULL THEN 'stopped' ELSE 'reduced' END status
			FROM (` + pairs + ` HAVING pt IS NOT NULL AND pt - COALESCE(ct,0) >= ? ORDER BY COALESCE(ct,0) - pt ASC LIMIT ?) g
			LEFT JOIN recipients r ON r.recipient_key = g.npi ORDER BY delta ASC`
	}
	args := append(append([]any{}, a...), minDelta, f.limit)
	// Both years must have rows inside this scope, or every pair would read
	// as "stopped" / "new" purely because a year is not synced.
	for _, y := range []int{year - 1, year} {
		var n int
		cq := `SELECT COUNT(*) FROM (SELECT 1 FROM payments_general p WHERE ` + w + ` AND program_year = ? LIMIT 1)`
		if err := db.QueryRowContext(ctx, cq, append(append([]any{}, a...), y)...).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			fmt.Fprintf(cmd.ErrOrStderr(), "hint: no %d general payments synced for this scope; year-over-year needs %d and %d (e.g. sync --years %d-%d --states <ST>)\n", y, year-1, year, year-1, year)
			return printRows(cmd, flags, nil)
		}
	}
	return runLocal(ctx, cmd, flags, db, q, args...)
}

func itoa(i int) string { return strconv.Itoa(i) }
