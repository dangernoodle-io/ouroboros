---
name: persist
description: Scan conversation for decisions, facts, notes, and plans worth persisting to the ouroboros knowledge base
---

1. **Project name.** Run `git rev-parse --show-toplevel | xargs basename`. If not in a git repo, use `workspace`.

2. **Material.** If args were supplied (`/persist <notes>`), treat them as raw material. Otherwise scan the conversation. Candidate types:
   - `decision` — architectural choices, technology selections, design trade-offs with clear rationale
   - `fact` — configuration values, endpoints, credential references, version numbers, environment details
   - `note` — procedures, processes, how-tos, important observations
   - `relation` — dependencies between components, projects, or systems
   - `plan` — implementation plans discussed or deferred; terse step list in `content`, narrative in `notes`

3. **Search before put.** For each candidate, call `search` with the proposed title. If a matching entry exists for the same project, reuse its title verbatim — the server upserts on `type+project+category+title`. Only skip if existing content is already identical.

4. **Store via `put`** with these fields:
   - `type`, `project`, `title` (concise, searchable — used as the upsert key)
   - `content` — terse, ≤300 chars target / 500 hard cap. Structured:
     ```
     Rule: <the thing>
     Trigger: <when it applies>   (optional)
     Effect: <what happens>        (optional)
     Why: <one-line summary>       (optional)
     ```
     Agents read this on every injection — longer explanation goes in `notes`, not here.
   - `notes` — unlimited narrative for humans (rationale, trade-offs, context); shown only when asked
   - `category` — optional (e.g. `config` for facts, procedure type for notes)
   - `tags` — array

5. **Report.** One line per item:
   - Stored: `[type] title — project`
   - Skipped: `[type] title — already identical`

## Be selective

Skip trivial implementation details, anything derivable from code, temporary debugging notes, and obvious/redundant details.
