# Master Plan: Open Payments CLI (openpayments-pp-cli)

**Owner:** Greg Van Horn
**Tool:** CLI Printing Press (https://github.com/mvanhorn/cli-printing-press)
**Goal:** Print an agent-native CLI + MCP server for CMS Open Payments that can answer any question about industry payments to doctors and teaching hospitals, fast and offline.

---

## 1. Mission

Build `openpayments-pp-cli` and `openpayments-pp-mcp` with the Printing Press.

The finished tool must let an agent answer questions like these in one or two commands:

- "Which pain and spine doctors in PA and NJ got research payments in 2024, and for which trials?"
- "Which device companies paid orthopedic surgeons in Montgomery County the most, 2019–2025?"
- "Show every payment Stryker made to Dr. X, by year and by nature of payment."
- "Which sponsors pay principal investigators in NJ for NCT05XXXXXX?"
- "Which doctors are new to industry payments this year?"

---

## 2. Verified facts about the API (tested live, 2026-09-26)

Treat these as ground truth. Re-check them in Phase 0.

| Fact | Detail |
|---|---|
| Base URL | `https://openpaymentsdata.cms.gov/api/1` |
| Auth | None. No key, no signup. |
| Platform | DKAN (open-source government data catalog) |
| OpenAPI spec | `GET https://openpaymentsdata.cms.gov/api/1` returns an OpenAPI 3.0.2 document |
| Dataset catalog | `GET /metastore/schemas/dataset/items` → 74 datasets |
| Row query | `GET /datastore/query/{datasetId}/0` with `conditions`, `properties`, `sorts`, `limit`, `offset` |
| Server aggregation | Works. `properties[n][expression][operator]=sum` + `groupings[]` + `sorts[]` |
| SQL endpoint | `GET /datastore/sql?query=[SELECT ... FROM {distributionId}][WHERE ...][LIMIT n]` — needs the **distribution ID**, not the dataset ID |
| Row limit | **500 rows per call** max. Paginate with `offset`. |
| Count only | `results=false` returns the count and schema without rows |
| Bulk CSV | Each dataset lists a `downloadURL` in its metastore item (`show-reference-ids=true`) |
| Scale | 2024 General Payments = **15,498,687 rows** nationally. PA alone = 670,866. |
| Freshness | 2024 General dataset `modified` = 2026-06-30 (CMS publishes each June, refreshes in January) |

### Dataset families (one per program year, 2019–2025)

- **General Payment Data** — meals, travel, consulting, speaking, royalties, etc. 91 columns.
- **Research Payment Data** — 817,215 rows in 2024. Includes principal investigator 1–5 (NPI, name, specialty, address), `name_of_study`, **`clinicaltrials_gov_identifier`**, `context_of_research`, `preclinical_research_indicator`.
- **Ownership Payment Data** — physician ownership and investment interests.
- **Profiles** — Covered Recipient Profile Supplement, Physician (distinct) profile, Teaching Hospital profile, Reporting Entity profile, Provider profile ID mapping.
- **Pre-grouped summaries** — by reporting entity, covered recipient, nature of payment, state, specialty, national totals.

### Key columns (General Payments)

`covered_recipient_npi`, `covered_recipient_profile_id`, `covered_recipient_first_name`, `covered_recipient_last_name`, `covered_recipient_specialty_1..6`, `recipient_city`, `recipient_state`, `recipient_zip_code`, `teaching_hospital_ccn`, `applicable_manufacturer_or_applicable_gpo_making_payment_name`, `applicable_manufacturer_or_applicable_gpo_making_payment_id`, `total_amount_of_payment_usdollars`, `date_of_payment`, `nature_of_payment_or_transfer_of_value`, `form_of_payment_or_transfer_of_value`, `name_of_drug_or_biological_or_device_or_medical_supply_1..5`, `product_category_or_therapeutic_area_1..5`, `record_id`, `program_year`, `dispute_status_for_publication`.

### Traps to handle

1. **Dataset IDs change every year and every refresh.** Never hard-code them. Resolve them at runtime from the metastore by title pattern (`"{year} General Payment Data"`, etc.) and cache the map.
2. **SQL uses distribution IDs; query uses dataset IDs.** Store both.
3. **All values come back as strings,** including dollar amounts. Cast on ingest.
4. **SQL results use Title_Case keys;** query results use snake_case. Normalize to snake_case.
5. **Records get corrected or deleted** between refreshes (`change_type`). Upsert on `record_id` + `program_year`.
6. **National data is huge.** Do not sync all 15M+ rows per year by default.

---

## 3. How to run the Printing Press

### Setup

```bash
# Prereqs: Go 1.26.6+, Node/npm, Claude Code
curl -fsSL https://raw.githubusercontent.com/mvanhorn/cli-printing-press/main/scripts/install.sh | bash
cli-printing-press --version
# Restart Claude Code so the skills load
```

### Print

Inside Claude Code:

```
/printing-press https://openpaymentsdata.cms.gov/api/1
```

If the press does not pick up the spec from the URL, save it and pass it:

```bash
curl -s https://openpaymentsdata.cms.gov/api/1 -o openpayments-openapi.json
```
```
/printing-press --spec ./openpayments-openapi.json --name openpayments
```

Optional: add `codex` to cut Opus token use in Phase 3.

### Steer the run

Paste Sections 2, 4, 5, and 6 of this plan into the session when the press asks for the product thesis and absorb manifest. Approve the manifest only when it includes every item in Section 5.

---

## 4. Product thesis (the Non-Obvious Insight)

> "Open Payments isn't just a transparency database. It's a **map of who industry trusts.** Every payment is a signal about which doctors a company invests in, which sites run its trials, and which relationships are growing or cooling."

Use this NOI in Phase 0. It drives the compound commands in Section 5.

---

## 5. Required feature set

### Layer 1 — Wrap every endpoint (Rung 1–2)

- `datasets list` / `datasets resolve --year 2024 --type general|research|ownership` (metastore)
- `query` (datastore query with conditions, properties, groupings, sorts)
- `sql` (DKAN bracket SQL, auto-resolving dataset → distribution ID)
- `search` (catalog search + facets)
- `download` (bulk CSV by year and type)
- All standard agent flags: `--json`, `--select`, `--compact`, `--csv`, `--dry-run`, `--limit`, auto-JSON when piped, typed exit codes.

### Layer 2 — Local data layer (Rung 3)

SQLite with real columns, not JSON blobs. FTS5 on names, companies, products, and study names.

**Tables:**
- `payments_general` (typed columns from Section 2, `amount` as REAL, `date_of_payment` as DATE)
- `payments_research` (plus PI 1–5 flattened into `research_investigators` child table, `nct_id`)
- `payments_ownership`
- `recipients` (NPI, profile_id, name, specialties, city, state, zip)
- `teaching_hospitals` (CCN, name, address)
- `reporting_entities` (company ID, name, state, country)
- `products` (payment ↔ drug/device/category, 1–5 per payment, normalized to rows)
- `dataset_registry` (year, type, dataset_id, distribution_id, modified, row_count)
- `sync_state` (scope, cursor, last_run)

**Sync strategy — scoped, not national:**

```bash
openpayments-pp-cli sync --years 2019-2025 --types general,research,ownership --states PA,NJ
openpayments-pp-cli sync --specialty "Pain Medicine,Orthopaedic Surgery,Neurological Surgery,Physical Medicine"
openpayments-pp-cli sync --npi 1234567890
openpayments-pp-cli sync --company "Stryker Corporation"
openpayments-pp-cli sync --full --year 2024 --type research   # uses bulk CSV, not the API
```

- Page at 500 rows with `offset`. Run pages in parallel with `--concurrency` (default 4). Back off on 429 or 5xx.
- For `--full` pulls, stream the bulk CSV into SQLite. Do not page 15M rows through the API.
- Incremental: compare `dataset_registry.modified`. Re-sync only datasets whose `modified` changed.
- `--data-source auto|local|live`: live API for one-off questions outside the synced scope; local for everything else.

### Layer 3 — Domain commands (Rung 4)

| Command | What it answers |
|---|---|
| `doctor <npi or name>` | Full dossier: every payment by year, company, nature, product. Totals and trend. |
| `company <name>` | Who a company pays: top recipients, specialties, states, products, by year. |
| `top --by recipient\|company\|product --state PA --specialty ... --year ...` | Leaderboards. |
| `research --state PA,NJ --specialty ... ` | Research payments with PI, study name, NCT ID, sponsor. |
| `investigators --nct NCT05...` | Every PI paid for one trial, with location. |
| `hospital <ccn or name>` | Payments to a teaching hospital. |
| `ownership --state ...` | Physician ownership and investment interests. |
| `nature` | Breakdown by nature of payment (consulting, food, travel, royalties, research...). |
| `compare <npi> <npi>` | Two doctors side by side. |
| `near --zip 19002 --miles 25` | Recipients near a ZIP (use a bundled ZIP centroid table). |

### Layer 4 — Insight commands (Rung 5)

| Command | Logic |
|---|---|
| `new-recipients --year 2025` | NPIs with payments this year and none in any prior synced year. |
| `rising` / `cooling` | Recipients or company-recipient pairs with the largest YoY change in total dollars. |
| `relationships <npi>` | Every company that paid a doctor, first year, last year, total, and trend. |
| `research-sites --specialty ... --state ...` | Doctors who get **research** payments (proven trial sites) vs. only general payments. |
| `kol --specialty ... --state ...` | Key-opinion-leader score: speaking + consulting dollars, number of companies, years active. |
| `concentration <company>` | How much of a company's spend goes to its top 10 recipients. |
| `product <name>` | Every doctor paid in connection with a drug or device. |
| `changed --since-last-sync` | Records added, corrected, or removed since the prior sync. |

### Layer 5 — Cross-join with ClinicalTrials.gov

The `clinical-trials` CLI already exists in the Printing Press Library (`npx -y @mvanhorn/printing-press-library install clinical-trials`).

Add a `trials` command group that joins on `nct_id`:

- `trials sites --nct NCT05...` → trial status, phase, sponsor, locations **plus** every Open Payments PI paid for it.
- `trials gaps --condition "low back pain" --state PA,NJ` → recruiting trials whose sponsor pays **no** local investigator. These are open doors.
- `trials sponsor <company>` → a sponsor's active trials plus the sites it already pays.

Implement the join locally: call the ClinicalTrials.gov v2 API (`https://clinicaltrials.gov/api/v2/studies/{nct}`) or shell out to `clinical-trials-pp-cli --json` if installed. Cache results in a `trials` table.

---

## 6. "Ask it any question" — the natural-language layer

The Printing Press makes the CLI agent-native. The agent does the reasoning. To make any question answerable:

1. **`sql` against the local store** is the escape hatch. Expose the full schema with `schema --json` so an agent can write its own SQL.
2. **`ask "<question>"`** (optional): print the schema, a column glossary, and 10 worked examples, then let the calling agent compose SQL. Do not embed an LLM in the binary.
3. **Column glossary:** ship `glossary.md` that maps plain words to columns ("company" → `applicable_manufacturer_or_applicable_gpo_making_payment_name`, "specialty" → `covered_recipient_specialty_1..6`, "trial" → `clinicaltrials_gov_identifier`).
4. **Skill file** (`/pp-openpayments`) with the 20 test questions in Section 8 as worked examples.

---

## 7. Build phases and gates

| Phase | Work | Gate to pass |
|---|---|---|
| 0 | Resolve spec, confirm facts in Section 2, write NOI | All Section 2 facts re-verified |
| 1 | Research brief: competitors (Apify Open Payments scrapers, API Evangelist profile, any GitHub/PyPI Open Payments clients, ProPublica-style doctor lookups) | Competitor list with features |
| 1.5 | Absorb manifest | Includes all of Section 5 |
| 2 | Generate CLI + MCP from spec | Builds; wrapper commands hit live API |
| 3 | Data layer + domain + insight commands + trials join | Section 8 tests pass locally |
| 4 | Shipcheck: dogfood, verify, scorecard | Grade A (85+), all proofs pass |
| 5 | Live smoke test (read-only) | PASS |
| 5.5 | `/printing-press-polish openpayments` | No dead flags, clean README |
| 6 | Optional: `/printing-press-publish openpayments` | PR opened to the public library |

---

## 8. Acceptance tests

Seed data: `sync --years 2023-2025 --types general,research,ownership --states PA,NJ`.

Each question must be answerable with at most two commands, return in under 2 seconds from local data, and match a live-API spot check.

1. Top 20 PA recipients by total general payments in 2024.
2. Top 10 companies paying PA Pain Medicine doctors, 2019–2025.
3. Every research payment in NJ with an NCT ID, 2024.
4. All PIs paid for a given NCT ID.
5. A single doctor's full dossier by NPI.
6. Doctors in PA new to industry payments in 2025.
7. Largest YoY increase for any company–doctor pair in PA.
8. Stryker's total payments to Orthopaedic Surgeons in PA by year.
9. Breakdown of a doctor's payments by nature of payment.
10. Teaching hospitals in PA ranked by research dollars.
11. Physician ownership interests in NJ, grouped by company.
12. Doctors within 25 miles of ZIP 19002 with research payments.
13. Products (devices) most linked to payments to neurosurgeons in PA.
14. Doctors who received research payments from 3+ sponsors.
15. KOL ranking for Physical Medicine & Rehabilitation in PA.
16. Recruiting spine trials in PA/NJ whose sponsor pays no local PI (`trials gaps`).
17. Records changed since the last sync.
18. Concentration of spend for a named company.
19. Same question via MCP tool call returns the same result as the CLI.
20. `sql` with a custom query the agent writes from `schema --json`.

---

## 9. Guardrails

- **Read-only.** The API is public data. Never call write endpoints (`datastore/imports`, `harvest/*`).
- **Be polite to CMS.** Default concurrency 4, adaptive backoff, cache aggressively.
- **Facts only.** Report payments as published. Never label a payment improper or a doctor conflicted. Show `dispute_status_for_publication` when present.
- **Personal data.** Recipient names and business addresses are public records. Do not enrich with personal contact data from other sources.
- **Provenance.** Every output row carries `program_year`, `record_id`, and the dataset `modified` date.

---

## 10. Deliverables

- `openpayments-pp-cli` (Go binary) and `openpayments-pp-mcp` (MCP server)
- `README.md` with a cookbook of the Section 8 questions
- `glossary.md` and `schema --json`
- `/pp-openpayments` skill
- Research manuscript, verification proofs, scorecard
- Optional: library PR via `/printing-press-publish`

Sources:
- CLI Printing Press — https://github.com/mvanhorn/cli-printing-press
- Printing Press Library — https://github.com/mvanhorn/printing-press-library
- Open Payments API — https://openpaymentsdata.cms.gov/about/api
- DKAN endpoint reference — https://downloads.cms.gov/files/OpenPaymentsData-DKAN-API-Endpoints.pdf
- ClinicalTrials.gov API v2 — https://clinicaltrials.gov/data-api/api
