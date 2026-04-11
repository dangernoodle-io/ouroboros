# ouroboros

MCP server for persistent project knowledge base and backlog management. Stores decisions, facts, notes, and relations across conversations. Tracks work items, implementation plans, and project configuration in SQLite.

## Installation

Build requires Go 1.26.1:

```bash
make build
```

Register with Claude Code:

```bash
claude mcp add --scope user ouroboros /absolute/path/to/ouroboros
```

## Quick start

The server exposes 10 tools: 6 for knowledge base, 4 for backlog management.

### Knowledge base (KB)

- **put** — Create or update a document (upserts by type+project+category+title)
- **get** — Retrieve by ID (full content) or list with filters (summaries)
- **search** — Full-text search across all entries
- **delete** — Remove a document by ID
- **export** — Export KB to markdown (filters by project/type)
- **import** — Import knowledge entries from JSON

### Backlog

- **project** — Create projects or list all (auto-derives prefix from name)
- **item** — Manage backlog items (create, get, update, list by status/priority)
- **plan** — Create implementation plans, link to projects/items
- **config** — Key-value settings storage

## Tool reference

### Knowledge base tools

#### put
Create or update a knowledge entry. Upserts by `type+project+category+title` to avoid duplicates.

```
put(type, project, title, content?, category?, metadata?, tags?)
```

- **type** — Entry type: `decision`, `fact`, `note`, `relation`, etc.
- **project** — Project name (auto-derived from `git rev-parse --show-toplevel`)
- **title** — Summary (for decisions) or key (for facts)
- **content** — Body text, rationale, implementation details
- **category** — Grouping within a project
- **metadata** — JSON string of additional attributes
- **tags** — Array of string tags

#### get
Retrieve entries. By ID returns full content; with filters returns summaries (to save tokens).

```
get(id?, type?, project?, category?, query?, tags?, limit?)
```

- **id** — Document ID for full detail
- **type** / **project** / **category** — Filters
- **query** — Full-text search within filters
- **tags** — All tags must match
- **limit** — Max results (default 50, max 500)

#### search
Full-text search across all documents. Returns summaries only.

```
search(query, type?, project?, limit?)
```

#### delete
Remove a document by ID.

```
delete(id)
```

#### export
Export knowledge base to markdown.

```
export(project?, type?)
```

#### import
Import knowledge entries from JSON file content.

```
import(content, project?)
```

### Backlog tools

#### project
Create a project or list all. Projects have auto-derived prefixes (e.g., `acme-corp` → `AC`).

```
project(name?)
```

- No args: list all projects
- **name** — Create new project with this name

#### item
Manage backlog items. Mode is auto-detected from inputs.

```
item(id?, project?, priority?, title?, description?, status?, priority_min?, priority_max?)
```

Modes:
- **id + fields** → Update an existing item
- **id only** → Get full detail
- **project + priority + title** → Create new item (status defaults to "open")
- **Filters only** → List summaries

- **priority** — P0 (critical/blocking) through P6 (someday/maybe)
- **status** — `open` or `done` (set `status: "done"` to close)
- **priority_min / priority_max** — Filter range (e.g., `priority_min: "P0"`, `priority_max: "P2"`)

Item IDs: auto-generated as `<project-prefix>-<seq>` (e.g., `AC-1`, `AC-2`)

#### plan
Manage implementation plans. Mode is auto-detected from inputs.

```
plan(id?, title?, content?, status?, project?, item_id?)
```

Modes:
- **id + fields** → Update
- **id only** → Get full plan
- **title (no id)** → Create new plan
- **No id or title** → List all plans

- **status** — `draft` → `active` → `complete` (default `draft`)
- **project** — Link to a project
- **item_id** — Link to a backlog item

#### config
Get or set configuration. One per call.

```
config(key?, value?)
```

- No args: list all config
- **key only** — Get one config value
- **key + value** — Set a value

## Configuration

Default SQLite path: `~/.local/share/ouroboros/kb.db`

Environment variables:

| Var | Description |
|-----|-------------|
| `PROJECT_KB_PATH` | SQLite database path (primary) |
| `QM_DB_PATH` | SQLite database path (fallback alias) |
| `QM_BACKUP_MODE` | Backup strategy: `none`, `dedicated`, or `shared` (default `none`) |
| `QM_GIT_REPO` | Git repository path for backups |
| `QM_SPARSE_PATH` | Sparse checkout directory (shared backup mode) |

## CLI

The server registers a `query` subcommand for direct database access:

```bash
ouroboros query --project <name> [--type <type>] [--limit N]
```

## Testing

```bash
go test -v -coverprofile=coverage.out -timeout=120s ./...
```

## License

See workspace LICENSE.
