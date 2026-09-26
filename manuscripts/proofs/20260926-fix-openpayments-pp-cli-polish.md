# Polish (Phase 5.5) — openpayments-pp-cli

Scorecard 89 → 93; verify 100% → 100%; live-check 11/1 → 12/0; gosec (hand-written) 12 → 0; tools-audit 2 → 0 pending; dogfood PASS → PASS.

Fixes applied:
- trials sponsor: misplaced COALESCE produced "SQL logic error near FROM" whenever a trial matched → const sponsorTrialPaymentsSQL + regression test TestSponsorTrialPaymentsSQLParses
- 12 gosec findings in hand-written code (explicit Close/Remove discards, filepath.Clean on user paths, documented #nosec G201 on static-schema upsert)
- removed dead generator helpers (handleBinaryResponseDelivery, readSecretFromStdin, successfulNoop + 4 deliver.go helpers) — Dead Code 2/5 → 5/5
- root Short/Long: "CMS Open Payments CLI"
- 17 hand-written command Shorts start with a verb (MCP tool catalog)
- 2 thin-short tools-audit findings in generated files accepted with rationale

Skipped (retro candidates): 31 gosec findings in generator-emitted files; MCP Tool Design 7/10 needs spec mcp.intents + regen; Cache Freshness 5/10 structural.

ship_recommendation: ship
