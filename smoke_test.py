#!/usr/bin/env python3
"""Sequential MCP smoke test for ouroboros."""
import json, os, subprocess, sys, tempfile

BINARY = os.path.join(os.path.dirname(os.path.abspath(__file__)), "ouroboros")
DB_PATH = os.path.join(tempfile.gettempdir(), "ouroboros-smoke.db")

# Clean up stale DB from previous runs
if os.path.exists(DB_PATH):
    os.remove(DB_PATH)

proc = subprocess.Popen(
    [BINARY],
    stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
    env={"PROJECT_KB_PATH": DB_PATH, "PATH": ""},
)

def call(id, method, params=None):
    msg = {"jsonrpc": "2.0", "id": id, "method": method}
    if params:
        msg["params"] = params
    proc.stdin.write((json.dumps(msg) + "\n").encode())
    proc.stdin.flush()
    line = proc.stdout.readline().decode().strip()
    return json.loads(line)

def tool(id, name, args=None):
    r = call(id, "tools/call", {"name": name, "arguments": args or {}})
    text = r["result"]["content"][0]["text"]
    is_err = r["result"].get("isError", False)
    try:
        parsed = json.loads(text)
    except Exception:
        parsed = text
    return parsed, is_err

# MCP handshake
call(1, "initialize", {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {"name": "smoke", "version": "1.0"},
})
proc.stdin.write(b'{"jsonrpc":"2.0","method":"notifications/initialized"}\n')
proc.stdin.flush()

# --- Tests ---
print("--- PUT: decision type document ---")
r, err = tool(2, "put", {
    "type": "decision",
    "project": "acme-corp",
    "title": "Use PostgreSQL",
    "content": "Superior query performance for our use case",
    "tags": ["database", "infrastructure"],
})
print(json.dumps(r))
assert not err, f"put decision failed: {r}"
decision_id = r["id"] if not err else None

print("\n--- PUT: fact type document ---")
r, err = tool(3, "put", {
    "type": "fact",
    "project": "acme-corp",
    "category": "hardware",
    "title": "cpu_cores",
    "content": "8",
})
print(json.dumps(r))
assert not err, f"put fact failed: {r}"

print("\n--- PUT: note type document ---")
r, err = tool(4, "put", {
    "type": "note",
    "project": "acme-corp",
    "category": "procedure",
    "title": "release-process",
    "content": "1. Tag version\n2. Push tag\n3. Monitor CI",
    "tags": ["release"],
})
print(json.dumps(r))
assert not err, f"put note failed: {r}"

print("\n--- PUT: relation type document ---")
r, err = tool(5, "put", {
    "type": "relation",
    "project": "acme-corp",
    "category": "depends_on",
    "title": "api->database",
    "content": "API queries the database",
    "metadata": json.dumps({"source": "api", "target": "database"}),
})
print(json.dumps(r))
assert not err, f"put relation failed: {r}"

print("\n--- GET by ID (full content) ---")
if decision_id:
    r, err = tool(6, "get", {"id": decision_id})
    print(json.dumps(r))
    assert not err, f"get by id failed: {r}"

print("\n--- GET list by type (summaries) ---")
r, err = tool(7, "get", {"type": "decision", "project": "acme-corp"})
print(json.dumps(r, indent=2))
assert not err, f"get list by type failed: {r}"

print("\n--- GET list by project (summaries) ---")
r, err = tool(8, "get", {"project": "acme-corp"})
print(json.dumps(r, indent=2) if isinstance(r, list) else json.dumps(r))
assert not err, f"get list by project failed: {r}"

print("\n--- SEARCH by query ---")
r, err = tool(9, "search", {"query": "PostgreSQL"})
print(json.dumps(r, indent=2) if isinstance(r, list) else json.dumps(r))
assert not err, f"search failed: {r}"

print("\n--- SEARCH wildcard query ---")
r, err = tool(9.5, "search", {"query": "*"})
print(json.dumps(r, indent=2) if isinstance(r, list) else json.dumps(r))
assert not err, f"wildcard search failed: {r}"
assert r is not None, "wildcard search returned null"
assert isinstance(r, list), "wildcard search should return list"

print("\n--- EXPORT markdown ---")
r, err = tool(10, "export", {"project": "acme-corp", "type": "decision"})
print(r if isinstance(r, str) else json.dumps(r, indent=2))
assert not err, f"export failed: {r}"

print("\n--- IMPORT JSON ---")
import_data = {
    "documents": [
        {
            "type": "decision",
            "project": "test-proj",
            "title": "Use Redis",
            "content": "Fast caching layer",
            "tags": ["cache"],
        },
        {
            "type": "fact",
            "project": "test-proj",
            "category": "database",
            "title": "postgres_version",
            "content": "15.3",
        },
    ]
}
r, err = tool(11, "import", {
    "content": json.dumps(import_data),
    "project": "test-proj",
})
print(json.dumps(r))
assert not err, f"import failed: {r}"

print("\n--- GET imported documents ---")
r, err = tool(12, "get", {"project": "test-proj"})
print(json.dumps(r, indent=2) if isinstance(r, list) else json.dumps(r))
assert not err, f"get imported failed: {r}"

print("\n--- DELETE document ---")
if decision_id:
    r, err = tool(13, "delete", {"id": decision_id})
    print(json.dumps(r))
    assert not err, f"delete failed: {r}"

print("\n--- SEARCH after delete ---")
r, err = tool(14, "search", {"query": "PostgreSQL", "project": "acme-corp"})
print(json.dumps(r, indent=2) if isinstance(r, list) else json.dumps(r))
assert not err, f"search after delete failed: {r}"

# --- BACKLOG TOOLS ---

print("\n--- PROJECT: create ---")
r, err = tool(15, "project", {"name": "acme-corp"})
print(json.dumps(r))
assert not err, f"project create failed: {r}"

print("\n--- PROJECT: list ---")
r, err = tool(16, "project")
print(json.dumps(r, indent=2))
assert not err, f"project list failed: {r}"

print("\n--- ITEM: create ---")
r, err = tool(17, "item", {"project": "acme-corp", "priority": "P1", "title": "Implement auth", "description": "Add OAuth2 support"})
print(json.dumps(r))
assert not err, f"item create failed: {r}"
item_id = r.get("id", "AC-1")

r, err = tool(18, "item", {"project": "acme-corp", "priority": "P3", "title": "Update docs"})
print(json.dumps(r))
assert not err, f"item create 2 failed: {r}"

print("\n--- ITEM: get by id ---")
r, err = tool(19, "item", {"id": item_id})
print(json.dumps(r, indent=2))
assert not err, f"item get failed: {r}"

print("\n--- ITEM: list ---")
r, err = tool(20, "item", {"project": "acme-corp"})
print(r if isinstance(r, str) else json.dumps(r, indent=2))
assert not err, f"item list failed: {r}"

print("\n--- ITEM: update ---")
r, err = tool(21, "item", {"id": item_id, "title": "Implement auth v2", "priority": "P0"})
print(json.dumps(r))
assert not err, f"item update failed: {r}"

print("\n--- ITEM: mark done ---")
r, err = tool(22, "item", {"id": "AC-2", "status": "done"})
print(json.dumps(r))
assert not err, f"item done failed: {r}"

print("\n--- ITEM: list open only ---")
r, err = tool(23, "item", {"project": "acme-corp", "status": "open"})
print(r if isinstance(r, str) else json.dumps(r, indent=2))
assert not err, f"item list open failed: {r}"

print("\n--- PLAN: create ---")
r, err = tool(24, "plan", {"title": "Auth implementation plan", "content": "## Steps\n1. Add OAuth2 provider\n2. Create middleware\n3. Write tests", "project": "acme-corp"})
print(json.dumps(r))
assert not err, f"plan create failed: {r}"

print("\n--- PLAN: get ---")
r, err = tool(25, "plan", {"id": 1})
print(json.dumps(r, indent=2))
assert not err, f"plan get failed: {r}"

print("\n--- PLAN: update ---")
r, err = tool(26, "plan", {"id": 1, "status": "active"})
print(json.dumps(r))
assert not err, f"plan update failed: {r}"

print("\n--- PLAN: list ---")
r, err = tool(27, "plan")
print(json.dumps(r, indent=2))
assert not err, f"plan list failed: {r}"

print("\n--- CONFIG: set ---")
r, err = tool(28, "config", {"key": "theme", "value": "dark"})
print(json.dumps(r))
assert not err, f"config set failed: {r}"

print("\n--- CONFIG: get ---")
r, err = tool(29, "config", {"key": "theme"})
print(json.dumps(r))
assert not err, f"config get failed: {r}"

print("\n--- CONFIG: list all ---")
r, err = tool(30, "config")
print(json.dumps(r, indent=2))
assert not err, f"config list failed: {r}"

# --- NEW: Test notes field and verbose flag ---

print("\n--- PUT: with notes field ---")
r, err = tool(31, "put", {
    "type": "decision",
    "project": "test-notes",
    "title": "Use Redis",
    "content": "Fast caching",
    "notes": "Redis chosen for 100ms latency target. Considered Memcached but Redis has better data structures.",
})
print(json.dumps(r))
assert not err, f"put with notes failed: {r}"
assert r["action"] == "created", f"expected action=created, got {r}"
notes_doc_id = r["id"]

print("\n--- GET with verbose=true ---")
r, err = tool(32, "get", {"id": notes_doc_id, "verbose": True})
print(json.dumps(r))
assert not err, f"get verbose=true failed: {r}"
assert r["notes"] == "Redis chosen for 100ms latency target. Considered Memcached but Redis has better data structures.", f"notes mismatch: {r}"

print("\n--- GET with verbose=false (default) ---")
r, err = tool(33, "get", {"id": notes_doc_id, "verbose": False})
print(json.dumps(r))
assert not err, f"get verbose=false failed: {r}"
assert "notes" not in r or r["notes"] == "", f"notes should be absent or empty with verbose=false, got: {r}"

print("\n--- PUT: content hard cap validation ---")
long_content = "x" * 501
r, err = tool(34, "put", {
    "type": "decision",
    "project": "test-notes",
    "title": "Oversized",
    "content": long_content,
})
print(json.dumps(r))
assert err, f"expected error for oversized content, got: {r}"

print("\n--- PUT: action=updated on second put ---")
r, err = tool(35, "put", {
    "type": "decision",
    "project": "test-notes",
    "title": "Use Redis",
    "content": "Fast caching v2",
    "notes": "Updated notes",
})
print(json.dumps(r))
assert not err, f"put update failed: {r}"
assert r["action"] == "updated", f"expected action=updated, got {r}"

proc.terminate()
print("\nDone.")
