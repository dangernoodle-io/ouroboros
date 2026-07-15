---
name: recall
description: Query ouroboros for project context — searches KB entries and backlog items in one shot
context: fork
model: sonnet
---

1. **Project.** `git rev-parse --show-toplevel | xargs basename`.

2. **Query.** Use args as the search query (e.g. `/recall auth middleware`). If no args, do a broad project dump.

3. **Query both sources:**
   - KB: `query` with `domain: "kb"` + `query`/`queries` + `projects: ["<project>"]` for full-text search; if no query, `query` with `domain: "kb"` + `projects: ["<project>"]` for summaries
   - Backlog: `query` with `domain: "backlog"` + `projects: ["<project>"]` (add `status: "open"` for broad queries); narrow to a sub-area with `component="plugin"` (e.g. `query domain="backlog" projects=["ouroboros"] component="plugin"`)

4. **Present** grouped by source:
   - **Knowledge Base** — decisions, facts, notes, relations (summaries only)
   - **Open Items** — backlog grouped by priority

   Targeted queries: highlight best matches. Broad queries: summaries only; fetch full content only on request.

## Guidelines

- Prefer summaries; only `query` with `ids` if the user asks for details
- Cross-reference KB decisions to related open items when relevant
- If no results, say so — don't speculate
- For deep investigation ("why did we do X", code cross-reference, staleness checks), spawn the `knowledge-explorer` subagent instead. This skill is for quick inline lookups.
