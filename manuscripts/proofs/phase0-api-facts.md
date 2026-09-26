# Phase 0 — Open Payments API facts, re-verified live 2026-09-26

| Fact | Plan claim | Live result | Status |
|---|---|---|---|
| OpenAPI spec | GET /api/1 → OpenAPI 3.0.2 | `openapi: 3.0.2`, 22 paths, `servers: null`, `security: null` | ✅ (no servers block — base URL must be set) |
| Auth | none | all calls unauthenticated | ✅ |
| Catalog | 74 datasets | 74 | ✅ |
| Year families | 2019–2025 General/Research/Ownership | all 21 present, all `modified=2026-06-30` | ✅ |
| Row limit | 500 max | limit=500 → 500 rows; limit=1000 → HTTP error "JSON Schema validation failed" (rejected, not clamped) | ✅ + clamp client-side |
| Count only | results=false | returns count + schema (91 fields for general) | ✅ |
| Scale | 2024 general 15,498,687; PA 670,866; research 817,215 | 15,498,687 / 670,866 / 817,215 | ✅ exact |
| Server aggregation | sum + groupings + sorts | works; sums returned as strings ("180214208.7") | ✅ |
| SQL endpoint | needs distribution ID | dist id works; dataset id → 400 "distribution … not found" | ✅ |
| SQL keys | Title_Case | `Record_ID`, `Total_Amount_of_Payment_USDollars` | ✅ |
| Types | all strings | amounts, program_year all strings | ✅ |
| Dates | — | `date_of_payment` = `MM/DD/YYYY` (new finding) | ⚠ parse to ISO on ingest |
| change_type | exists | `UNCHANGED` observed | ✅ |
| Bulk CSV | downloadURL per distribution | `https://download.cms.gov/openpayments/PGYR2024_P06302026_06032026/OP_DTL_GNRL_PGYR2024_P06302026_06032026.csv` | ✅ |

Write endpoints present in spec and MUST be excluded: `/datastore/imports*`, `/harvest/*`, metastore POST/PUT/PATCH/DELETE.
