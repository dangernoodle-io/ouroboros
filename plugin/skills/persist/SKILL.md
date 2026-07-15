---
name: persist
description: Scan conversation for decisions, facts, notes, and plans worth persisting to the ouroboros knowledge base
---

1. **Project name.** Run `git rev-parse --show-toplevel | xargs basename`. If not in a git repo, use `workspace`.

2. **Material.** If args were supplied (`/persist <notes>`), treat them as raw material. Otherwise scan the conversation. Candidate types:
   - `decision` — architectural choices, technology selections, design trade-offs with clear rationale
   - `fact` — configuration values, endpoints, credential references, version numbers, environment details
   - `note` — procedures, processes, how-tos, important observations
   - `relation` — dependencies between components, projects, or systems, e.g.
     ```
     type: relation
     title: breadboard depends on ouroboros KB
     content:
       Rule: breadboard's persist hooks call ouroboros MCP tools directly.
       Trigger: any breadboard release that changes hook payloads.
       Effect: bump the ouroboros plugin pin in breadboard's marketplace ref.
     ```
   - `plan` — implementation plans discussed or deferred; terse step list in `content`, narrative in `notes`

3. **Search before kb.** Collect all candidate titles, then call `query` once with `queries: [title1, title2, ...]` and `projects: ["<project>"]`. The response is positional — `results[i]` corresponds to `queries[i]`. If a matching entry exists for the same project, reuse its title verbatim — the server upserts on `type+project+category+title`. Only skip if existing content is already identical.

4. **Store via `kb`** with these fields:
   - `type`, `project`, `title` (concise, searchable — used as the upsert key). If step 3 found a near-match needing a retitle or correction (e.g. fixing an earlier duplicate) rather than a fresh entry, pass that entry's `id` to update it in place instead of creating another one.
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
   - `component` — optional subproject tag (e.g. `"plugin"`, `"app"`) when filing item-typed work scoped to a sub-area

5. **Report.** One line per item:
   - Stored: `[type] title — project`
   - Skipped: `[type] title — already identical`

6. **Emit KB block.** After storing, emit a summary ```kb``` fenced block listing all persisted entries (JSON array with `"_persisted_by": "persist-skill"` at the top level or on the first entry). This sentinel prevents the stop hook from re-persisting the same entries. Do not run `kb` twice.

## Be selective

Skip trivial implementation details, anything derivable from code, temporary debugging notes, and obvious/redundant details.

**KB = knowledge only.** Do not persist work status, task tracking, or TODO-like material to the KB — that belongs in backlog tickets, not a `decision`/`note`/`plan` entry.

**Multi-unit work → tickets, not a KB entry.** If the material describes more than one or two units of work, it's an epic (`EPIC: <name>` parent + child tickets) filed via the backlog, not KB content. Redirect it there instead of storing it here.
