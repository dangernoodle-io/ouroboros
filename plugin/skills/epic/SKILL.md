---
name: epic
description: Decompose an epic into child tickets and file them in one atomic backlog write
---

1. **Project.** `git rev-parse --show-toplevel | xargs basename`; if not a git repo, ask which project.

2. **Resolve the epic** (the scope source):
   - Arg is an existing backlog id (e.g. `/epic OU-176`): `get` with `domain: "backlog"`, `ids: [that id]`, `verbose: true` — read title + description + notes as scope. If the item is NOT `EPIC:`-titled, say so and ask whether to decompose it anyway or stop.
   - Arg is free text (`/epic "add semantic search ..."`) or no arg: treat as a NEW epic — draft an `EPIC:`-prefixed title + a concise description from the args and/or current conversation. No-arg = infer the epic under discussion; if unclear, ask.

3. **Propose children.** From the scope, draft a table — each child: title, one-line description, priority (P0-P6), optional component. Rules: one unit of work per child; a child that is itself multi-unit is a sub-epic — flag it rather than burying scope. Bias toward FEWER, well-scoped tickets. Show the epic (existing id, or "NEW — will be created") above the table.

4. **Gate.** Present the proposal and STOP for the user to approve / edit (titles, priorities, scope, component) / add or drop children / reject. Write NOTHING until explicit approval.

5. **Populate** — ONE atomic `backlog` write with `entries[]`:
   - NEW epic: `entries[0]` = the epic (`{title: "EPIC: ...", description, priority}`); each child = `{title, description, priority, component (if any), epic: "$0"}`.
   - EXISTING epic: no epic entry; each child = `{..., epic: "<existing-id>"}`.
   Whole-batch atomic — a single bad entry rolls back everything (no partial populate). `$0` back-references `entries[0]` so the epic and all children land in one call. Report the epic id (new or existing) and every child id.

6. **Roadmap** (optional, opt-in — never automatic). After filing, ASK whether to add the children to the roadmap. If yes: `roadmap` `op=add` for each child under the epic axis (`epic=<epic-id>`), default section `next` (offer `now`/`next`). If the user declines, skip entirely.

7. **Report.** Epic + children table with assigned ids; note whether the roadmap was updated.

## Guidelines

- The gate is load-bearing — never file without explicit approval; decomposition is where over-splitting happens.
- Prefer fewer, well-scoped children; surface a multi-unit child as a sub-epic, don't bury it.
- Keep each description terse (the backlog write enforces a 500-char hard cap); put any longer narrative in the item's notes field, not the description.
- Component convention: Go binary/library items omit component; plugin-wrapper items use `component: "plugin"`.
- New-epic children reference the parent with `epic: "$0"` (positional back-ref to `entries[0]`); existing-epic children use the literal epic id.
- One atomic write for the whole populate — do NOT file the epic and children in separate calls.
