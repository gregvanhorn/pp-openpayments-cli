package op

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// CompanyProfileTitle is the metastore title of the reporting-entity table.
const CompanyProfileTitle = "Reporting entity profile information"

// EnsureCompanyProfiles caches the ~3k reporting-entity profiles locally so
// loose names like "stryker" resolve to CMS's exact spelling without a slow
// wildcard query against 15M-row payment tables.
func EnsureCompanyProfiles(ctx context.Context, g Getter, db *sql.DB, reg []Dataset, refresh bool) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS company_profiles (company_id TEXT PRIMARY KEY, name TEXT, alt_names TEXT, state TEXT, country TEXT, fetched_at TEXT)`); err != nil {
		return err
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM company_profiles`).Scan(&n)
	if n > 0 && !refresh {
		return nil
	}
	var ds Dataset
	for _, d := range reg {
		if d.Title == CompanyProfileTitle {
			ds = d
		}
	}
	if ds.DatasetID == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var all []Row
	for offset := 0; ; offset += MaxPageSize {
		resp, err := RunQuery(ctx, g, ds.DatasetID, Query{Limit: MaxPageSize, Offset: offset})
		if err != nil {
			return err
		}
		for _, r := range resp.Results {
			all = append(all, Row(r))
		}
		if len(resp.Results) < MaxPageSize {
			break
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range all {
		var alts []string
		for i := 1; i <= 5; i++ {
			if a := r.S("amgpo_making_payment_alternate_name" + string(rune('0'+i))); a != "" {
				alts = append(alts, a)
			}
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO company_profiles VALUES (?,?,?,?,?,?)`,
			r.S("amgpo_making_payment_id"), r.S("amgpo_making_payment_name"), strings.Join(alts, "; "),
			r.S("amgpo_making_payment_state"), r.S("amgpo_making_payment_country"), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResolveCompanies expands loose company names to CMS's exact names.
// An exact (case-insensitive) match wins; otherwise every profile whose name
// or alternate name contains the term is returned. Unmatched terms pass
// through unchanged so an exact name that is missing from profiles still works.
func ResolveCompanies(db *sql.DB, terms []string) ([]string, map[string][]string, error) {
	seen := map[string]bool{}
	var out []string
	matches := map[string][]string{}
	for _, t := range terms {
		var names []string
		rows, err := db.Query(`SELECT name FROM company_profiles WHERE lower(name) = lower(?)`, t)
		if err != nil {
			return nil, nil, err
		}
		names = scanStrings(rows)
		if len(names) == 0 {
			rows, err = db.Query(`SELECT name FROM company_profiles WHERE lower(name) LIKE lower(?) OR lower(alt_names) LIKE lower(?) ORDER BY name`, "%"+t+"%", "%"+t+"%")
			if err != nil {
				return nil, nil, err
			}
			names = scanStrings(rows)
		}
		if len(names) == 0 {
			names = []string{t}
		}
		matches[t] = names
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out, matches, nil
}

func scanStrings(rows *sql.Rows) []string {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s sql.NullString
		if rows.Scan(&s) == nil && s.Valid {
			out = append(out, s.String)
		}
	}
	return out
}
