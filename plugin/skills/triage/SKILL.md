---
name: triage
description: Review and manage backlog items — reprioritize, update status, clean up stale items
context: fork
model: sonnet
---

1. **Project.** `git rev-parse --show-toplevel | xargs basename`. If not in a git repo, ask which project to triage.

2. **Load state:**
   - `item` with `projects: ["<project>"]` + `status: "open"` — open items

3. **Summarize.** Show open items grouped by priority (P0 first) with counts per level. Priority scale: P0 (critical/blocking) through P6 (someday/maybe). When triaging, consider filtering by `component` to scope to a sub-area; items without component are project-level.

4. **Suggest actions.** If args were supplied (e.g. `/triage reprioritize`), act on them. Otherwise suggest:
   - Items that may be stale (old, no recent updates)
   - Items that could be consolidated
   - Priority adjustments based on current context
   - Items that appear done — run `git log --oneline -20` and cross-reference commit subjects against open item titles; suggest closure for anything clearly landed

5. **Apply.** For each confirmed change, call `item` with entries (id + updated fields). To delete items, use `delete_ids: [...]`. Report what changed.

## Guidelines

- Always show current state before suggesting changes
- Never close or reprioritize without user confirmation
- Group related items when suggesting consolidation
- Component convention: Go binary/library items have no component; plugin wrapper items use `component: "plugin"`
