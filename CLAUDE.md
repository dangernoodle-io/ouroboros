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
- `internal/cli/` — cobra root + CLI subcommands (query, items, put)
- `internal/app/` — MCP server setup, tool handlers
- `internal/store/` — SQLite schema, migrations, KB CRUD, FTS5 search
- `internal/backlog/` — backlog CRUD (projects, items, plans, config)
- `internal/backup/` — git backup operations
- `internal/config/` — bootstrap config file + env var loading
- `internal/kb/` — KB export/import, validation

## Tools

| Tool | Domain | Description |
|------|--------|-------------|
| put | KB | Create or update a knowledge entry (upserts by type+project+category+title) |
| get | KB | Get entries — by id for full content, or summaries with filters |
| delete | KB | Delete a knowledge entry by ID |
| search | KB | Full-text search across knowledge entries |
| export | KB | Export knowledge base to markdown |
| import | KB | Import knowledge entries from JSON |
| project | Backlog | Create a project (with name) or list all (no params) |
| item | Backlog | Create, get, update, or list backlog items (mode by inputs) |
| plan | Backlog | Create, get, update, or list implementation plans |
| config | Backlog | Get or set key-value configuration |

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
- `plugin/scripts/install.sh` — downloads the GitHub release archive, verifies SHA256, installs to plugin data dir
- `plugin/scripts/lib.js` — shared hook utilities (stdin, project resolution, cooldown, KB formatting)
- `plugin/scripts/*.js` — hook scripts for KB persistence nudges, context injection, staleness warnings
- `plugin/skills/` — persist, recall, triage skills
- `plugin/agents/` — backlog-manager, knowledge-explorer subagents
- `plugin/tests/` — node:test suite (zero npm deps), run via `plugin/tests/run.sh`

**No plugin version field**: `plugin/.claude-plugin/plugin.json` intentionally omits `version`. When absent, Claude Code keys its plugin cache on the source commit sha, so changing the `marketplace.json` ref to a new tag automatically invalidates the cache — no lockstep bump required. Release automation only needs to update the marketplace ref.

**Local dev**: from a clone of `dangernoodle-marketplace`, run `.scripts/plugin-dev.sh link ouroboros-mcp` to symlink the plugin cache dir to this working tree.
