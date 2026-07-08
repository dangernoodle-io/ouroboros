# ouroboros

MCP server for project knowledge base and backlog management. Persists decisions, facts, notes, and relations across conversations. Tracks work items, plans, and project configuration.

## Module

`dangernoodle.io/ouroboros`, Go 1.26.1

## Build

```bash
make build    # CGO_ENABLED=0 go build
make test     # go test ./...
make lint     # golangci-lint run
```

## Project layout

- `main.go` — thin wrapper, delegates to `internal/cli.Execute`
- `internal/cli/` — cobra root + CLI subcommands (query, kb, ls, roadmap)
- `internal/app/` — MCP server setup, tool handlers
- `internal/store/` — SQLite schema, migrations, KB CRUD, FTS5 search
- `internal/backlog/` — backlog CRUD (projects, items, plans, config)
- `internal/roadmap/` — roadmap CRUD (per-project singletons, item mutations)
- `internal/backup/` — git backup operations
- `internal/config/` — bootstrap config file + env var loading
- `internal/kb/` — KB export/import, validation

## Tools

MCP surface is intentionally narrow (5 tools); operator-style ops are CLI-only. The 5 tools follow a clean read/write axis: reads (`get`/`search`) are parameterized by `domain` (kb|backlog|roadmap); writes are concept-named (`kb` for knowledge entries, `backlog` for items, `roadmap` for per-project roadmap mutations). This design reduces tool mis-selection.

| Tool | Type | Description |
|------|------|-------------|
| get | Read | Fetch entries by ID or filters (requires `domain`: kb\|backlog\|roadmap) |
| search | Read | Full-text search (requires `domain`: kb\|backlog\|roadmap) |
| kb | Write | Create or update knowledge entries (upserts by type+project+category+title) |
| backlog | Write | Create, update, or delete backlog items |
| roadmap | Write | Mutate per-project roadmap (now/next/parked/done sections via op=add\|update\|move\|done\|remove) |

CLI-only ops (run `ouroboros <cmd> --help`): `project` (create/get/list/rename/delete), `plan` (create/get/list/update), `config` (get/set), `kb delete`, `export`, `import`, `roadmap` (show/add/update/move/done/remove). Browse with `ls items`, `ls kb`, `ls plans`, `ls projects`.

## Configuration

| Env var | Description |
|---------|-------------|
| PROJECT_KB_PATH | SQLite database path (primary) |
| QM_DB_PATH | SQLite database path (alias) |
| QM_BACKUP_MODE | none, dedicated, or shared |
| QM_GIT_REPO | Git repository path for backups |
| QM_SPARSE_PATH | Sparse checkout path (shared mode) |

## Storage

SQLite with WAL mode. Schema managed by versioned migrations. Tables: documents (KB), documents_fts (FTS5), projects, items, plans, config, schema_migrations.

Default DB path: `~/.local/share/ouroboros/kb.db`

## Dependencies

- `github.com/mark3labs/mcp-go` — MCP server framework
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
- `plugin/hooks/hooks.json` — hooks for SessionStart (install), PostToolUse, SubagentStart, SubagentStop, Stop, UserPromptSubmit
- `plugin/scripts/bootstrap.js` — single pure-Node SessionStart installer + validator (no npm deps): installs the binary (dev path, local Homebrew, or GitHub release archive, verified via SHA256), then checks the binary and every hook script are on disk, repairing the binary when missing/broken; fail-open, always exits 0
- `plugin/scripts/lib.js` — shared hook utilities (stdin, project resolution, cooldown, KB formatting)
- `plugin/scripts/*.js` — hook scripts for KB persistence nudges, context injection, staleness warnings
- `plugin/skills/` — persist, recall, triage skills
- `plugin/agents/` — backlog-manager, knowledge-explorer subagents
- `plugin/tests/` — node:test suite (zero npm deps), run via `plugin/tests/run.sh`

### Component convention

- Items scoped to the Go binary/library code: omit `component` (project-level).
- Items scoped to the plugin wrapper (`plugin/` directory): set `component: "plugin"`.
- Examples: an MCP handler bug = no component; a `stop.js` hook fix = `component: "plugin"`.

**No plugin version field**: `plugin/.claude-plugin/plugin.json` intentionally omits `version`. When absent, Claude Code keys its plugin cache on the source commit sha, so changing the `marketplace.json` ref to a new tag automatically invalidates the cache — no lockstep bump required. Release automation only needs to update the marketplace ref.

**Local dev**: from a clone of `dangernoodle-marketplace`, run `.scripts/plugin-dev.sh link ouroboros-mcp` to symlink the plugin cache dir to this working tree.
