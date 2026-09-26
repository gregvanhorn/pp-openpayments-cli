package op

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"openpayments-pp-cli/internal/cliutil"
)

// UserAgent is sent on every hand-built request. CMS's Akamai edge returns
// 403 for short or "cli"-containing agents; this form is accepted.
const UserAgent = "openpayments-pp/1.0 (+https://github.com/mvanhorn/printing-press-library)"

// BulkOptions tunes a bulk CSV load.
type BulkOptions struct {
	HTTP     *http.Client
	Progress io.Writer
	MaxRows  int // 0 = unlimited (dogfood/tests curtail)
}

// BulkSync streams a dataset's bulk CSV into the local store, keeping only
// rows inside scope. It never writes the (multi-GB) CSV to disk.
func BulkSync(ctx context.Context, db *sql.DB, ds Dataset, typ string, scope Scope, opts BulkOptions) (*SyncReport, error) {
	start := time.Now()
	if ds.DownloadURL == "" {
		return nil, fmt.Errorf("dataset %s has no bulk download URL", ds.Title)
	}
	hc := opts.HTTP
	if hc == nil {
		hc = &http.Client{Transport: http.DefaultTransport}
	}
	limiter := cliutil.NewAdaptiveLimiter(1)
	if err := limiter.Wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ds.DownloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		limiter.OnRateLimit()
		return nil, &cliutil.RateLimitError{URL: ds.DownloadURL, RetryAfter: cliutil.RetryAfter(resp)}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bulk download %s: HTTP %d", ds.DownloadURL, resp.StatusCode)
	}
	limiter.OnSuccess()
	r := csv.NewReader(resp.Body)
	r.ReuseRecord = true
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("reading CSV header: %w", err)
	}
	keys := make([]string, len(header))
	for i, h := range header {
		keys[i] = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
	}
	keep := scopePredicate(typ, scope)

	res, err := db.Exec(`INSERT INTO sync_runs (started_at, args, status) VALUES (?, ?, 'running')`, time.Now().UTC().Format(time.RFC3339), "bulk "+ds.Title+" "+scopeArgs(scope))
	if err != nil {
		return nil, err
	}
	run, _ := res.LastInsertId()
	// One bulk pass covers every filter in the scope (e.g. PA and NJ); each
	// is recorded and reconciled as its own sync scope afterwards.
	var units []*scopeUnit
	for _, f := range filtersFor(typ, scope) {
		if f.piPass {
			continue
		}
		units = append(units, &scopeUnit{key: fmt.Sprintf("%s:%d:%s", typ, ds.Year, f.desc), typ: typ, year: ds.Year, ds: ds, filter: f})
	}
	if scope.Empty() {
		units = []*scopeUnit{{key: fmt.Sprintf("%s:%d:all", typ, ds.Year), typ: typ, year: ds.Year, ds: ds, filter: filterSpec{desc: "all"}}}
	}
	allPrev := true
	for _, x := range units {
		var prevComplete int
		_ = db.QueryRow(`SELECT complete FROM sync_scopes WHERE scope=?`, x.key).Scan(&prevComplete)
		x.prevDone = prevComplete == 1
		allPrev = allPrev && x.prevDone
	}
	// Per-row change logging treats the load as a re-sync only when every
	// covered scope was complete before.
	u := &scopeUnit{key: units[0].key, typ: typ, year: ds.Year, ds: ds, prevDone: allPrev}
	part := &partition{scope: u, label: u.key}
	w := newWriter(db, run)
	batch := make([]map[string]any, 0, 2000)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := w.writePage(pageResult{part: part, rows: batch})
		batch = batch[:0]
		return err
	}
	read := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading CSV row %d: %w", read+1, err)
		}
		read++
		if opts.Progress != nil && read%500000 == 0 {
			fmt.Fprintf(opts.Progress, "bulk: %s read %d rows, kept %d\n", ds.Title, read, part.rows)
		}
		row := make(map[string]any, len(keys))
		for i, k := range keys {
			if i < len(rec) {
				row[k] = rec[i]
			}
		}
		if !keep(Row(row)) {
			continue
		}
		batch = append(batch, row)
		part.rows++
		if len(batch) == cap(batch) {
			if err := flush(); err != nil {
				return nil, err
			}
		}
		if opts.MaxRows > 0 && part.rows >= opts.MaxRows {
			break
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	complete := 1
	if opts.MaxRows > 0 && part.rows >= opts.MaxRows {
		complete = 0
	}
	deleted := 0
	var reports []ScopeReport
	for _, x := range units {
		if complete == 1 && x.filter.where != "" {
			n, err := w.reconcileDeletes(x)
			if err != nil {
				return nil, err
			}
			deleted += n
		}
		rows := part.rows
		if x.filter.where != "" {
			_ = db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE program_year = ? AND sync_run = ? AND %s`, TableFor(typ), x.filter.where), append([]any{ds.Year, run}, x.filter.args...)...).Scan(&rows)
		}
		if _, err := db.Exec(`INSERT INTO sync_scopes (scope, payment_type, program_year, dataset_id, dataset_modified, filter, rows, sync_run, last_run, complete)
			VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(scope) DO UPDATE SET dataset_id=excluded.dataset_id, dataset_modified=excluded.dataset_modified,
			rows=excluded.rows, sync_run=excluded.sync_run, last_run=excluded.last_run, complete=excluded.complete`,
			x.key, typ, ds.Year, ds.DatasetID, ds.Modified, x.filter.desc+";bulk", rows, run, time.Now().UTC().Format(time.RFC3339), complete); err != nil {
			return nil, err
		}
		reports = append(reports, ScopeReport{Scope: x.key, Type: typ, Year: ds.Year, Filter: x.filter.desc + " (bulk csv)", Modified: ds.Modified, Rows: rows, Status: "synced"})
	}
	if err := RebuildDerived(db); err != nil {
		return nil, err
	}
	_, _ = db.Exec(`UPDATE sync_runs SET finished_at=?, rows_seen=?, added=?, amended=?, deleted=?, status='ok' WHERE sync_run=?`,
		time.Now().UTC().Format(time.RFC3339), w.seen, w.added, w.amended, deleted, run)
	return &SyncReport{
		SyncRun: run, RowsSeen: w.seen, Added: w.added, Amended: w.amended, Deleted: deleted, Partitions: 1,
		Scopes:  reports,
		Elapsed: time.Since(start).Round(time.Millisecond).String(),
	}, nil
}

// scopePredicate mirrors filtersFor on raw rows for the bulk path.
func scopePredicate(typ string, s Scope) func(Row) bool {
	set := func(xs []string, upper bool) map[string]bool {
		m := map[string]bool{}
		for _, x := range xs {
			if upper {
				x = strings.ToUpper(x)
			}
			m[x] = true
		}
		return m
	}
	states, npis := set(s.States, true), set(s.NPIs, false)
	companies := map[string]bool{}
	for _, c := range s.Companies {
		companies[strings.ToLower(c)] = true
	}
	npiKey, specKey := recipientField(typ, "npi"), recipientField(typ, "specialty")
	return func(r Row) bool {
		if len(states) > 0 && !states[strings.ToUpper(r.S("recipient_state"))] {
			return false
		}
		if len(companies) > 0 && !companies[strings.ToLower(r.S(companyField))] {
			return false
		}
		if len(npis) > 0 && !npis[r.S(npiKey)] {
			pi := false
			if typ == TypeResearch {
				for i := 1; i <= 5; i++ {
					if npis[r.S(fmt.Sprintf("principal_investigator_%d_npi", i))] {
						pi = true
					}
				}
			}
			if !pi {
				return false
			}
		}
		if len(s.Specialties) > 0 {
			spec := strings.ToLower(r.S(specKey))
			ok := false
			for _, sp := range s.Specialties {
				if strings.Contains(spec, strings.ToLower(sp)) {
					ok = true
				}
			}
			if !ok {
				return false
			}
		}
		return true
	}
}
