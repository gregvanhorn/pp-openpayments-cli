# Phase 4.85 — agentic output review (2026-09-26)

Status: WARN (7 warnings). Samples: scorecard --live-check (11 pass / 5 unsynced prerequisite) plus read-only commands against a partially seeded PA/NJ store.

| # | Check | Finding | Outcome |
|---|---|---|---|
| 1 | semantic | rising/cooling reported every pair "stopped"/"new" when the target or prior year had no rows in scope | fixed: both years must have rows in the filter scope, else exit 3 with a sync hint |
| 2 | format | specialty via MAX() picked alphabetical minority value (oncologist shown as Pediatrics) | fixed: recipients.specialty = most frequent (ties → latest year); grouped outputs with npi use it |
| 3 | format | JSON escapes & as & | not fixed: valid JSON from generator printJSONFiltered (json.Marshal); retro candidate for the Printing Press (SetEscapeHTML(false)) |
| 4 | format | "store not synced" hint on populated store (bulk/in-progress loads) | fixed: unsynced hint only when the domain table is empty; stale hint otherwise |
| 5 | format | null counts/lists on empty results (overlap, trials sponsor) | fixed: COALESCE to 0 / "" and [] |
| 6 | format | duplicate site names differing by case | fixed (case); punctuation variants remain as published by CMS |
| 7 | semantic | ask ranked product leaderboard first for "who pays"; example SQL lacked year filter | fixed: company example + keywords; SQL aligned |

Guardrail check: no improper/conflict/red-flag wording in outputs.
