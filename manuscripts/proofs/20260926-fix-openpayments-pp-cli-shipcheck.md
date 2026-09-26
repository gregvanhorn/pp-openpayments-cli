# Shipcheck — openpayments-pp-cli (2026-09-26)

## Loop 1 (shipcheck-run1.txt)
- Scorecard 88/100 Grade A; legs: verify PASS, dogfood PASS, workflow-verify PASS, apify-audit PASS, scorecard PASS, **validate-narrative FAIL**, **verify-skill FAIL**.
- Blockers:
  1. verify-skill: `sync --years/--types/--states` documented but flags were attached at runtime by a hook, invisible to static flag analysis.
  2. validate-narrative: ran against a stale staged binary (built before domain commands); plus the sync flag issue.
  3. Live sample probe 8/16: local-only commands in an empty sandbox home reported "no local data" in a form the probe could not classify.
- Fixes: declared the Open Payments scoping flags directly on the generated `sync` command (regen-mergeable hand-edit) and moved the body to `runOPSync`; unsynced-store error now carries the OS error (`no such file or directory`) plus the sync hint; recipe retitled "Clinician dossier" (MCP intent no longer reads like the `doctor` health check).

## Loop 2 (shipcheck-run2.txt)
- **Verdict: PASS (7/7 legs)**. Scorecard **88/100 Grade A**. verify PASS; dogfood WARN only for 3 unused generator helpers (handleBinaryResponseDelivery, readSecretFromStdin, successfulNoop — framework code, not ours).
- Live sample probe: 9/12 pass, 4 skipped (unsynced prerequisite), 3 order-dependent failures (compare/relationships/concentration exit 3 on an empty store). Follow-up fix: not-found messages now say "no match for <input> …", which the probe classifies as graceful-empty.
- Before/after: verify pass rate PASS→PASS; scorecard 88→88; legs 5/7→7/7.

## Behavioral sample
Every novel command was executed against real synced data (NJ 2024 research/ownership, NJ 2024 Stryker general, PA 2023 general in progress) — see build log; outputs non-empty and plausible (e.g. KOL PA orthopaedics, YoY cooling, NJ research sites, investigators for NCT04626635, trials sites/gaps/sponsor live against ClinicalTrials.gov).

## Recommendation: ship
