# ouroboros

[![Go](https://img.shields.io/badge/Go-1.26.1-00ADD8?logo=go)](https://go.dev/)
[![Build](https://github.com/dangernoodle-io/ouroboros/actions/workflows/build.yml/badge.svg)](https://github.com/dangernoodle-io/ouroboros/actions/workflows/build.yml)
[![Release](https://github.com/dangernoodle-io/ouroboros/actions/workflows/release.yml/badge.svg)](https://github.com/dangernoodle-io/ouroboros/actions/workflows/release.yml)
[![Coverage Status](https://coveralls.io/repos/github/dangernoodle-io/ouroboros/badge.svg?branch=main)](https://coveralls.io/github/dangernoodle-io/ouroboros?branch=main)

MCP server for persistent project knowledge base and backlog management. Stores decisions, facts, notes, and relations across conversations. Tracks work items, implementation plans, and project configuration in SQLite.

> **Maintained by AI** — This project is developed and maintained by Claude (via [@dangernoodle-io](https://github.com/dangernoodle-io)).
> If you find a bug or have a feature request, please [open an issue](https://github.com/dangernoodle-io/ouroboros/issues) with examples so it can be addressed.


## Tools

| Namespace | Tools | Docs |
|-----------|-------|------|
| Read | `query` — fetch/search entries (requires domain: kb\|backlog\|roadmap) | [Wiki](../../wiki/Knowledge-Base), [Wiki](../../wiki/Backlog), [Wiki](../../wiki/Roadmap) |
| Write | `kb` (KB entries), `backlog` (items), `roadmap` (per-project sections, grouped by component/epic) | [Wiki](../../wiki/Knowledge-Base), [Wiki](../../wiki/Backlog), [Wiki](../../wiki/Roadmap) |

Cross-reference edges (`blocks`/`relates`/`explains` between items/KB docs) fold into the existing tools rather than a 6th tool: `backlog` write entries[].edges[], `kb` write `[[Title]]` autolinks, `query` verbose=true edges sidecar. See [Backlog](../../wiki/Backlog).

Operator-style ops (`project`, `plan`, `config`, `kb delete <id>...`, `export`, `import`, `roadmap`, `link`/`unlink`) are CLI-only — see `ouroboros --help`.

## Use with Claude Code

The recommended way to run ouroboros is via the marketplace plugin — it handles installation and wires up auto-context injection, persistence hooks, and workflow skills on top of the raw MCP server.

```
/plugin marketplace add dangernoodle-io/dangernoodle-marketplace
/plugin install ouroboros-mcp@dangernoodle-marketplace
```

The plugin adds, beyond the raw MCP tools:

- Auto-installs the `ouroboros` binary on session start — no manual install step
- Hooks that inject project KB context into every turn and auto-persist decisions when conversations end
- Skills: `/persist`, `/recall`, `/triage`, `/epic` for common KB and backlog workflows
- Subagents: `backlog-manager` and `knowledge-explorer` for deeper investigation

Source: [dangernoodle-io/dangernoodle-marketplace](https://github.com/dangernoodle-io/dangernoodle-marketplace).

## Install the binary standalone

If you're not using Claude Code, or you want ouroboros as a plain MCP server without the plugin's hooks and skills, install the binary directly.

### Homebrew

```bash
brew install dangernoodle-io/tap/ouroboros
```

### From Source

```bash
go install dangernoodle.io/ouroboros@latest
```

### GitHub Releases

Download pre-built binaries from [releases](https://github.com/dangernoodle-io/ouroboros/releases).

### Register manually with Claude Code

```bash
claude mcp add --scope user ouroboros /absolute/path/to/ouroboros -- server
```

The `--` separator passes `server` as an argument to the binary (not to `claude mcp add` itself), matching the plugin's own `plugin.json` (`"args": ["server"]`) — bare `ouroboros` is a help dispatcher, not the MCP server, so omitting `server` here registers a server that never actually starts. This gives you the 4 MCP tools but none of the auto-context injection or persistence hooks that the plugin provides.

## Browse with the noun-first CLI

When you're not using the MCP server, the noun-first subcommands provide a read-only CLI for browsing (plus writes: `backlog create/update/delete`, `kb write`, `edges link/unlink`). All commands support tabular output and `--json` for scripting. The `roadmap show` command renders a per-project roadmap as Markdown, or as a rich self-contained HTML fragment with `--html`.

```bash
ouroboros project list                              # list all projects
ouroboros backlog list --project acme-corp          # list items in a project
ouroboros backlog get AC-1                          # show item detail
ouroboros kb search caching                         # search knowledge base
ouroboros kb get 42 --json                          # fetch document as JSON
ouroboros plan list --status active                 # list active plans
ouroboros roadmap show acme-corp --by epic          # print roadmap grouped by epic
ouroboros roadmap show acme-corp --html -o rm.html  # render a standalone HTML file
ouroboros edges link item:BB-9 blocks item:TM-40    # create a cross-reference edge (top-level `link` alias also works)
ouroboros edges list --label blocks                 # list edges
```

Flags: `backlog list`: `--project`, `--status` (repeatable), `--priority` (P0–P6, exact match), `--component`, `--epic`, `--epics`, `--since`, `--sort created`, `--limit`. `backlog get <id>...`: `--verbose`. `backlog search <text>`: same filters as `list` (single-valued `--status`). `backlog create`: `--project`, `--priority` (required, P0-P6), `--title` (required), `--component`, `--epic`, `--description` (500 char cap), `--notes`. `backlog update <id>`: `--priority`, `--title`, `--status`, `--component`, `--epic`, `--description`, `--notes`, `--append-notes`. `backlog delete <id>...`: all-or-nothing. `kb list`: `--project`, `--type`, `--category`, `--tag` (repeatable), `--limit`. `kb search <text>`: `--project`, `--type`, `--category`, `--limit`. `kb get <id>...`: `--verbose`. `plan list`: `--project`, `--status`. `edges list`: `--label`, `--type item|kb` + `--id` (together). All read subcommands: `--json`. Roadmap: `show` (`--by component|epic`, `--component`, `--epic`, `--html` for a self-contained HTML fragment/standalone doc instead of Markdown, `--output`/`-o <file>` to write it — without `-o` the bare embeddable fragment prints to stdout, with `-o` it's wrapped in a minimal standalone `<!doctype html>` document), `add`, `update`, `move`, `reorder`, `done`, `remove`, `seed` (`--backlog`, `--priority` — max cap, e.g. `P2` includes P0-P2, unlike `backlog list --priority`'s exact match — `--component`, `--status`, `--replace` — resyncs only the current fetch's matches, leaving previously-seeded items outside the filter untouched) — see `ouroboros roadmap --help`. Edges: `edges link <src> <label> <dst>` / `edges unlink <src> <label> <dst>` (also available as top-level `link`/`unlink`) where src/dst are `item:<id>` or `kb:<id>`.

## Server

`ouroboros server [--http <addr>] [--stateless] [--read-only]` runs the MCP server; stdio by default. `--http <addr>` switches to streamable-HTTP on that address; `--stateless` (defaults to true in HTTP mode) drops session state between requests; `--read-only` advertises only read-only tools, gating out destructive ones.

## Configuration

ouroboros stores the knowledge base and backlog in SQLite. See [Configuration](../../wiki/Configuration) for environment variables and the default database path.

## License

See workspace LICENSE.
