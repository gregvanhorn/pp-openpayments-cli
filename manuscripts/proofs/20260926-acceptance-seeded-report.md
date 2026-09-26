# Acceptance — seeded store (plan Section 8), 2026-09-26T17:36Z

Seed: general 2023–2025 PA+NJ (bulk CSV), research+ownership 2023–2025 PA+NJ (API), general+research 2019–2022 PA Pain Medicine (bulk CSV).
Store: 3,571,176 general / 127,203 research / 783 ownership rows; 120,104 recipients; ~4.2 GB.

Result: **20/20 PASS**; every local question under 2 s (test 16 is live ClinicalTrials.gov, timing reported not gated).

```
1    PASS  Top 20 PA recipients by general payments, 2024 — 699ms; #1 1497709075 local=1730183.06 live=1730183.06
2    PASS  Top 10 companies paying PA Pain Medicine doctors, 2019-2025 — 1190ms; 10 rows; years=2019-2025
3    PASS  Every NJ research payment with an NCT ID, 2024 — 386ms; 3833 rows (recipient or PI in NJ); recipient-NJ local=3744 live=3744
4    PASS  All PIs paid for NCT NCT05665088 — 58ms; 1 PIs
5    PASS  Full dossier for NPI 1497709075 — 56ms; 2024 local=1730277.17 live=1730277.1700000002
6    PASS  PA doctors new to industry payments in 2025 — 1566ms; 50 rows; prior years 2019,2020,2021,2022,2023,2024
7    PASS  Largest YoY increase company-doctor pair in PA — 1215ms; top: Abboud, Joseph Albert / Zimmer Biomet Holdings, Inc. +785066.57
8    PASS  Stryker to PA Orthopaedic Surgeons by year — 687ms; by_year=[{"y":2023,"t":5017846.79},{"y":2024,"t":5621079.47},{"y":2025,"t":4814739.19}]; all-specialty 2024 spot local=6701958.91 live=6701958.910000028
9    PASS  Nature-of-payment breakdown for NPI 1497709075 — 33ms; 4 natures
10   PASS  PA teaching hospitals ranked by research dollars — 103ms; #1 hospital of the univ of penna
11   PASS  NJ ownership interests grouped by company — 22ms; 54 companies; 2024 rows local=215 live=215
12   PASS  Doctors within 25 mi of 19002 with research payments — 365ms; 50 PIs; max miles 16.69
13   PASS  Products most linked to PA neurosurgeon payments — 1274ms; #1 The Tether
14   PASS  Doctors with research payments from 3+ sponsors — 886ms; 100 PIs; min sponsors 3
15   PASS  KOL ranking, PM&R in PA — 1139ms; #1 Smith, Jack Michael $264596.25 7 cos
16   PASS  Recruiting spine trials in PA/NJ with no local sponsor-paid PI — 2335ms (live CT.gov); 3 gap trials
17   PASS  Records changed since last sync — 55ms; run 20 counts=[]
18   PASS  Spend concentration for Stryker Corporation — 197ms; hhi=614.52 top10=54.4%
19   PASS  MCP tool call matches CLI (top) — cli=[{"npi":"1497709075","total":1730183.06},{"npi":"1497793657","total":1251229.99},{"npi":"1487626149","total":1193109.91},{"npi":"1851416606","total":924945.73},{"npi":"1972531424","total":874801.67}] mcp=[{"npi":"1497709075","total":1730183.06},{"npi":"1497793657","total":1251229.99},{"npi":"1487626149","total":1193109.91},{"npi":"1851416606","total":924945.73},{"npi":"1972531424","total":874801.67}]
20   PASS  Agent-written SQL from schema --json — 472ms for 2 commands; tables=payments_general,payments_research,research_investigators,payments_ownership,products,recipients,teaching_hospitals,reporting_entities,company_names,general_pairs,dataset_registry,sync_scopes,sync_runs,sync_changes,trials,trial_locations,zip_centroids; rows=[{"pis":1896,"program_year":2023,"total":292620186.41},{"pis":1880,"program_year":2024,"total":316212955.82},{"pis":1799,"program_year":2025,"total":309377199.42}]
PASSED 20 / 20
```

## Scale fixes made while seeding (first seeded run was 13/20)
- `research --state`, research specialty filter, `roster`: correlated `EXISTS` over research_investigators → uncorrelated `(record_id, program_year) IN (...)` (65 s → 0.07 s).
- `ix_gen_cover` covering index (state, program_year, npi, company, amount, specialties, nature, record_id, dataset_modified); recipient rollups (`top --by recipient`, `kol`) aggregate on covered columns and take name/specialty/city from `recipients`.
- `--company` resolves through the new `company_names` table to an indexed `company IN (...)`; state/year predicates are de-prioritised with unary `+` when a company filter is present (`company` 17 s → 0.7 s).
- Product joins use `CROSS JOIN` so payments drive product lookups (16 s → <1.4 s).
- Redundant `LOWER()` around `LIKE` removed (SQLite LIKE is already ASCII case-insensitive).
- New `general_pairs` rollup (state × year × npi × company); `rising`/`cooling` run one conditional-sum pass (9.8 s → 1.2 s; output verified identical to the raw-row path), `new-recipients` uses `HAVING MIN(program_year)=Y` (timeout → 1.5 s; verified against a brute-force recomputation, 875/875 NPIs).
- Test 5 now checks the dossier against the all-state live total (the dossier spans states; exact match 1,730,277.17).
