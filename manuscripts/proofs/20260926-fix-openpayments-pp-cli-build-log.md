Manifest transcendence rows: 16 planned, 16 built. Phase 3 will not pass until all 16 ship.

## Phase 2 notes
- Generated from read-only overlay (9 GET endpoints; harvest/imports/revisions/download/write methods removed; basic_auth scheme dropped; servers set).
- Hand-edit internal/client/client.go default User-Agent: CMS Akamai returns 403 for any UA containing 'cli' (verified: curl -A openpayments-pp-cli/v1 → 403, openpayments-pp/1.0 → 200). OPENPAYMENTS_USER_AGENT override preserved.
- Wrapper commands verified live: datastore datasetindex-query-get, datastore sql, catalog search, metastore get-all.

## Phase 3 build (2026-09-26)
### Built
- internal/op: dataset registry (title → year/type, dataset+distribution IDs, 24h cache), DKAN query builder (clamped pages, append-syntax properties, aggregate validation), bracket SQL with Title_Case → snake_case, typed schema (payments_general/research/ownership, research_investigators, products, recipients, teaching_hospitals, reporting_entities, dataset_registry, sync_scopes, sync_runs, sync_changes, trials, trial_locations, zip_centroids, FTS5 x4), scoped sync engine (state/specialty/NPI/company/month partitions, 4 workers, single writer, upsert on record_id+program_year, change log, deletion reconciliation, incremental by dataset modified), bulk CSV streaming with in-stream scope filter, company profile resolution, ClinicalTrials.gov v2 client with adaptive limiter + sponsor matching, bundled Census ZCTA centroids, glossary + worked examples.
- Commands: sync (domain flags via hook), datasets list/resolve, download, query, sql (local + DKAN bracket), schema, dossier, company, top, research, hospital, ownership, nature, product; novel: kol, rising, cooling, new-recipients, relationships, compare, overlap, roster, concentration, research-sites, investigators, near, changed, ask, trials sites/gaps/sponsor.
- Tests: internal/op/op_test.go (12 tests incl. sync upsert/amend/delete lifecycle against a fake DKAN).
### Decisions / generator findings
- Framework `sync_state` table name collides with a domain table → renamed domain table to sync_scopes; domain syncs mirror counts into framework sync_state so generated hints work.
- DKAN quirks found: url.Values key sorting makes properties[10] precede properties[2] → PHP object → 400; unknown property names also produce sparse arrays; limit=500 + long property list → 400 (499 works). Sync fetches the live dataset schema and uses 499-row pages.
- Plan's `doctor` → `dossier` and catalog `search` → `catalog search` (framework-reserved names).
- validate-narrative --framework-only flags `sync --years/--states` (static framework vocabulary); full-example validation against the binary is authoritative.
### Deferred
- none of the approved rows. trials gaps/sponsor sponsor matching is name-based (printed pairs + --sponsor-alias) per brainstorm verifiability flag.
