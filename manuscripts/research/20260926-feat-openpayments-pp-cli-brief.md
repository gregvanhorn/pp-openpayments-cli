# CMS Open Payments CLI Brief

## API Identity
- Domain: US federal transparency data (Physician Payments Sunshine Act). CMS publishes every payment/transfer of value from drug/device manufacturers and GPOs to physicians, non-physician practitioners and teaching hospitals: General, Research, and Ownership/Investment payments, program years 2019-2025 (refreshed each June, corrected each January).
- Users (concrete, from research):
  - **Medical-device / pharma field & medical-affairs teams** (MSLs, KOL managers, clinical-operations site scouts) mapping which physicians a competitor pays, and which sites already run sponsored trials (Apify actors "Pro + NPI" and physician-intelligence actors exist precisely for this; paid per row).
  - **Investigative / health journalists and researchers** (ProPublica Dollars for Docs lineage; propublica/d4dPartD-analysis, PublicI/medicaid-influence-analysis) ranking top recipients, tracking company spend, and joining payments to prescribing.
  - **Compliance / conflict-of-interest reviewers** (hospital COI offices, journal editors, alexandriashai/mcp-open-payments) checking a named doctor's industry relationships.
  - **Clinical-research business development** (the plan owner's persona): finding PIs and sites in a region (PA/NJ) by specialty (pain, spine, ortho, PM&R), and trials whose sponsors have no local investigator.
- Data profile: huge, append-mostly, yearly partitions. 2024 General = 15,498,687 rows (PA 670,866); 2024 Research = 817,215. 91 columns in General. All values strings; dates MM/DD/YYYY. Records corrected/deleted between refreshes (`change_type`).

## Reachability Risk
- Low. No auth, Akamai-fronted DKAN, CORS *, cache-control 602s. 15 back-to-back calls all 200 (competitor research) and all Phase 0 probes 200.
- Hard 500-row cap: limit=501 → HTTP 400 "JSON Schema validation failed" (must clamp client-side).
- Latency: count=true on 15.5M rows ≈ 6 s; `like '%Pfizer%'` ≈ 31 s; exact-match filters 1-2 s. → default count=false, generous timeouts, exact-match filters, local store for anything analytic.
- Politeness: robots.txt `Crawl-delay: 5` (crawler policy, not an API ToS). CLI default: 4 req/s ceiling, concurrency 4 (plan), adaptive backoff on 429/5xx; bulk CSV path for national pulls.
- No GitHub issues reporting 403/deprecation (wrappers too new). Historical break: Socrata→DKAN migration killed `/resource/*.json` URLs — design for identifier churn.

## Top Workflows
1. **Doctor dossier**: given an NPI or name, every payment by year × company × nature × product, with totals/trend and dispute status (COI reviewer, journalist).
2. **Company footprint**: who does Stryker pay, in which specialties/states/products, by year; concentration of spend (field teams, journalists).
3. **Regional research-site scouting**: research payments in PA/NJ by specialty with PI, study name, NCT ID, sponsor; all PIs for one NCT (clinical BD).
4. **Leaderboards**: top recipients / companies / products for a state × specialty × year (journalists).
5. **Change detection**: new recipients this year, rising/cooling relationships, what changed since last refresh (field teams, BD).

## Table Stakes
- Search by physician name/NPI, company, teaching hospital, state, specialty, nature of payment, year, amount (official site, Apify actors).
- Export JSON/CSV (Apify, official Dataset Explorer).
- Automatic yearly dataset-ID resolution (Pipeworx MCP).
- Product (drug/device) lookup across all 5 product slots (Pipeworx `product`).
- Research payments with PI + NCT (Pipeworx `research`).
- Top companies (Pipeworx, from pre-grouped summary).
- Cross-year history per recipient/company/product (Pipeworx, sampled — we do exact).

## Data Layer
- Primary entities: payments_general, payments_research (+ research_investigators child, nct_id), payments_ownership, recipients, teaching_hospitals, reporting_entities, products (payment↔product rows), dataset_registry, sync_state, trials (ClinicalTrials.gov cache), zip_centroids (bundled).
- Sync cursor: per scope (year×type×filter) offset + dataset `modified`; re-sync only when `modified` changes. Upsert on (record_id, program_year).
- FTS/search: FTS5 over recipient names, company names, product names, study names.
- Scoped sync (states / specialty / NPI / company) via DKAN conditions; `--full` streams bulk CSV.

## User Vision
- From the owner's master plan: "Print an agent-native CLI + MCP server for CMS Open Payments that can answer any question about industry payments to doctors and teaching hospitals, fast and offline." NOI: "Open Payments isn't just a transparency database. It's a map of who industry trusts." Required: every command in plan Section 5 (Layers 1-5), scoped sync, `sql` + `schema --json` escape hatch, `ask` helper, glossary, skill, ClinicalTrials.gov join (`trials sites|gaps|sponsor`). Acceptance: 20 questions answerable in ≤2 commands, <2 s from local data, matching live spot checks. Guardrails: read-only, polite, facts only (never label a payment improper), provenance on every row.

## Source Priority
- Primary: CMS Open Payments — official OpenAPI 3.0.2 (DKAN) — free, no auth.
- Secondary: ClinicalTrials.gov API v2 — official — free, no auth. Used only for the `trials` join (status, phase, leadSponsor, locations, conditions). Note: locations[].state is the full state name → map to USPS codes.
- Economics: both free; no keys anywhere.
- Inversion risk: none — Open Payments owns every headline command; ClinicalTrials.gov is scoped to the `trials` group.

## Product Thesis
- Name: openpayments-pp-cli / openpayments-pp-mcp ("CMS Open Payments")
- Why it should exist: The only maintained client in any language (no PyPI/npm client exists). Competing MCPs cannot aggregate over 15M-row years and return sampled cross-year totals; Apify charges per row and covers General only. A scoped local SQLite mirror with typed columns turns "map of who industry trusts" questions (dossiers, leaderboards, new/rising relationships, research-site gaps vs. ClinicalTrials.gov) into sub-second exact answers, offline, agent-native, with provenance.

## Build Priorities
1. Dataset registry + resolver (year×type → dataset_id + distribution_id + modified), DKAN query/sql wrappers with clamped pagination and backoff.
2. Typed SQLite store + scoped sync (states/specialty/NPI/company; bulk CSV for --full) + incremental by `modified` + change tracking.
3. Domain commands (physician dossier, company, top, research, investigators, hospital, ownership, nature, compare, near) and `sql`/`schema`/`ask`.
4. Insight commands (new-recipients, rising/cooling, relationships, research-sites, kol, concentration, product, changed).
5. ClinicalTrials.gov join (`trials sites|gaps|sponsor`) with local cache.

## Reachability Gate
- Decision: PASS
- Evidence: GET /api/1/metastore/schemas → 200; GET clinicaltrials.gov/api/v2/studies/NCT04280705 → 200 (2026-09-26)
