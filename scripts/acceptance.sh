#!/usr/bin/env bash
# Acceptance tests from the master plan (Section 8). Seed first:
#   openpayments-pp-cli sync --years 2023-2025 --types general,research,ownership --states PA,NJ
#   openpayments-pp-cli sync --years 2019-2022 --types general,research --states PA --specialty "Pain Medicine"
# Each question must be answered by <=2 commands, in <2 s from local data,
# and (where marked) match a live-API spot check.
set -uo pipefail
B=${B:-openpayments-pp-cli}
MCP=${MCP:-openpayments-pp-mcp}
pass=0; fail=0; results=()
now() { date +%s%N; }
ms() { echo $(( ($2 - $1) / 1000000 )); }
record() { # id desc status detail
  results+=("$(jq -nc --arg id "$1" --arg d "$2" --arg s "$3" --arg x "$4" '{test:$id,question:$d,status:$s,detail:$x}')")
  if [ "$3" = PASS ]; then pass=$((pass+1)); else fail=$((fail+1)); fi
  printf '%-4s %-5s %s — %s\n' "$1" "$3" "$2" "$4"
}
# run ID DESC LOCAL(1|0) -- cmd...   ; sets OUT and ELAPSED
run() {
  local id=$1 desc=$2 local=$3; shift 4
  local t0 t1; t0=$(now); OUT=$("$@" --json 2>/dev/null); local rc=$?; t1=$(now); ELAPSED=$(ms $t0 $t1)
  if [ $rc -ne 0 ]; then record "$id" "$desc" FAIL "exit $rc"; return 1; fi
  local n; n=$(echo "$OUT" | jq 'if type=="array" then length elif type=="object" then (.results // .trials // .relationships // .top // .changes // .by_year // [1] | length) else 0 end' 2>/dev/null || echo 0)
  if [ "$local" = 1 ] && [ "$ELAPSED" -ge 2000 ]; then record "$id" "$desc" FAIL "${ELAPSED}ms >= 2000ms"; return 1; fi
  N=$n; return 0
}
live_sum() { # year where... -> total
  local y=$1; shift; local args=(); for w in "$@"; do args+=(--where "$w"); done
  $B query --year "$y" "${args[@]}" --group-by program_year --sum total_amount_of_payment_usdollars --json 2>/dev/null | jq -r '.[0].total // 0'
}
close() { awk -v a="$1" -v b="$2" 'BEGIN{d=a-b; if(d<0)d=-d; exit !(d < 0.01 + 0.0001*(a<0?-a:a))}'; }

# 1
if run 1 "Top 20 PA recipients by general payments, 2024" 1 -- $B top --by recipient --state PA --year 2024 --limit 20; then
  NPI=$(echo "$OUT" | jq -r '.[0].npi'); LOC=$(echo "$OUT" | jq -r '.[0].total')
  LIVE=$(live_sum 2024 recipient_state=PA covered_recipient_npi=$NPI)
  if close "$LOC" "$LIVE"; then record 1 "Top 20 PA recipients by general payments, 2024" PASS "${ELAPSED}ms; #1 $NPI local=$LOC live=$LIVE"; else record 1 "Top 20 PA recipients" FAIL "spot mismatch local=$LOC live=$LIVE"; fi
fi
# 2
run 2 "Top 10 companies paying PA Pain Medicine doctors, 2019-2025" 1 -- $B top --by company --state PA --specialty "Pain Medicine" --year 2019-2025 --limit 10 && \
  record 2 "Top 10 companies paying PA Pain Medicine doctors, 2019-2025" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; $N rows; years=$(echo "$OUT" | jq -r '[.[].first_year]|min')-$(echo "$OUT" | jq -r '[.[].last_year]|max')"
# 3
if run 3 "Every NJ research payment with an NCT ID, 2024" 1 -- $B research --state NJ --year 2024 --has-nct --limit 100000; then
  LOCN=$($B sql "SELECT COUNT(*) n FROM payments_research WHERE state='NJ' AND program_year=2024 AND nct_id IS NOT NULL" --json | jq '.[0].n')
  LIVEN=$($B query --year 2024 --type research --where recipient_state=NJ --where "clinicaltrials_gov_identifier!=" --limit 1 --count --json 2>/dev/null | jq '.count')
  record 3 "Every NJ research payment with an NCT ID, 2024" $([ "$LOCN" = "$LIVEN" ] && echo PASS || echo FAIL) "${ELAPSED}ms; $N rows (recipient or PI in NJ); recipient-NJ local=$LOCN live=$LIVEN"
  NCT=$(echo "$OUT" | jq -r '[.[] | select(.nct_id != null)][0].nct_id')
fi
# 4
run 4 "All PIs paid for NCT $NCT" 1 -- $B investigators --nct "$NCT" && record 4 "All PIs paid for NCT $NCT" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; $N PIs"
# 5
if run 5 "Full dossier for NPI $NPI" 1 -- $B dossier "$NPI"; then
  LOC=$(echo "$OUT" | jq -r '[.by_year[] | select(.program_year==2024) | .total][0] // 0')
  record 5 "Full dossier for NPI $NPI" $(close "$LOC" "$LIVE" && echo PASS || echo FAIL) "${ELAPSED}ms; 2024 local=$LOC live=$LIVE"
fi
# 6
run 6 "PA doctors new to industry payments in 2025" 1 -- $B new-recipients --year 2025 --state PA --limit 50 && record 6 "PA doctors new to industry payments in 2025" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; $N rows; prior years $(echo "$OUT" | jq -r '.[0].prior_years_checked')"
# 7
run 7 "Largest YoY increase company-doctor pair in PA" 1 -- $B rising --state PA --limit 20 && record 7 "Largest YoY increase company-doctor pair in PA" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; top: $(echo "$OUT" | jq -r '.[0] | "\(.name) / \(.company) +\(.delta)"')"
# 8
if run 8 "Stryker to PA Orthopaedic Surgeons by year" 1 -- $B company "Stryker Corporation" --state PA --specialty "Orthopaedic Surgery"; then
  LOC=$($B sql "SELECT ROUND(SUM(amount),2) t FROM payments_general WHERE company='Stryker Corporation' AND state='PA' AND program_year=2024" --json | jq '.[0].t')
  LIVE8=$(live_sum 2024 recipient_state=PA "applicable_manufacturer_or_applicable_gpo_making_payment_name=Stryker Corporation")
  record 8 "Stryker to PA Orthopaedic Surgeons by year" $(close "$LOC" "$LIVE8" && echo PASS || echo FAIL) "${ELAPSED}ms; by_year=$(echo "$OUT" | jq -c '[.by_year[]|{y:.program_year,t:.total}]'); all-specialty 2024 spot local=$LOC live=$LIVE8"
fi
# 9
run 9 "Nature-of-payment breakdown for NPI $NPI" 1 -- $B nature --npi "$NPI" && record 9 "Nature-of-payment breakdown for NPI $NPI" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; $N natures"
# 10
run 10 "PA teaching hospitals ranked by research dollars" 1 -- $B hospital --state PA --metric research --limit 20 && record 10 "PA teaching hospitals ranked by research dollars" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; #1 $(echo "$OUT" | jq -r '.[0].hospital')"
# 11
if run 11 "NJ ownership interests grouped by company" 1 -- $B ownership --state NJ --group-by company --limit 500; then
  LOCN=$($B sql "SELECT COUNT(*) n FROM payments_ownership WHERE state='NJ' AND program_year=2024" --json | jq '.[0].n')
  LIVEN=$($B query --year 2024 --type ownership --where recipient_state=NJ --limit 1 --count --json 2>/dev/null | jq '.count')
  record 11 "NJ ownership interests grouped by company" $([ "$LOCN" = "$LIVEN" ] && echo PASS || echo FAIL) "${ELAPSED}ms; $N companies; 2024 rows local=$LOCN live=$LIVEN"
fi
# 12
run 12 "Doctors within 25 mi of 19002 with research payments" 1 -- $B near --zip 19002 --miles 25 --type research && record 12 "Doctors within 25 mi of 19002 with research payments" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; $N PIs; max miles $(echo "$OUT" | jq '[.[].miles]|max')"
# 13
run 13 "Products most linked to PA neurosurgeon payments" 1 -- $B top --by product --state PA --specialty "Neurological Surgery" && record 13 "Products most linked to PA neurosurgeon payments" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; #1 $(echo "$OUT" | jq -r '.[0].product')"
# 14
run 14 "Doctors with research payments from 3+ sponsors" 1 -- $B research-sites --min-sponsors 3 --limit 100 && record 14 "Doctors with research payments from 3+ sponsors" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; $N PIs; min sponsors $(echo "$OUT" | jq '[.[].sponsors]|min')"
# 15
run 15 "KOL ranking, PM&R in PA" 1 -- $B kol --specialty "Physical Medicine" --state PA && record 15 "KOL ranking, PM&R in PA" $([ "$N" -gt 0 ] && echo PASS || echo FAIL) "${ELAPSED}ms; #1 $(echo "$OUT" | jq -r '.[0] | "\(.name) $\(.speaking_consulting_total) \(.companies) cos"')"
# 16 (live ClinicalTrials.gov; timing is network-bound and reported, not gated)
run 16 "Recruiting spine trials in PA/NJ with no local sponsor-paid PI" 0 -- $B trials gaps --condition spine --state PA,NJ && record 16 "Recruiting spine trials in PA/NJ with no local sponsor-paid PI" PASS "${ELAPSED}ms (live CT.gov); $N gap trials"
# 17
run 17 "Records changed since last sync" 1 -- $B changed --since-last-sync && record 17 "Records changed since last sync" PASS "${ELAPSED}ms; run $(echo "$OUT" | jq -r '.sync_run') counts=$(echo "$OUT" | jq -c '.counts')"
# 18
run 18 "Spend concentration for Stryker Corporation" 1 -- $B concentration "Stryker Corporation" && record 18 "Spend concentration for Stryker Corporation" PASS "${ELAPSED}ms; hhi=$(echo "$OUT" | jq '.hhi') top10=$(echo "$OUT" | jq '.top10_share_pct')%"
# 19
CLI=$($B top --by recipient --state PA --year 2024 --limit 5 --json 2>/dev/null | jq -c '[.[]|{npi,total}]')
MCPOUT=$(printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"acceptance","version":"1"}}}' '{"jsonrpc":"2.0","method":"notifications/initialized"}' '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"top","arguments":{"by":"recipient","state":"PA","year":"2024","limit":5}}}' | timeout 60 $MCP 2>/dev/null | tail -1 | jq -r '.result.content[0].text' | jq -c '[.[]|{npi,total}]')
record 19 "MCP tool call matches CLI (top)" $([ -n "$CLI" ] && [ "$CLI" = "$MCPOUT" ] && echo PASS || echo FAIL) "cli=$CLI mcp=$MCPOUT"
# 20
T0=$(now); TABLES=$($B schema --json | jq -r '[.[].name]|join(",")'); OUT=$($B sql "SELECT p.program_year, COUNT(DISTINCT i.npi) pis, ROUND(SUM(p.amount),2) total FROM payments_research p JOIN research_investigators i USING(record_id, program_year) WHERE i.state IN ('PA','NJ') GROUP BY 1 ORDER BY 1" --json); T1=$(now)
record 20 "Agent-written SQL from schema --json" $([ "$(echo "$OUT" | jq length)" -gt 0 ] && echo PASS || echo FAIL) "$(ms $T0 $T1)ms for 2 commands; tables=$TABLES; rows=$(echo "$OUT" | jq -c .)"

printf '%s\n' "${results[@]}" | jq -s --arg p $pass --arg f $fail '{passed:($p|tonumber), failed:($f|tonumber), results:.}' > "${ACCEPTANCE_OUT:-acceptance-results.json}"
echo "PASSED $pass / $((pass+fail))"
[ $fail -eq 0 ]
