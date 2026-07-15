# ouroboros

MCP server for project knowledge base and backlog management. Persists decisions, facts, notes, and relations across conversations. Tracks work items, plans, and project configuration.

## Module

`dangernoodle.io/ouroboros`, Go 1.26.1

## Build

```bash
make build    # CGO_ENABLED=0 go build
make test     # go test ./...
make acc      # ACC_OUROBOROS=1 go test ./integration/...
make cover    # coverage profile + func summary
make lint     # golangci-lint run
```

`make acc` runs the MCP wire acceptance harness (`integration/mcp_acc_test.go`): spawns the built server over stdio and drives initialize→tools/list→tools/call against the 4 tools. Skipped by default (`internal/testutil.SkipUnlessAcc`) unless `ACC_OUROBOROS=1` is set.

## Project layout

- `main.go` — thin wrapper, delegates to `internal/cli.Execute`
- `internal/cli/` — cobra root + noun-first CLI subcommands (kb, backlog, edges, project, plan, config, roadmap, dashboard, export, import, claude) + server
- `internal/app/` — MCP server setup, tool handlers
- `internal/store/` — SQLite schema, migrations, KB CRUD, FTS5 search
- `internal/backlog/` — backlog CRUD (projects, items, plans, config)
- `internal/roadmap/` — roadmap CRUD (per-project singletons, item mutations)
- `internal/edges/` — polymorphic cross-reference graph (item/kb, blocks\|relates\|explains); `[[Title]]` KB autolinking; cascade cleanup on item/kb delete
- `internal/config/` — config loading via shesha `config.Load` + `xdgpath` (bootstrap.json overlay + `PROJECT_KB_PATH`/`QM_DB_PATH` env alias)
- `internal/kb/` — KB export/import, validation
- `internal/embed/` — pure-Go, offline static text embeddings (Model2Vec potion-retrieval-32M, int8-quantized, `//go:embed`'d asset); foundation for semantic search — not yet wired into the CLI/MCP surface
- `internal/dashboard/` — dashboard data-capture (builtin/exec/shell segment producers → NDJSON contract; gated by `dashboard.enabled`); exec/shell run per-segment with a `timeout` (default 5s, clamped [1s,30s]) — sh/jq are NOT required (exec is native argv, shell is opt-in `sh -c`), and producers are operator-configured local commands (trust boundary = a Makefile/git hook), not remote input
- `internal/testutil/` — `SkipUnlessAcc` acceptance-test gate
- `integration/` — MCP wire acceptance harness (ACC-gated, `make acc`)

## Tools

MCP surface is intentionally narrow (4 tools); operator-style ops are CLI-only. The 4 tools follow a clean read/write axis: `query` is the single read tool, parameterized by `domain` (kb|backlog|roadmap); writes are concept-named (`kb` for knowledge entries, `backlog` for items, `roadmap` for per-project roadmap mutations). This design reduces tool mis-selection.

| Tool | Type | Description |
|------|------|-------------|
| query | Read | Fetch/search entries (requires `domain`: kb\|backlog\|roadmap). `ids[]` = exact fetch; `query`/`queries[]` (kb batch) = full-text search; else a filtered list. roadmap supports `format=md\|html`; backlog: `epic`/`epics_only`, `since` (duration/date/RFC3339 creation-time cutoff), `sort=created` (newest-first) |
| kb | Write | Create or update (by id) knowledge entries (id absent: upserts by type+project+category+title) |
| backlog | Write | Create, update, or delete backlog items |
| roadmap | Write | Mutate per-project roadmap (now/next/deferred/parked/dropped/done sections; items carry two single-valued grouping axes, component + epic, plus optional position; via op=add\|update\|move\|reorder\|done\|remove) |

Cross-reference edges (`blocks`\|`relates`\|`explains`, item/kb endpoints) are not a 5th tool — they fold into the existing surface: `backlog` write entries[].edges[] (inline, primary path), `kb` write `[[Title]]` autolinks, `query` verbose=true edges sidecar. `edges list` is the CLI read; top-level `link`/`unlink` are aliases for `edges link`/`edges unlink` — both forms work. Epic membership stays a scalar field (`items.epic`), not an edge.

CLI-only ops (run `ouroboros <cmd> --help`): `project` (create/get/list/rename/delete), `plan` (create/get/list/update), `config` (get/set), `kb delete <id>...` (multiple ids, all-or-nothing), `export`, `import`, `roadmap` (show/add/update/move/reorder/done/remove/seed), `edges` (list/link/unlink; `link`/`unlink` also work as top-level aliases), `dashboard` (segment/view/project/status/refresh — views are named project sets, each refreshing to its own NDJSON output plus a self-contained `.html` page (theme-aware, meta-refresh at the view's cooldown); `project set --segments`/`--repo` set per-project segment/repo-path overrides, read-modify-write; a project's on-disk repo resolves via its `--repo` override else `dashboard.workspace_root`/`<project>`, else cwd auto-detect). Reads/writes are noun-first — see `ouroboros <noun> --help` or the wiki CLI page. Backlog write's `epic` field is validated (alias-aware): must resolve to an existing item, or the write errors and rolls back; clearing (empty) is always allowed. A batch write's `entries[]` share one transaction (any entry's failure rolls back the whole batch); `epic` may also be `"$N"`, a back-reference to the item created/updated by `entries[N]` earlier in the same batch, letting a child name its not-yet-created (server-assigned) epic parent or re-parent an existing item onto a new epic in one call.

## Configuration

| Env var | Description |
|---------|-------------|
| PROJECT_KB_PATH | SQLite database path (primary) |
| QM_DB_PATH | SQLite database path (alias) |

## Storage

SQLite with WAL mode. Schema managed by versioned migrations. Tables: documents (KB), documents_fts (FTS5), projects, items, plans, edges (cross-reference graph, app-level integrity — no FK across the type-erased endpoint columns), config, schema_migrations.

Default DB path: XDG data dir for `ouroboros` (`~/.local/share/ouroboros/kb.db`; override via `OUROBOROS_DATA_DIR` or `XDG_DATA_HOME`). bootstrap.json location is likewise the XDG config dir (`~/.config/ouroboros/bootstrap.json`; override via `OUROBOROS_CONFIG_DIR` or `XDG_CONFIG_HOME`).

## Dependencies

- `github.com/dangernoodle-io/shesha` (+ `shesha/cli`, `shesha/mcpx`, `shesha/host/generic`) — MCP server framework; the served ouroboros MCP server is the shesha-composed `buildServerV2` (`internal/app`), wired into `internal/cli` via `shesha/cli.ServerCmd`; bare `ouroboros` shows help, the MCP server runs via `ouroboros server` (stdio by default, `--http <addr>` optional, `--read-only` gate)
- `github.com/mark3labs/mcp-go` — test-only, the MCP wire-acceptance client (`integration/mcp_acc_test.go`); not imported by served code
- `modernc.org/sqlite` — pure Go SQLite driver (CGO_ENABLED=0 safe)

## Guiding principle: token conservation

ouroboros exists to replace ~14K tokens of unconditional project context loading with on-demand queryable retrieval. Every tool, output format, and default must honor that reason for existing. Concretely:

- **Compact by default.** List/search operations return ID + title + priority/tags only — never full content. Full content is fetched by explicit ID (`get id=...`, `item id=...`) and only when the caller has already decided it's needed.
- **Summaries have a hard ceiling.** Keep one-line summaries scannable; prefer a short title over a paragraph. Detailed context belongs in the body, fetched on demand.
- **Design changes must not bloat default output.** Any new field added to list responses is a cost multiplier across every call — justify it or put it behind an explicit flag.
- **Tool descriptions are context too.** MCP tool descriptions load on every session — keep them tight. One sentence of purpose, one sentence of mode-selection if the tool is overloaded.

When in doubt: the caller can always ask for more. They cannot un-spend tokens on output they didn't need.

## Plugin

`plugin/` contains the Claude Code plugin wrapper (`ouroboros-mcp`) — registers this binary as an MCP server.

- `plugin/.claude-plugin/plugin.json` — manifest; `mcpServers.ouroboros.command` points at `${CLAUDE_PLUGIN_DATA}/bin/ouroboros`
- `plugin/hooks/hooks.json` — hooks for SessionStart (install), PostToolUse, SubagentStart, SubagentStop, Stop, UserPromptSubmit, PreCompact
- `plugin/scripts/bootstrap.js` — single pure-Node SessionStart installer + validator (no npm deps): installs the binary (dev path, local Homebrew, or GitHub release archive, verified via SHA256), then checks the binary and every hook script are on disk, repairing the binary when missing/broken; fail-open, always exits 0
- All non-SessionStart hooks (KB persistence nudges, context injection, staleness warnings) are native Go handlers in the binary (`ouroboros claude hooks <event>`); `plugin/scripts/` contains only `bootstrap.js`
- `plugin/skills/` — persist, recall, triage, epic skills
- `plugin/agents/` — backlog-manager, knowledge-explorer subagents
- `plugin/tests/` — node:test suite (zero npm deps), run via `plugin/tests/run.sh`

### Component convention

- Items scoped to the Go binary/library code: omit `component` (project-level).
- Items scoped to the plugin wrapper (`plugin/` directory): set `component: "plugin"`.
- Examples: an MCP handler bug = no component; a `hooks.json`/`bootstrap.js` change = `component: "plugin"`; a Go hook-handler change (`internal/cli`) = no component (project-level).

**No plugin version field**: `plugin/.claude-plugin/plugin.json` intentionally omits `version`. When absent, Claude Code keys its plugin cache on the source commit sha, so changing the `marketplace.json` ref to a new tag automatically invalidates the cache — no lockstep bump required. Release automation only needs to update the marketplace ref.

**Local dev**: from a clone of `dangernoodle-marketplace`, run `.scripts/plugin-dev.sh link ouroboros-mcp` to symlink the plugin cache dir to this working tree.

**Upgrading**: after a marketplace ref bump / plugin pull, do a full restart (new session) so SessionStart runs `bootstrap.js` and installs the matching binary. `/reload-plugins` only reloads `hooks.json`, not the binary — it can transiently break hooks with `unknown command "claude"` until a restart. `bootstrap.js` self-heals at startup/resume: `checkBinary` probes the installed binary against the `claude <subcommand>` prefixes `hooks.json` references (derived from `hooks.json` itself, not hardcoded) and reinstalls when the binary lacks one (stale). `install()`'s local/dev-binary precedence is probed the same way so a stale local/dev source can't clobber a working release — an auto-discovered local binary that's stale falls through to the GitHub release; an explicit `OUROBOROS_DEV_BINARY` is honored even if stale but bootstrap warns loudly.
