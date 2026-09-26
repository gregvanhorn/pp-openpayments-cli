# CMS Open Payments CLI — Absorb Manifest

Approval: owner's master plan Section 3 delegates approval to 'manifest includes every item in Section 5'; verified item-by-item below. Owner instructed 'proceed with everything'.

### Absorbed (match or beat everything that exists)
| # | Feature | Best Source | Our Implementation | Added Value |
|---|---------|-----------|-------------------|-------------|
| 1 | List all datasets in catalog | Official Dataset Explorer; DKAN metastore | openpayments-pp-cli datasets list | Typed rows (year, type, dataset_id, distribution_id, modified, rows), cached registry, --json/--csv |
| 2 | Auto-resolve current yearly dataset IDs | Pipeworx MCP (auto-discovery) | openpayments-pp-cli datasets resolve | Resolves dataset AND distribution ID by title pattern, cached in dataset_registry, refreshed when `modified` changes |
| 3 | Raw datastore query (conditions/properties/sorts/limit/offset) | DKAN API; QuentinCody MCP code mode | openpayments-pp-cli query | --where k=v / k>v flags → DKAN conditions, --year/--type instead of UUIDs, auto-paginate past 500, clamp limit |
| 4 | Server-side aggregation (sum + groupings) | DKAN API | (behavior in openpayments-pp-cli query) --sum/--group-by/--sort build DKAN expressions | Exact live totals without downloading rows |
| 5 | DKAN bracket SQL | DKAN /datastore/sql; cliwant cms_query_dataset | (behavior in openpayments-pp-cli sql) `[SELECT ... FROM general:2024]` auto-resolves distribution ID, Title_Case keys normalized to snake_case | No UUID hunting; consistent keys |
| 6 | Catalog search + facets | DKAN /search, /search/facets | (generated endpoint) catalog search | Typed, --json, --select |
| 7 | Bulk CSV download by year/type | Official downloads; Apify export | openpayments-pp-cli download | Resolves downloadURL from metastore; streams to disk; --dry-run shows URL |
| 8 | Metastore schema/items browse | DKAN metastore | (generated endpoint) metastore get-all | Raw access kept for agents |
| 9 | Search physician by name/NPI | Official profile search; Apify actors; Pipeworx physician | openpayments-pp-cli dossier | Fuzzy FTS5 name match, not exact-only; NPI or profile_id |
| 10 | Physician payment history by year | Pipeworx recipient history (sampled) | (behavior in openpayments-pp-cli dossier) by-year × company × nature × product | Exact totals from local store, trend, dispute status |
| 11 | Company payments / top recipients | Pipeworx company; official company profile | openpayments-pp-cli company | Top recipients, specialties, states, products by year |
| 12 | Top companies | Pipeworx top_companies (offline summary) | openpayments-pp-cli top | --by recipient|company|product with state/specialty/year filters |
| 13 | Research payments with PI + NCT | Pipeworx research | openpayments-pp-cli research | PI 1-5 flattened, NCT filter, state/specialty filters |
| 14 | Product (drug/device) lookup, all 5 slots | Pipeworx product | openpayments-pp-cli product | Normalized products table; category filter |
| 15 | Teaching hospital payments | Official hospital profile; ProPublica | openpayments-pp-cli hospital | CCN or name; ranks by research dollars |
| 16 | Ownership/investment interests | Official Ownership dataset | openpayments-pp-cli ownership | Grouped by company/state |
| 17 | Filter by nature of payment | Apify actors; official | openpayments-pp-cli nature | Breakdown with totals and counts |
| 18 | Filter by state / specialty / amount / year | Apify actors | (behavior in openpayments-pp-cli top) shared --state/--specialty/--year/--min-amount filters on every domain command | Consistent flags across commands |
| 19 | Export JSON/CSV/Excel | Apify actors; Dataset Explorer | (behavior in openpayments-pp-cli query) --json/--csv/--select on every command + framework export | Pipe-friendly, typed exit codes |
| 20 | State totals | ProPublica; official state summary dataset | (behavior in openpayments-pp-cli top) --by state | Uses local store or pre-grouped summary datasets |
| 21 | Offline SQL over staged data | QuentinCody MCP (SQLite staging) | openpayments-pp-cli sql | Real typed tables + schema --json, not JSON blobs |
| 22 | Cross-year history per company/product | Pipeworx history tools (sampled) | (behavior in openpayments-pp-cli company) --years 2019-2025 | Exact from local store |
| 23 | Scoped local sync | none (competitors are live-only) | openpayments-pp-cli sync | --years/--types/--states/--specialty/--npi/--company, parallel paging, backoff, bulk CSV for --full, incremental by modified |
| 24 | Local schema for agents | none | openpayments-pp-cli schema | schema --json exposes every table/column for agent SQL |
| 25 | Clinical trial lookup | clinical-trials-pp-cli (printing-press library) | openpayments-pp-cli trials sites | Joined with Open Payments PIs on nct_id |

### Transcendence (only possible with our approach)
| # | Feature | Command | Score | Buildability | How It Works | Evidence | Long Description |
|---|---------|---------|-------|--------------|--------------|----------|------------------|
| 1 | Recipient compare | compare <npi> <npi> | 8/10 | hand-code | Local payments grouped by recipient×year×company×nature pivoted per NPI | Top Workflow 1; COI persona; plan Layer 3 | Use this command to put 2+ specific recipients side by side. Do NOT use it for one recipient's full history; use 'dossier' instead. Do NOT use it for a long NPI list; use 'roster' instead. |
| 2 | Radius search | near --zip 19002 --miles 25 | 7/10 | hand-code | Haversine between bundled zip_centroids and recipient ZIP5, sum per recipient | BD persona; plan Layer 3 | Use this command for radius queries around a ZIP. Do NOT use it for whole-state leaderboards; use 'top' instead. Do NOT use it for ranking research sites by specialty and state; use 'research-sites' instead. |
| 3 | New recipients | new-recipients --year 2025 | 9/10 | hand-code | Recipients with payments in year N and none in any prior synced year | Top Workflow 5; plan Layer 4 | Use this command for first-time relationships in a year. Do NOT use it for existing relationships that grew; use 'rising' instead. |
| 4 | Rising relationships | rising | 9/10 | hand-code | recipient×company sums N vs N-1, delta and pct | Top Workflow 5; Pipeworx sampled | Use this command for relationships whose dollars grew year over year. Do NOT use it for brand-new relationships; use 'new-recipients' instead. Do NOT use it for declines; use 'cooling' instead. |
| 5 | Cooling relationships | cooling | 7/10 | hand-code | Same YoY core, most negative delta incl. drop to $0 | plan Layer 4 | Use this command for relationships whose dollars fell or stopped. Do NOT use it for growth; use 'rising' instead. |
| 6 | Relationship timeline | relationships <npi> | 8/10 | hand-code | Per company: first/last year, years active, dollars per year, natures | NOI; plan Layer 4 | Use this command for how one recipient's company relationships evolved over time. Do NOT use it for the full payment dossier with products and disputes; use 'dossier' instead. Do NOT use it for comparing recipients; use 'compare' instead. |
| 7 | Research-site scouting | research-sites --specialty --state | 9/10 | hand-code | research + research_investigators + teaching_hospitals grouped by site/PI | Top Workflow 3; plan Layer 4 | Use this command to rank sites and PIs for a specialty and region. Do NOT use it for the PIs of one known trial; use 'investigators' instead. Do NOT use it for raw research payment rows; use 'research' instead. |
| 8 | KOL ranking | kol --specialty --state | 10/10 | hand-code | speaking+consulting dollars, distinct companies, distinct years (all columns shown) | Apify physician-intelligence actors; plan Layer 4 | Use this command to rank physicians by speaking and consulting activity. Do NOT use it for all-nature dollar leaderboards; use 'top' instead. |
| 9 | Spend concentration | concentration <company> | 7/10 | hand-code | top-10 share, HHI, recipients to reach 50%/80% | Top Workflow 2; plan Layer 4 | Use this command to measure how concentrated one company's spend is. Do NOT use it to list who a company pays; use 'company' instead. |
| 10 | Change detection | changed --since-last-sync | 7/10 | hand-code | sync_changes log (added/amended/deleted) + CMS change_type | Top Workflow 5; plan Layer 4 | none |
| 11 | Trial investigators | investigators --nct NCT… | 8/10 | hand-code | research_investigators for one nct_id deduped across PI 1-5 | Top Workflow 3; plan Layer 3 | Use this command to list who Open Payments shows was paid as PI on one trial. Do NOT use it for where the trial runs per ClinicalTrials.gov; use 'trials sites' instead. Do NOT use it for raw payment rows; use 'research' instead. |
| 12 | Trial gaps | trials gaps --condition --state | 8/10 | hand-code | CT.gov recruiting trials in region minus sponsors paying an in-region PI | BD persona; plan Layer 5 | Use this command to find recruiting trials that lack a paid local PI. Do NOT use it for ranking existing sites; use 'research-sites' instead. Do NOT use it for one trial's locations; use 'trials sites' instead. |
| 13 | Sponsor trial map | trials sponsor <company> | 6/10 | hand-code | CT.gov leadSponsor trials joined on nct_id to local research | plan Layer 5 | Use this command for one sponsor's trial portfolio with paid PIs. Do NOT use it for a company's general-payment footprint; use 'company' instead. Do NOT use it for trials missing local PIs; use 'trials gaps' instead. |
| 14 | Agent question helper | ask "<question>" | 8/10 | hand-code | Local schema + row counts + glossary + worked SQL examples ranked by keyword overlap; no LLM | plan Section 6 | Use this command when no dedicated command fits and you need schema and examples to write SQL. Do NOT use it to run a query; use 'sql' instead. |
| 15 | Company overlap | overlap <company> <company> | 8/10 | hand-code | Recipient set intersection/difference across two companies with dollars | field-team persona; Top Workflow 2 | Use this command to compare two companies' recipient sets. Do NOT use it for one company's footprint; use 'company' instead. Do NOT use it to compare doctors; use 'compare' instead. |
| 16 | Batch roster check | roster --file npis.txt | 8/10 | hand-code | One row per NPI: totals per year, top companies, natures, disputed count | COI persona | Use this command for a list of many NPIs at once. Do NOT use it for one recipient in depth; use 'dossier' instead. Do NOT use it for a small side-by-side; use 'compare' instead. |

Stubs: none.

### Plan Section 5 coverage check
- Layer 1: datasets list/resolve (A1-2), query (A3-4), sql (A5, A21), catalog search (A6; plan `search` renamed — framework reserved), download (A7), agent flags (framework globals).
- Layer 2: sync (A23), schema (A24), typed tables incl. products/research_investigators/dataset_registry/sync_state.
- Layer 3: dossier (plan `doctor`, renamed — framework reserved) (A9-10), company (A11), top (A12), research (A13), investigators (T11), hospital (A15), ownership (A16), nature (A17), compare (T1), near (T2).
- Layer 4: new-recipients (T3), rising/cooling (T4-5), relationships (T6), research-sites (T7), kol (T8), concentration (T9), product (A14), changed (T10).
- Layer 5: trials sites (A25), trials gaps (T12), trials sponsor (T13).
- Section 6: sql + schema --json, ask (T14), glossary.md, /pp-openpayments skill.

Note: validate-narrative --framework-only flags quickstart[2] (sync --years/--types/--states) because the plan requires domain-scoped flags hand-added to the framework sync command. Full-example validation against the built binary in shipcheck is the authoritative check.
