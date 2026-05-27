#!/usr/bin/env python3
"""OU-77 response-shape benchmark probe.

Measures MCP tool response sizes for the 8 primary + 3 stress shapes
identified in OU-77. Outputs a markdown table to stdout.

Usage:
    python3 bench/response-shapes/probe.py

Env:
    BENCH_DB  Path for throwaway SQLite DB (default: /tmp/ou-bench-response.db)
"""

import json
import os
import select
import subprocess
import sys
import time

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------

BINARY = os.path.join(os.path.dirname(os.path.dirname(os.path.dirname(
    os.path.abspath(__file__)))), "ouroboros")
BENCH_DB = os.environ.get("BENCH_DB", "/tmp/ou-bench-response.db")

# Thresholds (tokens)
WARN = 4000
CRIT = 8000

# ---------------------------------------------------------------------------
# Seed data (deterministic)
# ---------------------------------------------------------------------------

# 2 projects must be created via CLI before seeding — we create them by
# running the ouroboros CLI binary directly with project subcommand.
PROJECTS = ["alpha-proj", "beta-proj"]

KB_DOCS = []
for i in range(1, 6):
    KB_DOCS.append({
        "type": "decision", "project": "alpha-proj",
        "category": "architecture",
        "title": f"Alpha decision {i}",
        "content": f"Decision content {i}: " + ("lorem ipsum dolor sit amet " * 8).strip(),
    })
for i in range(1, 6):
    KB_DOCS.append({
        "type": "fact", "project": "alpha-proj",
        "category": "config",
        "title": f"Alpha fact {i}",
        "content": f"Fact content {i}: " + ("key=value configuration entry " * 6).strip(),
    })
for i in range(1, 6):
    KB_DOCS.append({
        "type": "note", "project": "beta-proj",
        "category": "ops",
        "title": f"Beta note {i}",
        "content": f"Note content {i}: " + ("operational detail for beta " * 6).strip(),
    })

ITEMS = []
for i in range(1, 11):
    ITEMS.append({
        "project": "alpha-proj",
        "priority": "P2",
        "title": f"Alpha backlog item {i}",
        "status": "open",
    })
for i in range(1, 6):
    ITEMS.append({
        "project": "beta-proj",
        "priority": "P3",
        "title": f"Beta backlog item {i}",
        "status": "open",
    })

# ---------------------------------------------------------------------------
# MCP session helpers
# ---------------------------------------------------------------------------

def start_proc(db_path):
    env = os.environ.copy()
    env["PROJECT_KB_PATH"] = db_path
    env["QM_BACKUP_MODE"] = "none"
    return subprocess.Popen(
        [BINARY],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        env=env,
    )


def send(proc, msg):
    line = json.dumps(msg) + "\n"
    proc.stdin.write(line.encode())
    proc.stdin.flush()


def recv_id(proc, target_id, timeout=10):
    deadline = time.time() + timeout
    buf = {}
    while time.time() < deadline:
        ready, _, _ = select.select([proc.stdout], [], [], 0.3)
        if ready:
            line = proc.stdout.readline()
            if not line:
                break
            try:
                obj = json.loads(line.decode().strip())
                oid = obj.get("id")
                if oid is not None:
                    buf[oid] = obj
                if oid == target_id:
                    return obj
            except Exception:
                pass
    return buf.get(target_id)


def mcp_session(db_path):
    """Open an MCP session, initialize it, and return (proc, next_id_counter).
    Caller must close proc.stdin and call proc.wait() when done.
    """
    proc = start_proc(db_path)
    send(proc, {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
        "protocolVersion": "2024-11-05",
        "capabilities": {},
        "clientInfo": {"name": "bench", "version": "1"},
    }})
    recv_id(proc, 1)
    line = json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n"
    proc.stdin.write(line.encode())
    proc.stdin.flush()
    # Unlock tier-1 by calling search
    send(proc, {"jsonrpc": "2.0", "id": 2, "method": "tools/call",
                "params": {"name": "search", "arguments": {"query": "x"}}})
    recv_id(proc, 2)
    return proc


def call_tool(proc, req_id, name, arguments):
    send(proc, {"jsonrpc": "2.0", "id": req_id, "method": "tools/call",
                "params": {"name": name, "arguments": arguments}})
    resp = recv_id(proc, req_id)
    if resp is None:
        return None
    # Extract text content from MCP tool response
    content = resp.get("result", {}).get("content", [])
    text = ""
    for block in content:
        if isinstance(block, dict) and block.get("type") == "text":
            text += block.get("text", "")
    return text

# ---------------------------------------------------------------------------
# Seeding via CLI
# ---------------------------------------------------------------------------

def seed_db(db_path):
    """Seed projects, KB docs, and items into db_path using the ouroboros CLI."""
    env = os.environ.copy()
    env["PROJECT_KB_PATH"] = db_path
    env["QM_BACKUP_MODE"] = "none"

    def cli(*args):
        result = subprocess.run(
            [BINARY] + list(args),
            env=env,
            capture_output=True,
            timeout=15,
        )
        if result.returncode != 0:
            raise RuntimeError(
                f"CLI {args} failed: {result.stderr.decode()[:200]}"
            )
        return result.stdout.decode()

    # Create projects
    for name in PROJECTS:
        cli("project", "create", name)

    # Seed KB docs via MCP put (batch)
    proc = mcp_session(db_path)
    req_id = 10
    batch_size = 5
    for i in range(0, len(KB_DOCS), batch_size):
        batch = KB_DOCS[i:i + batch_size]
        call_tool(proc, req_id, "put", {"entries": batch})
        req_id += 1
    proc.stdin.close()
    proc.wait(timeout=10)

    # Seed items via MCP item (one at a time to get stable IDs)
    proc = mcp_session(db_path)
    for item in ITEMS:
        call_tool(proc, req_id, "item", {"entries": [item]})
        req_id += 1
    proc.stdin.close()
    proc.wait(timeout=10)

# ---------------------------------------------------------------------------
# Measurement
# ---------------------------------------------------------------------------

def measure_shape(proc, req_id, name, arguments):
    """Call a tool, return (response_text_bytes, req_id+1)."""
    text = call_tool(proc, req_id, name, arguments)
    if text is None:
        return 0, req_id + 1
    return len(text.encode("utf-8")), req_id + 1


def flag(tokens):
    if tokens <= WARN:
        return "✓"
    if tokens <= CRIT:
        return "⚠"
    return "✗"


def run_bench(db_path):
    shapes = [
        # (label, tool_name, arguments)
        ("get filter v=false 5 decisions",
         "get", {"projects": ["alpha-proj"], "type": "decision"}),
        ("get filter all 10 docs",
         "get", {"projects": ["alpha-proj"]}),
        ("get ids=[1] single",
         "get", {"ids": [1]}),
        ("search 6 hits",
         "search", {"query": "alpha", "limit": 6}),
        ("item filter 8 items",
         "item", {"filters": {"projects": ["alpha-proj"], "limit": 8}}),
        ("item filter v=true",
         "item", {"filters": {"projects": ["alpha-proj"], "limit": 8}, "verbose": True}),
        ("item ids=[5]",
         "item", {"ids": [f"AL-{i}" for i in range(1, 6)]}),
        ("put entries=[10 new docs]",
         "put", {"entries": [{
             "type": "note", "project": "alpha-proj",
             "category": "bench", "title": f"Bench probe note {n}",
             "content": "Probe note for put-shape measurement.",
         } for n in range(1, 11)]}),
        ("item entries=[10 updates]",
         "item", {"entries": [{
             "id": f"AL-{i}", "status": "open",
         } for i in range(1, 11)]}),
        # Stress shapes
        ("stress: get ids=[1..15] v=false",
         "get", {"ids": list(range(1, 16))}),
        ("stress: get ids=[1..15] v=true",
         "get", {"ids": list(range(1, 16)), "verbose": True}),
        ("stress: item ids=[1..15]",
         "item", {"ids": [f"AL-{i}" for i in range(1, 11)] + [f"BE-{i}" for i in range(1, 6)]}),
    ]

    proc = mcp_session(db_path)
    req_id = 100

    results = []
    for label, tool, args in shapes:
        nbytes, req_id = measure_shape(proc, req_id, tool, args)
        tokens = nbytes // 4
        results.append((label, nbytes, tokens))

    proc.stdin.close()
    proc.wait(timeout=10)
    return results


# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

def print_table(results):
    col_shape = max(len(r[0]) for r in results)
    col_shape = max(col_shape, len("Shape"))

    header = (
        f"| {'Shape':<{col_shape}} | {'Bytes':>7} | {'~Tokens':>8} | {'Flag':>4} |"
    )
    sep = f"| {'-' * col_shape} | {'-' * 7} | {'-' * 8} | {'-' * 4} |"

    print(header)
    print(sep)
    for label, nbytes, tokens in results:
        f = flag(tokens)
        print(f"| {label:<{col_shape}} | {nbytes:>7} | {tokens:>8} | {f:>4} |")

    print()
    print(f"Thresholds: ✓ ≤{WARN} tk  ⚠ {WARN}–{CRIT} tk  ✗ >{CRIT} tk")


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    if not os.path.isfile(BINARY):
        print(f"ERROR: binary not found at {BINARY}", file=sys.stderr)
        print("Run `make build` first.", file=sys.stderr)
        sys.exit(1)

    db_path = BENCH_DB

    # Remove stale DB if present
    if os.path.exists(db_path):
        os.unlink(db_path)

    try:
        print(f"Seeding benchmark DB: {db_path}", file=sys.stderr)
        seed_db(db_path)
        print("Running shapes...", file=sys.stderr)
        results = run_bench(db_path)
        print_table(results)
    finally:
        for suffix in ["", "-wal", "-shm"]:
            p = db_path + suffix
            if os.path.exists(p):
                try:
                    os.unlink(p)
                except OSError:
                    pass

    return 0


if __name__ == "__main__":
    sys.exit(main())
