package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/op"
	"openpayments-pp-cli/internal/store"
)

// opFilter holds the shared --state/--specialty/--company/--years filters
// every local domain command accepts.
type opFilter struct {
	states      string
	specialties string
	companies   string
	years       string
	minAmount   float64
	limit       int
}

func addFilterFlags(cmd *cobra.Command, f *opFilter, defaultLimit int) {
	cmd.Flags().StringVar(&f.states, "state", "", "Recipient state(s), e.g. PA or PA,NJ")
	cmd.Flags().StringVar(&f.specialties, "specialty", "", "Specialty substring(s), e.g. \"Pain Medicine\" or \"Orthopaedic Surgery,Neurological Surgery\"")
	cmd.Flags().StringVar(&f.companies, "company", "", "Paying company name substring(s), e.g. Stryker")
	cmd.Flags().StringVar(&f.years, "year", "", "Program year(s): 2024, 2019-2025 or 2023,2025")
	cmd.Flags().Float64Var(&f.minAmount, "min-amount", 0, "Only payments of at least this many USD")
	cmd.Flags().IntVar(&f.limit, "limit", defaultLimit, "Maximum rows to return")
}

// where builds a SQL predicate for a payment table alias. amountCol lets
// ownership use amount_invested; specCol lets general/research match all six
// specialty slots.
func (f opFilter) where(alias, specCol, amountCol string) (string, []any, error) {
	var parts []string
	var args []any
	col := func(c string) string {
		if alias == "" {
			return c
		}
		return alias + "." + c
	}
	if s := upperList(splitCSVFlag(f.states)); len(s) > 0 {
		parts = append(parts, col("state")+" IN ("+qmarks(len(s))+")")
		for _, x := range s {
			args = append(args, x)
		}
	}
	if s := splitCSVFlag(f.specialties); len(s) > 0 {
		var ors []string
		for _, x := range s {
			ors = append(ors, "LOWER("+col(specCol)+") LIKE ?")
			args = append(args, "%"+strings.ToLower(x)+"%")
		}
		parts = append(parts, "("+strings.Join(ors, " OR ")+")")
	}
	if s := splitCSVFlag(f.companies); len(s) > 0 {
		var ors []string
		for _, x := range s {
			ors = append(ors, "LOWER("+col("company")+") LIKE ?")
			args = append(args, "%"+strings.ToLower(x)+"%")
		}
		parts = append(parts, "("+strings.Join(ors, " OR ")+")")
	}
	if f.years != "" {
		ys, err := op.ParseYears(f.years)
		if err != nil {
			return "", nil, usageErr(err)
		}
		parts = append(parts, col("program_year")+" IN ("+qmarks(len(ys))+")")
		for _, y := range ys {
			args = append(args, y)
		}
	}
	if f.minAmount > 0 && amountCol != "" {
		parts = append(parts, col(amountCol)+" >= ?")
		args = append(args, f.minAmount)
	}
	if len(parts) == 0 {
		return "1=1", nil, nil
	}
	return strings.Join(parts, " AND "), args, nil
}

func qmarks(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

// localCtx opens the store for a local-only domain command, validates the
// data-source strategy, and emits sync hints to stderr.
func localCtx(cmd *cobra.Command, flags *rootFlags, resource string) (context.Context, context.CancelFunc, *sql.DB, error) {
	if err := validateDataSourceStrategy(flags, "local"); err != nil {
		return nil, nil, nil, usageErr(err)
	}
	ctx, cancel := boundCtx(cmd.Context(), flags)
	st, db, err := openOPStoreRead(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	emitOPHints(cmd, flags, st, resource)
	return ctx, cancel, db, nil
}

// emitOPHints prints stderr hints when the scope was never synced or is old.
// CMS republishes twice a year, so the default staleness window is 30 days
// unless --max-age is set explicitly.
func emitOPHints(cmd *cobra.Command, flags *rootFlags, st *store.Store, resource string) {
	maxAge := 30 * 24 * time.Hour
	if f := cmd.Flags().Lookup("max-age"); f != nil && f.Changed {
		maxAge = flags.maxAge
	}
	if !hintIfUnsynced(cmd, st, resource) {
		hintIfStale(cmd, st, resource, maxAge)
	}
}

// runLocal executes a query and prints rows through the standard output path.
func runLocal(ctx context.Context, cmd *cobra.Command, flags *rootFlags, db *sql.DB, q string, args ...any) error {
	rows, err := queryArgs(ctx, db, q, args...)
	if err != nil {
		return err
	}
	return printRows(cmd, flags, rows)
}

func printRows(cmd *cobra.Command, flags *rootFlags, rows []map[string]any) error {
	if len(rows) == 0 && !flags.asJSON && !flags.agent && isTerminal(cmd.OutOrStdout()) {
		fmt.Fprintln(cmd.ErrOrStderr(), "no matching rows in the local store (sync a wider scope or check filters)")
	}
	return flags.printJSON(cmd, rows)
}

// queryArgs runs a parameterized read and returns plain maps (drain-first).
func queryArgs(ctx context.Context, db *sql.DB, q string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				m[c] = string(v)
			case float64:
				m[c] = roundCents(v)
			default:
				m[c] = v
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func roundCents(v float64) float64 {
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return float64(int64(v*100-0.5)) / 100
}

// resolveRecipient turns an NPI, profile id, or name into recipient keys.
// Names use FTS5 over synced recipients; ambiguous names return candidates.
func resolveRecipient(ctx context.Context, db *sql.DB, arg string) ([]map[string]any, error) {
	arg = strings.TrimSpace(arg)
	if isDigits(arg) && len(arg) == 10 {
		rows, err := queryArgs(ctx, db, `SELECT recipient_key, npi, profile_id, name, specialty, city, state FROM recipients WHERE npi = ?`, arg)
		if err != nil || len(rows) > 0 {
			return rows, err
		}
		// The recipients table is rebuilt at the end of each sync; fall back
		// to the payment tables so data from an in-progress sync resolves.
		return queryArgs(ctx, db, `SELECT npi recipient_key, npi, MAX(profile_id) profile_id, MAX(name) name, MAX(specialty) specialty, MAX(city) city, MAX(state) state FROM (
			SELECT npi, profile_id, recipient_name name, specialty, city, state FROM payments_general WHERE npi = ?
			UNION ALL SELECT npi, profile_id, recipient_name, specialty, city, state FROM payments_research WHERE npi = ?
			UNION ALL SELECT npi, profile_id, recipient_name, specialty, city, state FROM payments_ownership WHERE npi = ?
			UNION ALL SELECT npi, profile_id, name, specialty, city, state FROM research_investigators WHERE npi = ?) GROUP BY npi`, arg, arg, arg, arg)
	}
	if isDigits(arg) {
		return queryArgs(ctx, db, `SELECT recipient_key, npi, profile_id, name, specialty, city, state FROM recipients WHERE profile_id = ?`, arg)
	}
	match := ftsQuery(arg)
	return queryArgs(ctx, db, `SELECT r.recipient_key, r.npi, r.profile_id, r.name, r.specialty, r.city, r.state
		FROM recipients_fts f JOIN recipients r ON r.recipient_key = f.recipient_key
		WHERE recipients_fts MATCH ? ORDER BY rank LIMIT 25`, match)
}

func ftsQuery(s string) string {
	var toks []string
	for _, t := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '.' }) {
		t = strings.Trim(t, `"'*()`)
		if t != "" {
			toks = append(toks, `"name" : "`+strings.ReplaceAll(t, `"`, ``)+`"*`)
		}
	}
	return strings.Join(toks, " AND ")
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// recipientPredicate selects a recipient's rows in a payment table.
func recipientPredicate(alias string, rec map[string]any) (string, []any) {
	if npi, ok := rec["npi"].(string); ok && npi != "" {
		return alias + ".npi = ?", []any{npi}
	}
	return alias + ".profile_id = ?", []any{rec["profile_id"]}
}

// pickRecipient resolves exactly one recipient or returns a usage error
// listing candidates.
func pickRecipient(ctx context.Context, db *sql.DB, arg string) (map[string]any, error) {
	cands, err := resolveRecipient(ctx, db, arg)
	if err != nil {
		return nil, err
	}
	switch len(cands) {
	case 0:
		return nil, notFoundErr(fmt.Errorf("no synced recipient matches %q; sync their scope (e.g. 'sync --years 2019-2025 --npi <NPI>')", arg))
	case 1:
		return cands[0], nil
	}
	var b strings.Builder
	for i, c := range cands {
		if i == 10 {
			break
		}
		fmt.Fprintf(&b, "\n  %v  %v  %v, %v  %v", c["npi"], c["name"], c["city"], c["state"], c["specialty"])
	}
	return nil, usageErr(fmt.Errorf("%q matches %d recipients; pass an NPI:%s", arg, len(cands), b.String()))
}
