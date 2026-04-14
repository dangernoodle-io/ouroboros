# ouroboros

[![Go](https://img.shields.io/badge/Go-1.26.1-00ADD8?logo=go)](https://go.dev/)
[![Build](https://github.com/dangernoodle-io/ouroboros/actions/workflows/build.yml/badge.svg)](https://github.com/dangernoodle-io/ouroboros/actions/workflows/build.yml)
[![Release](https://github.com/dangernoodle-io/ouroboros/actions/workflows/release.yml/badge.svg)](https://github.com/dangernoodle-io/ouroboros/actions/workflows/release.yml)
[![Coverage Status](https://coveralls.io/repos/github/dangernoodle-io/ouroboros/badge.svg?branch=main)](https://coveralls.io/github/dangernoodle-io/ouroboros?branch=main)

MCP server for persistent project knowledge base and backlog management. Stores decisions, facts, notes, and relations across conversations. Tracks work items, implementation plans, and project configuration in SQLite.

> **Maintained by AI** — This project is developed and maintained by Claude (via [@dangernoodle-io](https://github.com/dangernoodle-io)).
> If you find a bug or have a feature request, please [open an issue](https://github.com/dangernoodle-io/ouroboros/issues) with examples so it can be addressed.

## Tools

| Tool | Domain | Docs |
|------|--------|------|
| put | KB | [Wiki](../../wiki/Knowledge-Base#put) |
| get | KB | [Wiki](../../wiki/Knowledge-Base#get) |
| search | KB | [Wiki](../../wiki/Knowledge-Base#search) |
| delete | KB | [Wiki](../../wiki/Knowledge-Base#delete) |
| export | KB | [Wiki](../../wiki/Knowledge-Base#export) |
| import | KB | [Wiki](../../wiki/Knowledge-Base#import) |
| project | Backlog | [Wiki](../../wiki/Backlog#project) |
| item | Backlog | [Wiki](../../wiki/Backlog#item) |
| plan | Backlog | [Wiki](../../wiki/Backlog#plan) |
| config | Backlog | [Wiki](../../wiki/Backlog#config) |

## Install

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

## Register with Claude Code

```bash
claude mcp add --scope user ouroboros /absolute/path/to/ouroboros
```

## Configuration

ouroboros stores the knowledge base and backlog in SQLite. See [Configuration](../../wiki/Configuration) for environment variables and the default database path.

## License

See workspace LICENSE.
