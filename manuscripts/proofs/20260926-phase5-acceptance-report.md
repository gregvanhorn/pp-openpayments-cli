Acceptance Report: openpayments
  Level: Full Dogfood (live CMS Open Payments + ClinicalTrials.gov; no auth; read-only)
  Tests: 225/225 passed (153 skipped as not applicable), runner verdict PASS; marker phase5-acceptance.json written by runner
  Loop 1: 210/223 — 13 failures
  Fixes applied: 11 (CLI fixes)
    - company/hospital/product/trials sponsor/ask: unknown input now exits 3 ("no match for …") instead of empty success
    - rising/cooling: missing year in scope → stderr hint + [] instead of exit 3 (still never reports false "stopped")
    - download/query dry-run JSON carries "action"
    - datastore sql --help gains Examples (spec x-pp-example added for regen)
    - metastore get-all: CMS returns 200 [] for unknown schema → exit 3 not found
  Loop 2: 224/225; Loop 3: 225/225
  Printing Press issues: 2
    - generated endpoint wrappers pass through 200-empty-array as success for unknown path ids (metastore get-all)
    - printJSONFiltered HTML-escapes & (&) in JSON output
  Gate: PASS
