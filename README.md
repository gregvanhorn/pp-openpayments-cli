# CMS Open Payments CLI

**A local, exact, offline map of who industry pays: every CMS Open Payments dataset, plus KOL rankings, new and rising relationships, and trial-site gaps no other tool computes.**

Sync the payments you care about (by state, specialty, NPI or company) into typed SQLite and answer dossier, leaderboard and year-over-year questions in under two seconds. Commands like kol, rising, research-sites and trials gaps join General, Research and Ownership data with ClinicalTrials.gov locally, with provenance on every row.

Learn more at [CMS Open Payments](https://openpaymentsdata.cms.gov).

Created by [@gregvanhorn](https://github.com/gregvanhorn) (Claude).

## Install

The recommended path installs both the `openpayments-pp-cli` binary and the `pp-openpayments` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press-library install openpayments
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press-library install openpayments --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press-library install openpayments --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press-library install openpayments --agent claude-code
npx -y @mvanhorn/printing-press-library install openpayments --agent claude-code --agent codex
```

### Without Node (Go fallback)

If `npx` isn't available (no Node, offline), install the CLI directly via Go (requires Go 1.26.6 or newer):

```bash
go install github.com/mvanhorn/printing-press-library/library/health/openpayments/cmd/openpayments-pp-cli@latest
```

This installs the CLI only — no skill.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/openpayments-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

Install the CLI binary first. The installer writes binaries to a per-user managed bin directory by default: `$HOME/.local/bin` on macOS/Linux and `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows.

```bash
npx -y @mvanhorn/printing-press-library install openpayments --cli-only
```

Then install the focused Hermes skill.

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-openpayments --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-openpayments --force
```

Restart the Hermes session or gateway if the newly installed skill is not visible immediately.

## Install for OpenClaw
Install both the CLI binary and the focused OpenClaw skill. The installer defaults binaries to a per-user bin directory (`$HOME/.local/bin` on macOS/Linux, `%LOCALAPPDATA%\Programs\PrintingPress\bin` on Windows):

```bash
npx -y @mvanhorn/printing-press-library install openpayments --agent openclaw
```

Restart the OpenClaw session or gateway if the newly installed skill is not visible immediately.

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/openpayments-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/health/openpayments/cmd/openpayments-pp-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "openpayments": {
      "command": "openpayments-pp-mcp"
    }
  }
}
```

</details>

## Authentication

No API key or account is needed. CMS Open Payments and ClinicalTrials.gov are public, unauthenticated APIs. The CLI paces itself (4 requests per second, 4 workers by default) to stay polite to CMS.

## Quick Start

```bash
# Confirm the CLI is installed; no network needed
openpayments-pp-cli doctor --dry-run


# See the current dataset IDs CMS publishes for a program year
openpayments-pp-cli datasets list --year 2024


# Pull a scoped slice into the local store (the plan's seed)
openpayments-pp-cli sync --years 2023-2025 --types general,research,ownership --states PA,NJ


# First leaderboard from local data
openpayments-pp-cli top --by recipient --state PA --year 2024 --limit 20


# Rank key opinion leaders by speaking and consulting dollars
openpayments-pp-cli kol --specialty "Physical Medicine" --state PA

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Relationship intelligence

- **`kol`** — Rank physicians in a specialty and region by speaking and consulting dollars, number of paying companies and years active.

  _Reach for this when asked who the key opinion leaders are in a specialty or territory._

  ```bash
  openpayments-pp-cli kol --specialty "Physical Medicine" --state PA --agent
  ```
- **`new-recipients`** — See which clinicians received industry payments this year for the first time.

  _Use when asked who is new to industry payments or which new relationships a company started._

  ```bash
  openpayments-pp-cli new-recipients --year 2025 --state PA --agent
  ```
- **`rising`** — Find recipient-company pairs whose dollars grew the most year over year.

  _Use for 'which relationships are growing' questions._

  ```bash
  openpayments-pp-cli rising --state PA --year 2025 --limit 20 --agent
  ```
- **`compare`** — Put two or more clinicians side by side by year, company and nature of payment.

  _Use for side-by-side COI or KOL comparisons._

  ```bash
  openpayments-pp-cli compare 1234567890 1987654321 --agent
  ```
- **`relationships`** — Show every company that paid a clinician with first year, last year, total and trend.

  _Use for how one clinician's industry relationships evolved._

  ```bash
  openpayments-pp-cli relationships 1234567890 --agent
  ```
- **`overlap`** — See which recipients two companies both pay, and who only one pays.

  _Use for competitive mapping between two manufacturers._

  ```bash
  openpayments-pp-cli overlap "Stryker Corporation" "Zimmer Biomet" --state PA --agent
  ```
- **`roster`** — Summarize payments for a whole list of NPIs in one pass, including disputed counts.

  _Use for conflict-of-interest reviews of many clinicians._

  ```bash
  openpayments-pp-cli roster --npi 1234567890,1987654321 --agent
  ```
- **`concentration`** — Measure how concentrated a company's spend is: top-10 share, HHI and recipients to reach 50% and 80%.

  _Use for questions about how concentrated a company's payments are._

  ```bash
  openpayments-pp-cli concentration "Stryker Corporation" --year 2024 --agent
  ```
- **`changed`** — See records added, corrected or removed since the previous sync.

  _Use after a CMS refresh to see what moved._

  ```bash
  openpayments-pp-cli changed --since-last-sync --agent
  ```
- **`cooling`** — Find recipient-company pairs whose dollars fell or stopped year over year.

  _Use for 'which relationships are cooling' questions._

  ```bash
  openpayments-pp-cli cooling --state PA --year 2025 --agent
  ```

### Trial-site intelligence

- **`research-sites`** — Rank sites and principal investigators by research dollars, trials and sponsors for a specialty and region.

  _Use when scouting proven trial sites or PIs in a region._

  ```bash
  openpayments-pp-cli research-sites --specialty "Pain Medicine" --state PA,NJ --agent
  ```
- **`trials gaps`** — List recruiting trials in your region whose sponsor pays no local principal investigator.

  _Use when looking for trials that still need local sites._

  ```bash
  openpayments-pp-cli trials gaps --condition "low back pain" --state PA,NJ --agent
  ```
- **`investigators`** — List every principal investigator paid for one ClinicalTrials.gov study, deduplicated.

  _Use when asked who was paid as PI on a specific trial._

  ```bash
  openpayments-pp-cli investigators --nct NCT04280705 --agent
  ```
- **`near`** — Find paid clinicians within N miles of a ZIP code.

  _Use for geographic questions around a ZIP._

  ```bash
  openpayments-pp-cli near --zip 19002 --miles 25 --type research --agent
  ```
- **`trials sponsor`** — Show a sponsor's ClinicalTrials.gov trials with the sites it already pays.

  _Use for one sponsor's trial portfolio._

  ```bash
  openpayments-pp-cli trials sponsor "Medtronic" --agent
  ```

### Agent plumbing

- **`ask`** — Print the local schema, a plain-word glossary and worked SQL examples for any question.

  _Use when no dedicated command fits and you need to write SQL._

  ```bash
  openpayments-pp-cli ask "top companies paying PA pain doctors"
  ```

## Recipes


### Top PA recipients

```bash
openpayments-pp-cli top --by recipient --state PA --year 2024 --limit 20 --agent --select name,npi,total
```

Top 20 PA recipients by total general payments, narrowed to three fields.

### Doctor dossier

```bash
openpayments-pp-cli dossier 1234567890 --agent
```

Every payment for one NPI by year, company, nature and product, with dispute status.

### PIs on one trial

```bash
openpayments-pp-cli investigators --nct NCT04280705 --agent
```

Every principal investigator Open Payments shows was paid for that study.

### Trial gaps

```bash
openpayments-pp-cli trials gaps --condition "low back pain" --state PA,NJ --agent
```

Recruiting trials in PA/NJ whose sponsor pays no local PI.

### Custom SQL

```bash
openpayments-pp-cli sql "SELECT company, SUM(amount) total FROM payments_general WHERE state='PA' GROUP BY company ORDER BY total DESC LIMIT 10" --agent
```

Write your own query after reading schema --json.

## Usage

Run `openpayments-pp-cli --help` for the full command reference and flag list.

## Paths & environment variables

This CLI separates local files into four path kinds:

| Kind | Contents |
|------|----------|
| `config` | User-editable settings such as `config.toml` and saved profiles |
| `data` | Durable local data such as `data.db` |
| `state` | Runtime state such as persisted queries, jobs, and `teach.log` |
| `cache` | Regenerable HTTP/cache files |

Each kind resolves independently. The ladder is:

1. Per-kind env var: `OPENPAYMENTS_CONFIG_DIR`, `OPENPAYMENTS_DATA_DIR`, `OPENPAYMENTS_STATE_DIR`, or `OPENPAYMENTS_CACHE_DIR`
2. `--home <dir>` for this invocation
3. `OPENPAYMENTS_HOME` for a flat relocated root
4. XDG env vars: `XDG_CONFIG_HOME`, `XDG_DATA_HOME`, `XDG_STATE_HOME`, `XDG_CACHE_HOME`
5. Platform defaults matching existing installs

For containers and agent sandboxes, prefer a single relocated root:

```bash
export OPENPAYMENTS_HOME=/srv/openpayments
openpayments-pp-cli doctor
```

Under `OPENPAYMENTS_HOME=/srv/openpayments`, the four dirs resolve to `/srv/openpayments/config`, `/srv/openpayments/data`, `/srv/openpayments/state`, and `/srv/openpayments/cache`.

MCP servers do not receive CLI flags from the host. Put relocation in the host `env` block:

```json
{
  "mcpServers": {
    "openpayments": {
      "command": "openpayments-pp-mcp",
      "env": {
        "OPENPAYMENTS_HOME": "/srv/openpayments"
      }
    }
  }
}
```

Precedence matters in fleets: an ambient per-kind variable such as `OPENPAYMENTS_DATA_DIR` overrides an explicit `--home` for that kind. Use `OPENPAYMENTS_HOME` or the per-kind variables for durable fleet relocation; treat `--home` as the weaker per-invocation lever.

Relocation is one-way. Unsetting `OPENPAYMENTS_HOME` does not move files back to platform defaults, and `doctor` cannot find files left under a former root. Move the files manually before unsetting relocation variables.

Existing installs keep working because the platform-default rung matches the legacy layout. Run `openpayments-pp-cli doctor --fail-on warn` to check path warnings in automation.

## Commands

### catalog

Manage catalog

- **`openpayments-pp-cli catalog search`** - Search the DKAN catalog
- **`openpayments-pp-cli catalog search-facets`** - Retrieve search facet information

### datastore

Manage datastore

- **`openpayments-pp-cli datastore datasetindex-query-get`** - Simple GET equivalent of a POST query -- see the POST endpoint documentation for full query schema. A few basic parameters are provided here as examples. For more reliable queries, write your query in JSON and then convert to a query string. See [this web tool](https://www.convertonline.io/convert/json-to-query-string) for an example.
- **`openpayments-pp-cli datastore resource-query-get`** - Simple GET equivalent of a POST query. Note that parameters containing arrays or objects are not yet supported by SwaggerUI. For conditions, sorts, and other complex parameters, write your query in JSON and then convert to a nested query string. See [this web tool](https://www.convertonline.io/convert/json-to-query-string) for an example.
- **`openpayments-pp-cli datastore sql`** - Interact with resources in the datastore using an SQL-like syntax.

### metastore

Work with metadata items.

- **`openpayments-pp-cli metastore dataset-get-item`** - Get a single dataset.
- **`openpayments-pp-cli metastore get-all`** - Get all items for a specific schema (e.g., "dataset")
- **`openpayments-pp-cli metastore get-schema`** - Get a specific schema
- **`openpayments-pp-cli metastore get-schemas`** - Get list of all schemas


### Self-learning loop

This CLI caches per-question discovery so repeat queries skip the walk and structurally similar queries get answered via entity substitution. The loop also self-captures: every invocation is journaled locally, and failed-flag corrections plus fresh teaches surface as candidates on the next `recall` for confirm/reject judgment. Agents call `recall` before discovery and fire `teach &` after answering. See the `## Automatic learning` section in `SKILL.md` for the full protocol.

- **`openpayments-pp-cli recall <query>`** - Look up cached resources for a query before running discovery
- **`openpayments-pp-cli teach`** - Record a query -> resource mapping (silent on success, safe to background with `&`)
- **`openpayments-pp-cli learnings list`** - Inspect taught rows
- **`openpayments-pp-cli learnings forget <query>`** - Undo a teach
- **`openpayments-pp-cli learnings candidates`** - List auto-captured candidates awaiting confirm/reject
- **`openpayments-pp-cli learnings stats`** - Local loop metrics: recall hit rate, teach-to-reuse, playbook resolution, candidate counts
- **`openpayments-pp-cli teach-pattern`** - Install a query/resource template up front
- **`openpayments-pp-cli teach-lookup`** - Add an entity mapping (e.g. country code, team alias) for pattern substitution

Pass `--no-learn` or set `OPENPAYMENTS_NO_LEARN=true` to disable the loop for deterministic flows.

The local store's schema version stamp is one-way: once this version of `openpayments-pp-cli` opens the database, older binaries refuse it with a version error — upgrade the binary rather than downgrading.

## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
openpayments-pp-cli catalog search

# JSON for scripting and agents
openpayments-pp-cli catalog search --json
# Filter to specific fields
openpayments-pp-cli catalog search --json --select name,total,type

# Dry run — show the request without sending
openpayments-pp-cli catalog search --dry-run

# Agent mode — JSON + compact + no prompts in one flag
openpayments-pp-cli catalog search --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select <field>[,<field>...]` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Read-only by default** - this CLI does not create, update, delete, publish, send, or mutate remote resources
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
openpayments-pp-cli doctor
```

Verifies configuration and connectivity to the API.

## Configuration

Run `openpayments-pp-cli doctor` to see the resolved config, data, state, and cache directories. The platform-default config path is `~/.config/cms-open-payments-pp-cli/config.toml`; `--home`, `OPENPAYMENTS_HOME`, and per-kind env vars can relocate it.

Static request headers can be configured under `headers`; per-command header overrides take precedence.

## Troubleshooting
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **A live query fails with 'JSON Schema validation failed'** — The DKAN API caps pages at 500 rows; use --limit 500 or less (the CLI paginates for you)
- **Local commands return nothing** — Run sync for the scope first, e.g. openpayments-pp-cli sync --years 2024 --states PA
- **Name or company filter is slow on live data** — Live wildcard filters take ~30 s on 15M-row years; sync the scope and query locally instead
- **Dataset ID not found after a CMS refresh** — Run openpayments-pp-cli datasets resolve --refresh; IDs change every June and January

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**mcp-sam-gov**](https://github.com/cliwant/mcp-sam-gov) — TypeScript (10 stars)
- [**openpayments-mcp-server**](https://github.com/QuentinCody/openpayments-mcp-server) — TypeScript (1 stars)
- [**mcp-open-payments**](https://github.com/alexandriashai/mcp-open-payments) — Python (1 stars)
- [**mcp-cms-open-payments (Pipeworx)**](https://github.com/pipeworx-io/mcp-cms-open-payments) — TypeScript

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
