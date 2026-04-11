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

print("\n--- GET by ID (full content) ---")
if decision_id:
    r, err = tool(6, "get", {"id": decision_id})
    print(json.dumps(r))

print("\n--- GET list by type (summaries) ---")
r, err = tool(7, "get", {"type": "decision", "project": "acme-corp"})
print(json.dumps(r, indent=2))

print("\n--- GET list by project (summaries) ---")
r, err = tool(8, "get", {"project": "acme-corp"})
print(json.dumps(r, indent=2) if isinstance(r, list) else json.dumps(r))

print("\n--- SEARCH by query ---")
r, err = tool(9, "search", {"query": "PostgreSQL"})
print(json.dumps(r, indent=2) if isinstance(r, list) else json.dumps(r))

print("\n--- EXPORT markdown ---")
r, err = tool(10, "export", {"project": "acme-corp", "type": "decision"})
print(r if isinstance(r, str) else json.dumps(r, indent=2))

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

print("\n--- GET imported documents ---")
r, err = tool(12, "get", {"project": "test-proj"})
print(json.dumps(r, indent=2) if isinstance(r, list) else json.dumps(r))

print("\n--- DELETE document ---")
if decision_id:
    r, err = tool(13, "delete", {"id": decision_id})
    print(json.dumps(r))

print("\n--- SEARCH after delete ---")
r, err = tool(14, "search", {"query": "PostgreSQL", "project": "acme-corp"})
print(json.dumps(r, indent=2) if isinstance(r, list) else json.dumps(r))

proc.terminate()
print("\nDone.")
