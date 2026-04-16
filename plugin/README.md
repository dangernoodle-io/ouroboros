# ouroboros-mcp

Project knowledge base and backlog management for Claude Code. Persist decisions, facts, and notes across conversations. Track work items, plans, and project configuration.

## Install

```bash
/plugin install ouroboros-mcp@dangernoodle-marketplace
```

## What it does

- **MCP server with 10 tools** for knowledge base and backlog management. See the [server wiki](https://github.com/dangernoodle-io/ouroboros/wiki) for full tool reference.
- **6 hooks auto-inject context** into prompts and persist kb blocks on turn end. SessionStart installs the binary; UserPromptSubmit and SubagentStart inject KB context; SubagentStop and Stop auto-persist or nudge.
- **3 skills** for common tasks: `/persist` (save a decision/fact), `/recall` (query KB), `/triage` (manage backlog).

## Hooks

- **SessionStart** — install binary from GitHub or local Homebrew
- **UserPromptSubmit** — inject project KB context before turn starts
- **SubagentStart** — inject KB context when a subagent spawns (skip read-only agents)
- **SubagentStop** — detect and persist kb blocks from subagent output; nudge if decisions found
- **PostToolUse Bash** — nudge to persist after git commits
- **PostToolUse Edit/Write** — flag related KB entries for review
- **Stop** — final auto-persist or nudge on turn end

## Configuration

- `OUROBOROS_DEV_BINARY` — path to local dev binary (bypasses GitHub download)
- `PROJECT_KB_PATH` — override SQLite KB database path (default: `~/.cloak/plugins/.../ouroboros.db`)
- `QM_DB_PATH` — override backlog database path (default: `~/.cloak/plugins/.../backlog.db`)
- `QM_BACKUP_MODE` — backup strategy: `none`, `dedicated`, or `shared` (default: `dedicated`)

## Server

Source and detailed docs at [github.com/dangernoodle-io/ouroboros](https://github.com/dangernoodle-io/ouroboros).

## License

MIT
