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
	pairs := `SELECT npi, company, SUM(amount) t, COUNT(*) n, MAX(recipient_name) name, MAX(specialty) specialty, MAX(city) city, MAX(state) state FROM payments_general p WHERE ` + w + ` AND npi IS NOT NULL AND program_year = ? GROUP BY npi, company`
	var q string
	var args []any
	if direction > 0 {
		cond := "pv.t > 0"
		if includeNew {
			cond = "1=1"
		}
		q = `WITH cur AS (` + pairs + `), prev AS (` + pairs + `)
			SELECT c.npi, c.name, c.specialty, c.city, c.state, c.company, ? AS prior_year, ROUND(COALESCE(pv.t,0),2) prior_total, ? AS year, ROUND(c.t,2) year_total,
			  ROUND(c.t - COALESCE(pv.t,0),2) delta, CASE WHEN COALESCE(pv.t,0) > 0 THEN ROUND(100.0*(c.t - pv.t)/pv.t,1) END pct_change, c.n payments
			FROM cur c LEFT JOIN prev pv ON pv.npi = c.npi AND pv.company = c.company
			WHERE ` + cond + ` AND c.t - COALESCE(pv.t,0) >= ? ORDER BY delta DESC LIMIT ?`
	} else {
		q = `WITH cur AS (` + pairs + `), prev AS (` + pairs + `)
			SELECT pv.npi, pv.name, pv.specialty, pv.city, pv.state, pv.company, ? AS prior_year, ROUND(pv.t,2) prior_total, ? AS year, ROUND(COALESCE(c.t,0),2) year_total,
			  ROUND(COALESCE(c.t,0) - pv.t,2) delta, ROUND(100.0*(COALESCE(c.t,0) - pv.t)/pv.t,1) pct_change, CASE WHEN c.t IS NULL THEN 'stopped' ELSE 'reduced' END status
			FROM prev pv LEFT JOIN cur c ON c.npi = pv.npi AND c.company = pv.company
			WHERE pv.t - COALESCE(c.t,0) >= ? ORDER BY delta ASC LIMIT ?`
	}
	args = append(args, a...)
	args = append(args, year)
	args = append(args, a...)
	args = append(args, year-1, year-1, year, minDelta, f.limit)
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
