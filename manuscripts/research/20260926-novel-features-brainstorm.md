# Novel-features brainstorm (subagent output, 2026-09-26)

## Customer model
- Dana Whitfield — KOL manager, mid-size spine/ortho device company. Today: official profile search + paid Apify "Pro + NPI" runs + multi-GB yearly CSV downloads joined in pandas. Ritual: pre-territory-call map of PA/NJ spine surgeons/physiatrists by speaking+consulting dollars, paying companies, relationship tenure; post-June/January refresh change review. Frustration: YoY and cross-company views need 2+ multi-GB downloads + manual joins; live API capped at 500 rows, `like` ~31 s.
- Marcus Oyelaran — health data reporter (Dollars for Docs lineage). Today: inherited notebook over bulk CSVs, DKAN explorer spot checks, Pipeworx sampled totals he can't cite. Ritual: leaderboards by state×specialty×year, company concentration, new/fast-rising relationships with provenance. Frustration: no exact reproducible cross-year aggregates with provenance without rebuilding a DB every cycle; must stay factual.
- Priya Raman — hospital COI analyst. Today: one-name-at-a-time official profile search + screenshots vs disclosure forms. Ritual: weekly batch of 20-60 faculty: dollars per company per year, nature, dispute status. Frustration: no batch or side-by-side; name ambiguity → wrong-doctor matches.
- Rachel Kim — clinical-research BD, PA/NJ site network (plan owner persona). Today: filters Research CSVs by state+specialty, pastes NCT IDs into ClinicalTrials.gov one by one. Ritual: scout sites/PIs near ZIP 19002, all PIs on an NCT, recruiting trials whose sponsor pays no local PI. Frustration: nothing joins OP research PIs to CT.gov recruiting status/locations.

## Candidates (pre-cut)
C1 compare, C2 near, C3 new-recipients, C4 rising, C5 cooling, C6 relationships, C7 research-sites, C8 kol, C9 concentration, C10 changed, C11 investigators, C12 trials sites (absorbed), C13 trials gaps, C14 trials sponsor, C15 ask, C16 overlap, C17 roster, C18 dossier (absorbed), C19 peers, C20 flags, C21 partd, C22 disputes, C23 rank, C24 territory, C25 refresh-check, C26 glossary.
Inline kills: C20 violates facts-only guardrail; C21 external service not in brief; C19 speculative; C12/C18 already absorbed.

## Survivors and kills
Survivors (score): compare 8, near 7, new-recipients 9, rising 9, cooling 7, relationships 8, research-sites 9, kol 10, concentration 7, changed 7, investigators 8, trials gaps 8, trials sponsor 6, ask 8, overlap 8, roster 8 — all hand-code. Verifiability flag: trials gaps/sponsor rely on CT.gov leadSponsor ↔ OP manufacturer name matching; print matched pairs and accept --sponsor-alias; manual QA against 3 sponsors.

### Killed candidates
| Feature | Kill reason | Closest surviving sibling |
|---|---|---|
| trials sites (C12) | Already absorbed (row 25) | investigators |
| dossier (C18) | Already absorbed (rows 9-10) | relationships |
| peers (C19) | Speculative, no research evidence | overlap |
| flags (C20) | Violates facts-only guardrail | rising |
| partd (C21) | External service not in brief | dossier |
| disputes (C22) | Too narrow; dispute column in dossier, count in roster | roster |
| rank (C23) | Fold into dossier --peer-rank | compare |
| territory (C24) | Duplicates near --company | near |
| refresh-check (C25) | Covered by datasets list + incremental sync | changed |
| glossary (C26) | ask prints the glossary | ask |
