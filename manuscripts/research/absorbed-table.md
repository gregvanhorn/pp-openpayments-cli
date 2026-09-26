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
