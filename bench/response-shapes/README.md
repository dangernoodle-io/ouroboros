# bench/response-shapes

Response-shape benchmark probe for ouroboros MCP tools (OU-77).

## Why this exists

OU-77 asked whether ouroboros tool *responses* are a meaningful token cost.
The probe seeds a deterministic fixture, calls the 8 primary response shapes
plus 3 stress shapes, and prints byte/token measurements. Finding: response
costs are 0.1–0.6x the request baseline (~669 tk); response bloat is not the
dominant cost vector.

Findings are persisted in the KB as:
> "MCP response-side token cost measurement (OU-77)"

## How to run

```bash
# via make (also rebuilds the binary)
make bench

# directly (binary must already exist)
python3 bench/response-shapes/probe.py

# custom DB path
BENCH_DB=/tmp/my-bench.db python3 bench/response-shapes/probe.py
```

Output is a markdown table to stdout; progress lines go to stderr.

## Shapes measured

**Primary (8):** get-filter-decisions, get-filter-all, get-single-id,
search-6-hits, item-filter, item-filter-verbose, item-single-id, put-1-doc.

**Stress (3):** get 15 ids v=false, get 15 ids v=true, item 15 ids.

## Threshold convention

| Flag | Tokens | Meaning |
|------|--------|---------|
| ✓ | ≤ 4000 | Acceptable |
| ⚠ | 4000–8000 | Worth watching |
| ✗ | > 8000 | Investigate |

The ≤4000 tk threshold is the red flag from OU-77. All primary shapes are
expected well below it (~107–414 tk). Stress shapes may approach or exceed it.

## CI status

Not run in CI today. See OU-211 follow-up item for adding a `bench` step.
