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
| KB | `put`, `get`, `delete`, `search`, `export`, `import` | [Wiki](../../wiki/Knowledge-Base) |
| Backlog | `project`, `item`, `plan`, `config` | [Wiki](../../wiki/Backlog) |

## Use with Claude Code

The recommended way to run ouroboros is via the marketplace plugin — it handles installation and wires up auto-context injection, persistence hooks, and workflow skills on top of the raw MCP server.

```
/plugin marketplace add dangernoodle-io/dangernoodle-marketplace
/plugin install ouroboros-mcp@dangernoodle-marketplace
```

The plugin adds, beyond the raw MCP tools:

- Auto-installs the `ouroboros` binary on session start — no manual install step
- Hooks that inject project KB context into every turn and auto-persist decisions when conversations end
- Skills: `/persist`, `/recall`, `/triage` for common KB and backlog workflows
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
claude mcp add --scope user ouroboros /absolute/path/to/ouroboros
```

This gives you the 10 MCP tools but none of the auto-context injection or persistence hooks that the plugin provides.

## Configuration

ouroboros stores the knowledge base and backlog in SQLite. See [Configuration](../../wiki/Configuration) for environment variables and the default database path.

## License

See workspace LICENSE.
