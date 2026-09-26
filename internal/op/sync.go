package op

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// Scope selects which slice of Open Payments to mirror locally. National
// years run to 15M+ rows, so every sync is scoped unless the bulk CSV path
// is used.
type Scope struct {
	Years       []int
	Types       []string
	States      []string
	Specialties []string
	NPIs        []string
	Companies   []string
}

// Empty reports whether no row filter was given.
func (s Scope) Empty() bool {
	return len(s.States) == 0 && len(s.Specialties) == 0 && len(s.NPIs) == 0 && len(s.Companies) == 0
}

// SyncOptions tunes a sync run.
type SyncOptions struct {
	Concurrency int
	Force       bool      // re-sync even when dataset modified date is unchanged
	MaxPages    int       // per partition; 0 = unlimited
	Progress    io.Writer // human progress lines (stderr)
	DryRun      bool
}

// SyncReport summarizes a run for JSON output.
type SyncReport struct {
	SyncRun    int64         `json:"sync_run"`
	Scopes     []ScopeReport `json:"scopes"`
	RowsSeen   int           `json:"rows_seen"`
	Added      int           `json:"added"`
	Amended    int           `json:"amended"`
	Deleted    int           `json:"deleted"`
	Skipped    int           `json:"scopes_skipped_unchanged"`
	Failed     int           `json:"scopes_failed"`
	Elapsed    string        `json:"elapsed"`
	Partitions int           `json:"partitions"`
}

// ScopeReport is one type × year × filter unit.
type ScopeReport struct {
	Scope    string `json:"scope"`
	Type     string `json:"type"`
	Year     int    `json:"program_year"`
	Filter   string `json:"filter"`
	Modified string `json:"dataset_modified"`
	Rows     int    `json:"rows"`
	Status   string `json:"status"` // synced, unchanged, failed, planned
	Error    string `json:"error,omitempty"`
}

type filterSpec struct {
	desc  string      // canonical "state=PA"
	conds []Condition // DKAN side
	where string      // local SQL predicate for deletion reconciliation
	args  []any
	// piNPIs marks research passes that match principal investigator slots
	// instead of the covered recipient; no deletion reconciliation.
	piPass bool
}

type partition struct {
	scope   *scopeUnit
	month   int // 0 = whole year
	extra   []Condition
	label   string
	offset  int
	pages   int
	rows    int
	err     error
	percent int
}

type scopeUnit struct {
	props    []string
	key      string
	typ      string
	year     int
	ds       Dataset
	filter   filterSpec
	prevDone bool
	parts    []*partition
	report   *ScopeReport
}

func recipientField(typ, field string) string {
	if typ == TypeOwnership {
		switch field {
		case "npi":
			return "physician_npi"
		case "specialty":
			return "physician_specialty"
		}
	}
	switch field {
	case "npi":
		return "covered_recipient_npi"
	case "specialty":
		return "covered_recipient_specialty_1"
	}
	return field
}

const companyField = "applicable_manufacturer_or_applicable_gpo_making_payment_name"

func filtersFor(typ string, s Scope) []filterSpec {
	var out []filterSpec
	var base []Condition
	var baseWhere []string
	var baseArgs []any
	var baseDesc []string
	if len(s.Companies) > 0 {
		base = append(base, Condition{Property: companyField, Operator: "in", Value: s.Companies})
		baseWhere = append(baseWhere, "LOWER(company) IN ("+placeholders(len(s.Companies))+")")
		for _, c := range s.Companies {
			baseArgs = append(baseArgs, strings.ToLower(c))
		}
		baseDesc = append(baseDesc, "company="+strings.Join(s.Companies, "|"))
	}
	if len(s.NPIs) > 0 {
		base = append(base, Condition{Property: recipientField(typ, "npi"), Operator: "in", Value: s.NPIs})
		baseWhere = append(baseWhere, "npi IN ("+placeholders(len(s.NPIs))+")")
		for _, n := range s.NPIs {
			baseArgs = append(baseArgs, n)
		}
		baseDesc = append(baseDesc, "npi="+strings.Join(s.NPIs, "|"))
	}
	states := s.States
	if len(states) == 0 {
		states = []string{""}
	}
	specs := s.Specialties
	if len(specs) == 0 {
		specs = []string{""}
	}
	for _, st := range states {
		for _, sp := range specs {
			f := filterSpec{conds: append([]Condition{}, base...), where: strings.Join(baseWhere, " AND "), args: append([]any{}, baseArgs...)}
			desc := append([]string{}, baseDesc...)
			if st != "" {
				f.conds = append(f.conds, Condition{Property: "recipient_state", Operator: "=", Value: st})
				f.where = andWhere(f.where, "state = ?")
				f.args = append(f.args, st)
				desc = append(desc, "state="+st)
			}
			if sp != "" {
				f.conds = append(f.conds, Condition{Property: recipientField(typ, "specialty"), Operator: "like", Value: "%" + sp + "%"})
				f.where = andWhere(f.where, "specialty LIKE ?")
				f.args = append(f.args, "%"+sp+"%")
				desc = append(desc, "specialty="+sp)
			}
			sort.Strings(desc)
			f.desc = strings.Join(desc, ";")
			if f.desc == "" {
				f.desc = "all"
			}
			out = append(out, f)
		}
	}
	// A doctor paid as principal investigator often is not the covered
	// recipient (the hospital is), so NPI-scoped research syncs also match
	// the five PI slots.
	if typ == TypeResearch && len(s.NPIs) > 0 {
		for i := 1; i <= 5; i++ {
			conds := []Condition{{Property: fmt.Sprintf("principal_investigator_%d_npi", i), Operator: "in", Value: s.NPIs}}
			if len(s.Companies) > 0 {
				conds = append(conds, Condition{Property: companyField, Operator: "in", Value: s.Companies})
			}
			out = append(out, filterSpec{desc: fmt.Sprintf("pi%d_npi=%s", i, strings.Join(s.NPIs, "|")), conds: conds, piPass: true})
		}
	}
	return out
}

func andWhere(a, b string) string {
	if a == "" {
		return b
	}
	return a + " AND " + b
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// needsMonthSplit keeps DKAN offsets shallow on big partitions (offset
// 650k costs ~7 s per page vs ~2 s at offset 0).
func needsMonthSplit(typ string, f filterSpec, s Scope) bool {
	if typ == TypeOwnership {
		return false
	}
	if f.piPass || len(s.NPIs) > 0 {
		return false
	}
	return true
}

// Sync mirrors the requested scope into the local store.
func Sync(ctx context.Context, g Getter, db *sql.DB, reg []Dataset, scope Scope, opts SyncOptions) (*SyncReport, error) {
	start := time.Now()
	if opts.Concurrency <= 0 {
		opts.Concurrency = 4
	}
	progress := opts.Progress
	if progress == nil {
		progress = io.Discard
	}
	types := scope.Types
	if len(types) == 0 {
		types = PaymentTypes
	}
	report := &SyncReport{}
	var units []*scopeUnit
	for _, typ := range types {
		for _, year := range scope.Years {
			ds, ok := Find(reg, year, typ)
			if !ok {
				report.Scopes = append(report.Scopes, ScopeReport{Type: typ, Year: year, Status: "failed", Error: fmt.Sprintf("no %s dataset published for program year %d", typ, year)})
				report.Failed++
				continue
			}
			for _, f := range filtersFor(typ, scope) {
				u := &scopeUnit{key: fmt.Sprintf("%s:%d:%s", typ, year, f.desc), typ: typ, year: year, ds: ds, filter: f}
				u.report = &ScopeReport{Scope: u.key, Type: typ, Year: year, Filter: f.desc, Modified: ds.Modified}
				var mod string
				var complete int
				err := db.QueryRow(`SELECT dataset_modified, complete FROM sync_scopes WHERE scope = ?`, u.key).Scan(&mod, &complete)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return nil, err
				}
				u.prevDone = complete == 1
				if u.prevDone && mod == ds.Modified && !opts.Force {
					u.report.Status = "unchanged"
					report.Skipped++
					report.Scopes = append(report.Scopes, *u.report)
					continue
				}
				if needsMonthSplit(typ, f, scope) {
					for m := 1; m <= 12; m++ {
						lo := fmt.Sprintf("%02d/00/%d", m, year)
						hi := fmt.Sprintf("%02d/99/%d", m, year)
						u.parts = append(u.parts, &partition{scope: u, month: m, extra: []Condition{{Property: "date_of_payment", Operator: "between", Value: []string{lo, hi}}}, label: fmt.Sprintf("%s m%02d", u.key, m)})
					}
				} else {
					u.parts = append(u.parts, &partition{scope: u, label: u.key})
				}
				units = append(units, u)
			}
		}
	}
	if opts.DryRun {
		for _, u := range units {
			u.report.Status = "planned"
			report.Scopes = append(report.Scopes, *u.report)
			report.Partitions += len(u.parts)
		}
		report.Elapsed = time.Since(start).Round(time.Millisecond).String()
		return report, nil
	}
	if len(units) == 0 {
		report.Elapsed = time.Since(start).Round(time.Millisecond).String()
		return report, nil
	}

	res, err := db.Exec(`INSERT INTO sync_runs (started_at, args, status) VALUES (?, ?, 'running')`, time.Now().UTC().Format(time.RFC3339), scopeArgs(scope))
	if err != nil {
		return nil, err
	}
	run, _ := res.LastInsertId()
	report.SyncRun = run

	// Request only columns the dataset really has: DKAN turns an unknown
	// property into a sparse array and rejects the whole query.
	fieldCache := map[string]map[string]bool{}
	for _, u := range units {
		if u.typ == TypeResearch {
			continue
		}
		fields, ok := fieldCache[u.ds.DatasetID]
		if !ok {
			fields, err = SchemaFields(ctx, g, u.ds.DatasetID)
			if err != nil {
				return nil, err
			}
			fieldCache[u.ds.DatasetID] = fields
		}
		for _, p := range Properties(u.typ) {
			if fields[p] {
				u.props = append(u.props, p)
			}
		}
	}
	w := newWriter(db, run)
	fetchCtx, cancelFetch := context.WithCancel(ctx)
	defer cancelFetch()
	pages := make(chan pageResult, opts.Concurrency*2)
	writeErr := make(chan error, 1)
	go func() { writeErr <- w.consume(pages, cancelFetch) }()

	var queue []*partition
	for _, u := range units {
		queue = append(queue, u.parts...)
		report.Partitions += len(u.parts)
	}
	work := make(chan *partition)
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	for i := 0; i < opts.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range work {
				fetchPartition(fetchCtx, g, p, opts.MaxPages, pages)
				mu.Lock()
				done++
				if p.err != nil {
					fmt.Fprintf(progress, "sync: %s failed: %v\n", p.label, p.err)
				} else {
					fmt.Fprintf(progress, "sync: [%d/%d] %s: %d rows\n", done, len(queue), p.label, p.rows)
				}
				mu.Unlock()
			}
		}()
	}
	for _, p := range queue {
		select {
		case work <- p:
		case <-fetchCtx.Done():
		}
		if fetchCtx.Err() != nil {
			break
		}
	}
	close(work)
	wg.Wait()
	close(pages)
	if err := <-writeErr; err != nil {
		_, _ = db.Exec(`UPDATE sync_runs SET finished_at=?, status='failed' WHERE sync_run=?`, time.Now().UTC().Format(time.RFC3339), run)
		return nil, err
	}

	for _, u := range units {
		failed := false
		for _, p := range u.parts {
			u.report.Rows += p.rows
			if p.err != nil {
				failed = true
				u.report.Error = p.err.Error()
			}
		}
		if ctx.Err() != nil {
			failed = true
			u.report.Error = ctx.Err().Error()
		}
		truncated := opts.MaxPages > 0 && anyTruncated(u, opts.MaxPages)
		if failed {
			u.report.Status = "failed"
			report.Failed++
		} else {
			u.report.Status = "synced"
			if !truncated && !u.filter.piPass && u.filter.where != "" {
				n, derr := w.reconcileDeletes(u)
				if derr != nil {
					return nil, derr
				}
				report.Deleted += n
			}
			complete := 1
			if truncated {
				complete = 0
			}
			_, err := db.Exec(`INSERT INTO sync_scopes (scope, payment_type, program_year, dataset_id, dataset_modified, filter, rows, sync_run, last_run, complete)
				VALUES (?,?,?,?,?,?,?,?,?,?) ON CONFLICT(scope) DO UPDATE SET dataset_id=excluded.dataset_id, dataset_modified=excluded.dataset_modified,
				rows=excluded.rows, sync_run=excluded.sync_run, last_run=excluded.last_run, complete=excluded.complete`,
				u.key, u.typ, u.year, u.ds.DatasetID, u.ds.Modified, u.filter.desc, u.report.Rows, run, time.Now().UTC().Format(time.RFC3339), complete)
			if err != nil {
				return nil, err
			}
		}
		report.Scopes = append(report.Scopes, *u.report)
	}
	report.RowsSeen = w.seen
	report.Added = w.added
	report.Amended = w.amended
	status := "ok"
	if report.Failed > 0 {
		status = "partial"
	}
	if err := RebuildDerived(db); err != nil {
		return nil, fmt.Errorf("rebuilding derived tables: %w", err)
	}
	_, _ = db.Exec(`UPDATE sync_runs SET finished_at=?, rows_seen=?, added=?, amended=?, deleted=?, status=? WHERE sync_run=?`,
		time.Now().UTC().Format(time.RFC3339), report.RowsSeen, report.Added, report.Amended, report.Deleted, status, run)
	report.Elapsed = time.Since(start).Round(time.Millisecond).String()
	return report, nil
}

func anyTruncated(u *scopeUnit, maxPages int) bool {
	for _, p := range u.parts {
		if p.pages >= maxPages && p.rows == p.pages*(MaxPageSize-1) {
			return true
		}
	}
	return false
}

func scopeArgs(s Scope) string {
	b, _ := json.Marshal(s)
	return string(b)
}

type pageResult struct {
	part *partition
	rows []map[string]any
}

func fetchPartition(ctx context.Context, g Getter, p *partition, maxPages int, out chan<- pageResult) {
	u := p.scope
	// DKAN rejects limit=500 combined with a long properties list
	// ("properties must be array"); 499 is accepted for every shape.
	pageSize := MaxPageSize - 1
	q := Query{
		Conditions: append(append([]Condition{}, u.filter.conds...), p.extra...),
		Limit:      pageSize,
		// A stable order keeps offset pages from skipping or repeating rows.
		Sorts: []Sort{{Property: "record_id"}},
	}
	// Selecting properties shrinks pages ~50x; research's PI slots make the
	// URL too long, so research pulls full rows (research volume is small).
	q.Properties = u.props
	for {
		if ctx.Err() != nil {
			p.err = ctx.Err()
			return
		}
		q.Offset = p.offset
		resp, err := RunQuery(ctx, g, u.ds.DatasetID, q)
		if err != nil {
			p.err = err
			return
		}
		n := len(resp.Results)
		if n > 0 {
			select {
			case out <- pageResult{part: p, rows: resp.Results}:
			case <-ctx.Done():
				p.err = ctx.Err()
				return
			}
		}
		p.rows += n
		p.pages++
		p.offset += n
		if n < pageSize || (maxPages > 0 && p.pages >= maxPages) {
			return
		}
	}
}

type writer struct {
	db      *sql.DB
	run     int64
	seen    int
	added   int
	amended int
	now     string
}

func newWriter(db *sql.DB, run int64) *writer {
	return &writer{db: db, run: run, now: time.Now().UTC().Format(time.RFC3339)}
}

func (w *writer) consume(pages <-chan pageResult, cancel func()) error {
	var firstErr error
	for pg := range pages {
		if firstErr != nil {
			continue // drain so fetchers never block
		}
		if err := w.writePage(pg); err != nil {
			firstErr = err
			if cancel != nil {
				cancel() // stop fetchers instead of paging for hours
			}
		}
	}
	return firstErr
}

func (w *writer) writePage(pg pageResult) error {
	u := pg.part.scope
	cols := Columns(u.typ)
	table := TableFor(u.typ)
	names := make([]string, 0, len(cols)+4)
	for _, c := range cols {
		names = append(names, c.Name)
	}
	names = append(names, "dataset_id", "dataset_modified", "synced_at", "sync_run")
	var updates []string
	for _, n := range names[2:] {
		updates = append(updates, n+"=excluded."+n)
	}
	insert := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s) ON CONFLICT(record_id, program_year) DO UPDATE SET %s`, // #nosec G201 -- table and column names come from the static schema in normalize.go (TableFor/Columns), never from input; values are bound as ? parameters
		table, strings.Join(names, ","), placeholders(len(names)), strings.Join(updates, ","))
	amountCol := "amount"
	if u.typ == TypeOwnership {
		amountCol = "amount_invested"
	}
	tx, err := w.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ins, err := tx.Prepare(insert)
	if err != nil {
		return err
	}
	defer ins.Close()
	prev, err := tx.Prepare(fmt.Sprintf(`SELECT COALESCE(%s, 0), COALESCE(dispute,'') FROM %s WHERE record_id=? AND program_year=?`, amountCol, table))
	if err != nil {
		return err
	}
	defer prev.Close()
	logChange, err := tx.Prepare(`INSERT INTO sync_changes (sync_run, payment_type, record_id, program_year, change, old_amount, new_amount, cms_change_type, npi, recipient_name, company) VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer logChange.Close()
	var delProd, insProd, delInv, insInv *sql.Stmt
	if u.typ != TypeOwnership {
		if delProd, err = tx.Prepare(`DELETE FROM products WHERE payment_type=? AND record_id=? AND program_year=?`); err != nil {
			return err
		}
		defer delProd.Close()
		if insProd, err = tx.Prepare(`INSERT OR REPLACE INTO products (payment_type, record_id, program_year, slot, covered, kind, category, name, ndc, pdi) VALUES (?,?,?,?,?,?,?,?,?,?)`); err != nil {
			return err
		}
		defer insProd.Close()
	}
	if u.typ == TypeResearch {
		if delInv, err = tx.Prepare(`DELETE FROM research_investigators WHERE record_id=? AND program_year=?`); err != nil {
			return err
		}
		defer delInv.Close()
		if insInv, err = tx.Prepare(`INSERT OR REPLACE INTO research_investigators (record_id, program_year, slot, npi, profile_id, first_name, last_name, name, recipient_type, city, state, zip5, country, primary_type, specialty) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`); err != nil {
			return err
		}
		defer insInv.Close()
	}
	for _, raw := range pg.rows {
		r := Row(SnakeKeys(raw))
		vals := make([]any, 0, len(names))
		for _, c := range cols {
			vals = append(vals, c.Value(r))
		}
		vals = append(vals, u.ds.DatasetID, u.ds.Modified, w.now, w.run)
		recordID, year := r.S("record_id"), r.I("program_year")
		if recordID == "" || year == nil {
			continue
		}
		newAmt, _ := r.F(amountSource(u.typ)).(float64)
		var oldAmt float64
		var oldDispute string
		perr := prev.QueryRow(recordID, year).Scan(&oldAmt, &oldDispute)
		existed := perr == nil
		if perr != nil && !errors.Is(perr, sql.ErrNoRows) {
			return perr
		}
		if _, err := ins.Exec(vals...); err != nil {
			return fmt.Errorf("upserting %s %s: %w", table, recordID, err)
		}
		w.seen++
		logIt := ""
		if !existed {
			w.added++
			if u.prevDone {
				logIt = "added"
			}
		} else if oldAmt != newAmt || oldDispute != r.S("dispute_status_for_publication") {
			w.amended++
			logIt = "amended"
		}
		if logIt != "" {
			name := PersonName(r.S("covered_recipient_first_name"), "", r.S("covered_recipient_last_name"), "")
			npi := r.S("covered_recipient_npi")
			if u.typ == TypeOwnership {
				name = PersonName(r.S("physician_first_name"), "", r.S("physician_last_name"), "")
				npi = r.S("physician_npi")
			}
			var old any
			if existed {
				old = oldAmt
			}
			if _, err := logChange.Exec(w.run, u.typ, recordID, year, logIt, old, newAmt, r.S("change_type"), npi, name, r.S(companyField)); err != nil {
				return err
			}
		}
		if delProd != nil {
			if _, err := delProd.Exec(u.typ, recordID, year); err != nil {
				return err
			}
			for _, p := range Products(r) {
				if _, err := insProd.Exec(u.typ, recordID, year, p.Slot, nullIf(p.Covered), nullIf(p.Kind), nullIf(p.Category), nullIf(p.Name), nullIf(p.NDC), nullIf(p.PDI)); err != nil {
					return err
				}
			}
		}
		if delInv != nil {
			if _, err := delInv.Exec(recordID, year); err != nil {
				return err
			}
			for _, inv := range Investigators(r) {
				if _, err := insInv.Exec(recordID, year, inv.Slot, nullIf(inv.NPI), nullIf(inv.ProfileID), nullIf(inv.First), nullIf(inv.Last), nullIf(inv.Name), nullIf(inv.Type), nullIf(inv.City), nullIf(inv.State), nullIf(inv.Zip5), nullIf(inv.Country), nullIf(inv.PrimaryType), nullIf(inv.Specialty)); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

func amountSource(typ string) string {
	if typ == TypeOwnership {
		return "total_amount_invested_usdollars"
	}
	return "total_amount_of_payment_usdollars"
}

func nullIf(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// reconcileDeletes removes local rows inside a fully re-synced scope that the
// API no longer returns (CMS deletes records between refreshes).
func (w *writer) reconcileDeletes(u *scopeUnit) (int, error) {
	table := TableFor(u.typ)
	where := "program_year = ? AND sync_run <> ? AND " + u.filter.where
	args := append([]any{u.year, w.run}, u.filter.args...)
	if len(u.parts) > 1 {
		// Month-partitioned fetches only see rows with a parseable payment
		// date in that year; never reconcile rows they could not have seen.
		where += " AND payment_date BETWEEN ? AND ?"
		args = append(args, fmt.Sprintf("%d-01-01", u.year), fmt.Sprintf("%d-12-31", u.year))
	}
	amountCol := "amount"
	if u.typ == TypeOwnership {
		amountCol = "amount_invested"
	}
	rows, err := w.db.Query(fmt.Sprintf(`SELECT record_id, %s, npi, recipient_name, company FROM %s WHERE %s`, amountCol, table, where), args...)
	if err != nil {
		return 0, err
	}
	type gone struct {
		id                 string
		amt                sql.NullFloat64
		npi, name, company sql.NullString
	}
	var dead []gone
	for rows.Next() {
		var g gone
		if err := rows.Scan(&g.id, &g.amt, &g.npi, &g.name, &g.company); err != nil {
			_ = rows.Close()
			return 0, err
		}
		dead = append(dead, g)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(dead) == 0 {
		return 0, nil
	}
	tx, err := w.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, g := range dead {
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM %s WHERE record_id=? AND program_year=?`, table), g.id, u.year); err != nil {
			return 0, err
		}
		_, _ = tx.Exec(`DELETE FROM products WHERE payment_type=? AND record_id=? AND program_year=?`, u.typ, g.id, u.year)
		_, _ = tx.Exec(`DELETE FROM research_investigators WHERE record_id=? AND program_year=?`, g.id, u.year)
		if _, err := tx.Exec(`INSERT INTO sync_changes (sync_run, payment_type, record_id, program_year, change, old_amount, new_amount, cms_change_type, npi, recipient_name, company) VALUES (?,?,?,?, 'removed', ?, NULL, NULL, ?, ?, ?)`,
			w.run, u.typ, g.id, u.year, g.amt, g.npi, g.name, g.company); err != nil {
			return 0, err
		}
	}
	return len(dead), tx.Commit()
}

// RebuildDerived refreshes recipients, hospitals, reporting entities and FTS
// indexes from the payment tables.
func RebuildDerived(db *sql.DB) error {
	stmts := []string{
		`DELETE FROM recipients`,
		`INSERT INTO recipients (recipient_key, npi, profile_id, name, first_name, last_name, recipient_type, specialty, city, state, zip5, first_year, last_year, total_general, total_research, total_ownership)
		 SELECT key, MAX(npi), MAX(profile_id), MAX(name), MAX(first_name), MAX(last_name), MAX(recipient_type), MAX(specialty), MAX(city), MAX(state), MAX(zip5),
		        MIN(program_year), MAX(program_year), SUM(g), SUM(r), SUM(o)
		 FROM (
		   SELECT COALESCE(npi, 'profile:'||profile_id) key, npi, profile_id, recipient_name name, first_name, last_name, recipient_type, specialty, city, state, zip5, program_year, amount g, 0 r, 0 o
		     FROM payments_general WHERE (npi IS NOT NULL OR profile_id IS NOT NULL) AND first_name IS NOT NULL
		   UNION ALL
		   SELECT COALESCE(npi, 'profile:'||profile_id), npi, profile_id, recipient_name, first_name, last_name, recipient_type, specialty, city, state, zip5, program_year, 0, amount, 0
		     FROM payments_research WHERE (npi IS NOT NULL OR profile_id IS NOT NULL) AND first_name IS NOT NULL
		   UNION ALL
		   SELECT COALESCE(npi, 'profile:'||profile_id), npi, profile_id, recipient_name, first_name, last_name, 'Covered Recipient Physician', specialty, city, state, zip5, program_year, 0, 0, amount_invested
		     FROM payments_ownership WHERE npi IS NOT NULL OR profile_id IS NOT NULL
		   UNION ALL
		   SELECT COALESCE(npi, 'profile:'||profile_id), npi, profile_id, name, first_name, last_name, recipient_type, specialty, city, state, zip5, program_year, 0, 0, 0
		     FROM research_investigators WHERE npi IS NOT NULL OR profile_id IS NOT NULL
		 ) GROUP BY key`,
		`UPDATE recipients SET specialty = (
		   SELECT specialty FROM (
		     SELECT specialty, COUNT(*) c, MAX(program_year) y FROM payments_general g WHERE g.npi = recipients.npi AND specialty IS NOT NULL GROUP BY specialty
		     UNION ALL SELECT specialty, COUNT(*), MAX(program_year) FROM research_investigators i WHERE i.npi = recipients.npi AND specialty IS NOT NULL GROUP BY specialty
		   ) ORDER BY c DESC, y DESC LIMIT 1)
		 WHERE npi IS NOT NULL AND EXISTS (SELECT 1 FROM payments_general g WHERE g.npi = recipients.npi UNION ALL SELECT 1 FROM research_investigators i WHERE i.npi = recipients.npi)`,
		`DELETE FROM teaching_hospitals`,
		`INSERT OR REPLACE INTO teaching_hospitals (ccn, hospital_id, name, city, state, zip5)
		 SELECT teaching_hospital_ccn, MAX(teaching_hospital_id), MAX(teaching_hospital_name), MAX(city), MAX(state), MAX(zip5) FROM (
		   SELECT teaching_hospital_ccn, teaching_hospital_id, teaching_hospital_name, city, state, zip5 FROM payments_general WHERE teaching_hospital_ccn IS NOT NULL
		   UNION ALL SELECT teaching_hospital_ccn, teaching_hospital_id, teaching_hospital_name, city, state, zip5 FROM payments_research WHERE teaching_hospital_ccn IS NOT NULL
		 ) GROUP BY teaching_hospital_ccn`,
		`DELETE FROM reporting_entities`,
		`INSERT OR REPLACE INTO reporting_entities (company_id, name, state, country)
		 SELECT company_id, MAX(company), MAX(company_state), MAX(company_country) FROM (
		   SELECT company_id, company, company_state, company_country FROM payments_general
		   UNION ALL SELECT company_id, company, company_state, company_country FROM payments_research
		   UNION ALL SELECT company_id, company, company_state, company_country FROM payments_ownership
		 ) WHERE company_id IS NOT NULL GROUP BY company_id`,
		`DELETE FROM general_pairs`,
		`INSERT OR REPLACE INTO general_pairs (state, program_year, npi, company, specialties, total, n)
		 SELECT state, program_year, npi, company, MAX(specialties), SUM(amount), COUNT(*) FROM payments_general
		 WHERE npi IS NOT NULL GROUP BY state, program_year, npi, company`,
		`DELETE FROM company_names`,
		`INSERT OR REPLACE INTO company_names (company, company_id)
		 SELECT company, MAX(company_id) FROM (
		   SELECT DISTINCT company, company_id FROM payments_general
		   UNION ALL SELECT DISTINCT company, company_id FROM payments_research
		   UNION ALL SELECT DISTINCT company, company_id FROM payments_ownership
		 ) WHERE company IS NOT NULL GROUP BY company`,
		`DELETE FROM recipients_fts`,
		`INSERT INTO recipients_fts (recipient_key, name, specialty, city, state) SELECT recipient_key, name, specialty, city, state FROM recipients`,
		`DELETE FROM companies_fts`,
		`INSERT INTO companies_fts (company_id, name) SELECT company_id, name FROM reporting_entities`,
		`DELETE FROM products_fts`,
		`INSERT INTO products_fts (name, category, kind) SELECT DISTINCT name, category, kind FROM products WHERE name IS NOT NULL`,
		`DELETE FROM studies_fts`,
		`INSERT INTO studies_fts (nct_id, name_of_study) SELECT nct_id, MAX(name_of_study) FROM payments_research WHERE name_of_study IS NOT NULL GROUP BY COALESCE(nct_id, name_of_study)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("%w (%s)", err, firstLine(s))
		}
	}
	return nil
}
