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

- `main.go` — MCP server entry point, KB handlers, CLI subcommands
- `tools.go` — MCP tool registration (KB + backlog)
- `handlers.go` — backlog handler implementations
- `internal/store/` — SQLite schema, migrations, KB CRUD, FTS5 search
- `internal/backlog/` — backlog CRUD (projects, items, plans, config)
- `internal/backup/` — git backup operations
- `internal/config/` — bootstrap config file + env var loading
- `internal/kb/` — KB export/import

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
