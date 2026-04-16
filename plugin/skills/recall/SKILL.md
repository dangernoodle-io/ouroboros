---
name: recall
description: Query ouroboros for project context — searches KB entries, backlog items, and plans in one shot
context: fork
model: haiku
---

1. **Project.** `git rev-parse --show-toplevel | xargs basename`.

2. **Query.** Use args as the search query (e.g. `/recall auth middleware`). If no args, do a broad project dump.

3. **Query all three sources:**
   - KB: `search` with query + project filter; if no query, `get` with project filter for summaries
   - Backlog: `item` with project filter (add `status: "open"` for broad queries)
   - Plans: `plan` with project filter

4. **Present** grouped by source:
   - **Knowledge Base** — decisions, facts, notes, relations (summaries only)
   - **Open Items** — backlog grouped by priority
   - **Plans** — active + draft with status

   Targeted queries: highlight best matches. Broad queries: summaries only; fetch full content only on request.

## Guidelines

- Prefer summaries; only `get` with `id` if the user asks for details
- Cross-reference KB decisions to related open items when relevant
- If no results, say so — don't speculate
- For deep investigation ("why did we do X", code cross-reference, staleness checks), spawn the `knowledge-explorer` subagent instead. This skill is for quick inline lookups.
