package op

import (
	"database/sql"
	"fmt"
	"strings"
)

// recipientCols are shared by the general and research payment tables.
const recipientCols = `
	recipient_type TEXT, npi TEXT, profile_id TEXT,
	first_name TEXT, middle_name TEXT, last_name TEXT, recipient_name TEXT,
	street TEXT, city TEXT, state TEXT, zip5 TEXT, zip TEXT, country TEXT,
	primary_type TEXT, specialty TEXT, specialties TEXT,
	teaching_hospital_ccn TEXT, teaching_hospital_id TEXT, teaching_hospital_name TEXT,
	company TEXT, company_id TEXT, company_state TEXT, company_country TEXT, submitting_company TEXT,
	amount REAL, payment_date TEXT, form TEXT, related_product TEXT,
	dispute TEXT, delay_publication TEXT, publication_date TEXT,`

const provenanceCols = `
	dataset_id TEXT, dataset_modified TEXT, synced_at TEXT, sync_run INTEGER,`

// DDL is the typed local schema. Every payment row carries program_year,
// record_id and the dataset modified date for provenance.
var DDL = []string{
	`CREATE TABLE IF NOT EXISTS payments_general (
	record_id TEXT NOT NULL, program_year INTEGER NOT NULL, change_type TEXT,` + recipientCols + `
	n_payments INTEGER, nature TEXT, travel_city TEXT, travel_state TEXT, travel_country TEXT,
	physician_ownership TEXT, third_party TEXT, charity TEXT, context TEXT,` + provenanceCols + `
	PRIMARY KEY (record_id, program_year))`,
	`CREATE TABLE IF NOT EXISTS payments_research (
	record_id TEXT NOT NULL, program_year INTEGER NOT NULL, change_type TEXT,` + recipientCols + `
	noncovered_entity TEXT, name_of_study TEXT, nct_id TEXT, context_of_research TEXT,
	preclinical TEXT, research_link TEXT, expenditure TEXT,` + provenanceCols + `
	PRIMARY KEY (record_id, program_year))`,
	`CREATE TABLE IF NOT EXISTS payments_ownership (
	record_id TEXT NOT NULL, program_year INTEGER NOT NULL, change_type TEXT,
	npi TEXT, profile_id TEXT, first_name TEXT, middle_name TEXT, last_name TEXT, recipient_name TEXT,
	street TEXT, city TEXT, state TEXT, zip5 TEXT, zip TEXT, country TEXT, primary_type TEXT, specialty TEXT,
	amount_invested REAL, value_of_interest REAL, terms TEXT, held_by TEXT,
	company TEXT, company_id TEXT, company_state TEXT, company_country TEXT, submitting_company TEXT,
	dispute TEXT, publication_date TEXT,` + provenanceCols + `
	PRIMARY KEY (record_id, program_year))`,
	`CREATE TABLE IF NOT EXISTS research_investigators (
	record_id TEXT NOT NULL, program_year INTEGER NOT NULL, slot INTEGER NOT NULL,
	npi TEXT, profile_id TEXT, first_name TEXT, last_name TEXT, name TEXT, recipient_type TEXT,
	city TEXT, state TEXT, zip5 TEXT, country TEXT, primary_type TEXT, specialty TEXT,
	PRIMARY KEY (record_id, program_year, slot))`,
	`CREATE TABLE IF NOT EXISTS products (
	payment_type TEXT NOT NULL, record_id TEXT NOT NULL, program_year INTEGER NOT NULL, slot INTEGER NOT NULL,
	covered TEXT, kind TEXT, category TEXT, name TEXT, ndc TEXT, pdi TEXT,
	PRIMARY KEY (payment_type, record_id, program_year, slot))`,
	`CREATE TABLE IF NOT EXISTS recipients (
	recipient_key TEXT PRIMARY KEY, npi TEXT, profile_id TEXT, name TEXT, first_name TEXT, last_name TEXT,
	recipient_type TEXT, specialty TEXT, city TEXT, state TEXT, zip5 TEXT,
	first_year INTEGER, last_year INTEGER, total_general REAL, total_research REAL, total_ownership REAL)`,
	`CREATE TABLE IF NOT EXISTS teaching_hospitals (
	ccn TEXT PRIMARY KEY, hospital_id TEXT, name TEXT, city TEXT, state TEXT, zip5 TEXT)`,
	`CREATE TABLE IF NOT EXISTS reporting_entities (
	company_id TEXT PRIMARY KEY, name TEXT, state TEXT, country TEXT)`,
	`CREATE TABLE IF NOT EXISTS company_names (
	company TEXT PRIMARY KEY, company_id TEXT)`,
	`CREATE TABLE IF NOT EXISTS general_pairs (
	state TEXT, program_year INTEGER NOT NULL, npi TEXT NOT NULL, company TEXT, specialties TEXT, total REAL, n INTEGER,
	PRIMARY KEY (state, program_year, npi, company))`,
	`CREATE TABLE IF NOT EXISTS dataset_registry (
	dataset_id TEXT PRIMARY KEY, year INTEGER, type TEXT, title TEXT, distribution_id TEXT,
	modified TEXT, download_url TEXT, row_count INTEGER, resolved_at TEXT)`,
	`CREATE TABLE IF NOT EXISTS sync_scopes (
	scope TEXT PRIMARY KEY, payment_type TEXT, program_year INTEGER, dataset_id TEXT, dataset_modified TEXT,
	filter TEXT, rows INTEGER, cursor TEXT, sync_run INTEGER, last_run TEXT, complete INTEGER)`,
	`CREATE TABLE IF NOT EXISTS sync_runs (
	sync_run INTEGER PRIMARY KEY AUTOINCREMENT, started_at TEXT, finished_at TEXT, args TEXT,
	rows_seen INTEGER, added INTEGER, amended INTEGER, deleted INTEGER, status TEXT)`,
	`CREATE TABLE IF NOT EXISTS sync_changes (
	sync_run INTEGER, payment_type TEXT, record_id TEXT, program_year INTEGER, change TEXT,
	old_amount REAL, new_amount REAL, cms_change_type TEXT, npi TEXT, recipient_name TEXT, company TEXT)`,
	`CREATE TABLE IF NOT EXISTS trials (
	nct_id TEXT PRIMARY KEY, title TEXT, status TEXT, phase TEXT, sponsor TEXT, sponsor_class TEXT,
	conditions TEXT, start_date TEXT, fetched_at TEXT, raw TEXT)`,
	`CREATE TABLE IF NOT EXISTS trial_locations (
	nct_id TEXT, facility TEXT, city TEXT, state TEXT, zip5 TEXT, country TEXT, status TEXT, lat REAL, lon REAL)`,
	`CREATE TABLE IF NOT EXISTS trial_searches (
	query_key TEXT PRIMARY KEY, nct_ids TEXT, fetched_at TEXT)`,
	`CREATE TABLE IF NOT EXISTS zip_centroids (zip5 TEXT PRIMARY KEY, lat REAL, lon REAL)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS recipients_fts USING fts5(recipient_key UNINDEXED, name, specialty, city, state)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS companies_fts USING fts5(company_id UNINDEXED, name)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS products_fts USING fts5(name, category, kind)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS studies_fts USING fts5(nct_id UNINDEXED, name_of_study)`,
	`CREATE INDEX IF NOT EXISTS ix_gen_npi ON payments_general(npi)`,
	`CREATE INDEX IF NOT EXISTS ix_gen_state_year ON payments_general(state, program_year)`,
	`CREATE INDEX IF NOT EXISTS ix_gen_company ON payments_general(company)`,
	`CREATE INDEX IF NOT EXISTS ix_gen_ccn ON payments_general(teaching_hospital_ccn)`,
	`CREATE INDEX IF NOT EXISTS ix_gen_run ON payments_general(sync_run)`,
	// Covering index for state/year/specialty/company rollups (top, company, kol...).
	`CREATE INDEX IF NOT EXISTS ix_gen_cover ON payments_general(state, program_year, npi, company, amount, specialties, nature, record_id, dataset_modified)`,
	`CREATE INDEX IF NOT EXISTS ix_res_npi ON payments_research(npi)`,
	`CREATE INDEX IF NOT EXISTS ix_res_nct ON payments_research(nct_id)`,
	`CREATE INDEX IF NOT EXISTS ix_res_state_year ON payments_research(state, program_year)`,
	`CREATE INDEX IF NOT EXISTS ix_res_company ON payments_research(company)`,
	`CREATE INDEX IF NOT EXISTS ix_res_ccn ON payments_research(teaching_hospital_ccn)`,
	`CREATE INDEX IF NOT EXISTS ix_own_npi ON payments_ownership(npi)`,
	`CREATE INDEX IF NOT EXISTS ix_own_state ON payments_ownership(state, program_year)`,
	`CREATE INDEX IF NOT EXISTS ix_inv_npi ON research_investigators(npi)`,
	`CREATE INDEX IF NOT EXISTS ix_inv_state ON research_investigators(state)`,
	`CREATE INDEX IF NOT EXISTS ix_prod_name ON products(name)`,
	`CREATE INDEX IF NOT EXISTS ix_changes_run ON sync_changes(sync_run)`,
	`CREATE INDEX IF NOT EXISTS ix_tloc_nct ON trial_locations(nct_id)`,
	`CREATE INDEX IF NOT EXISTS ix_tloc_state ON trial_locations(state)`,
}

// EnsureSchema creates every domain table if absent.
func EnsureSchema(db *sql.DB) error {
	for _, stmt := range DDL {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("creating schema: %w\n%s", err, firstLine(stmt))
		}
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}

// DomainTables lists the tables `schema` documents, in display order.
var DomainTables = []string{
	"payments_general", "payments_research", "research_investigators", "payments_ownership",
	"products", "recipients", "teaching_hospitals", "reporting_entities", "company_names", "general_pairs",
	"dataset_registry", "sync_scopes", "sync_runs", "sync_changes",
	"trials", "trial_locations", "zip_centroids",
}

// TableDescriptions are one-line glosses shown by `schema` and `ask`.
var TableDescriptions = map[string]string{
	"payments_general":       "General payments (meals, travel, consulting, speaking, royalties...). One row per record_id+program_year. amount is USD.",
	"payments_research":      "Research payments. nct_id = ClinicalTrials.gov ID; name_of_study; PIs in research_investigators.",
	"research_investigators": "Principal investigators 1-5 of each research payment, one row per slot. Join on record_id+program_year.",
	"payments_ownership":     "Physician ownership and investment interests. amount_invested and value_of_interest in USD.",
	"products":               "Drugs/devices/supplies tied to a payment (slots 1-5). payment_type is general or research.",
	"recipients":             "One row per recipient (NPI, or profile id when no NPI) with first/last synced year and totals.",
	"teaching_hospitals":     "Teaching hospitals seen in synced payments, keyed by CCN.",
	"reporting_entities":     "Manufacturers and GPOs making payments, keyed by company_id.",
	"general_pairs":          "Derived rollup of general payments per state, program_year, npi and company (total, n); rebuilt on every sync.",
	"company_names":          "Every distinct company name as spelled on synced payment rows, with its company_id (resolves --company patterns).",
	"dataset_registry":       "CMS dataset IDs per program year and type (dataset_id for query, distribution_id for SQL).",
	"sync_scopes":            "One row per synced scope (type x year x filter) with the dataset modified date it reflects.",
	"sync_runs":              "History of sync runs with added/amended/deleted counts.",
	"sync_changes":           "Per-record changes detected by each sync run (added, amended, deleted).",
	"trials":                 "Cached ClinicalTrials.gov studies (status, phase, lead sponsor, conditions).",
	"trial_locations":        "Cached ClinicalTrials.gov site locations with USPS state codes.",
	"zip_centroids":          "US ZIP code centroids (Census ZCTA) used by `near`.",
}
