#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');
const { readStdin, projectFromPath, getBinaryPath, formatContextLines, logHookEvent, isSkippedAgentType, queryKb } = require(__dirname + '/lib');

const MAX_ENTRIES = 5;
const MAX_ITEMS = 5;
const COOLDOWN_MS = 60000; // 60s TTL per session+project

function getCooldownPath(sessionId, project) {
  const key = `${sessionId || 'unknown'}:${project || 'unknown'}`;
  // Simple sanitize for filename
  const safe = key.replace(/[^a-zA-Z0-9_:.-]/g, '-');
  const dir = `${process.env.HOME}/.ouroboros/cooldowns/subagent-start`;
  return path.join(dir, safe);
}

function isWithinSessionCooldown(cooldownPath, cooldownMs) {
  try {
    const stats = fs.statSync(cooldownPath);
    return (Date.now() - stats.mtimeMs) < cooldownMs;
  } catch (e) {
    return false;
  }
}

function touchCooldown(cooldownPath) {
  try {
    fs.mkdirSync(path.dirname(cooldownPath), { recursive: true });
    fs.writeFileSync(cooldownPath, '');
  } catch (e) {}
}

function queryItems(binary, project) {
  try {
    const cmd = `"${binary}" items --project "${project}" --status open --limit ${MAX_ITEMS}`;
    const out = execSync(cmd, { timeout: 3000, encoding: 'utf-8' });
    const items = JSON.parse(out);
    if (!Array.isArray(items)) return null;
    return items;
  } catch (e) {
    return null;
  }
}

async function main() {
  try {
    const input = await readStdin();
    let data = {};
    try {
      data = JSON.parse(input);
    } catch (e) {
      // continue with empty data
    }

    const agent_type = data.agent_type || '';
    const session_id = data.session_id;
    const prompt = (data.prompt) || (data.input && data.input.prompt) || '';

    // Determine project, prefer cwd-based resolution
    const cwd = data.cwd || '';
    let project = null;
    if (cwd) {
      project = projectFromPath(cwd);
    }

    // Log fire event only once
    logHookEvent({ hook: 'subagent_start', kind: 'fire', session_id, project });
    logHookEvent({ hook: 'subagent_start', kind: 'subagent_start', session_id, agent_type });

    // Early exit: skip list
    if (isSkippedAgentType(agent_type)) {
      process.exit(0);
    }

    // Early exit: no project (fallback behavior)
    if (!project) {
      process.exit(0);
    }

    // Session-scoped cooldown: skip context injection if fired recently for same session+project
    const cooldownPath = getCooldownPath(session_id, project);
    if (isWithinSessionCooldown(cooldownPath, COOLDOWN_MS)) {
      process.exit(0);
    }

    // Find the ouroboros binary
    const binary = getBinaryPath();
    if (!binary) {
      process.exit(0);
    }

    // Query KB: search-based if prompt available, else recent
    let rows = null;
    if (prompt) {
      const escaped = prompt.replace(/'/g, '').substring(0, 200);
      rows = queryKb(project, { search: escaped, limit: MAX_ENTRIES });
    }
    // Fall back to recent if no prompt or search returned nothing
    if (!rows || rows.length === 0) {
      rows = queryKb(project, { limit: MAX_ENTRIES });
    }

    // Query open backlog items
    const items = queryItems(binary, project);

    // Silent exit if nothing to inject
    if ((!rows || rows.length === 0) && (!items || items.length === 0)) {
      process.exit(0);
    }

    // Touch cooldown after deciding to inject
    touchCooldown(cooldownPath);

    // Format KB context section
    if (rows && rows.length > 0) {
      const label = prompt ? 'KB context (relevant)' : 'KB context (recent)';
      process.stdout.write(`[ouroboros] ${label}:\n`);
      for (const row of rows) {
        process.stdout.write(`  [${row.type}] ${row.title}\n`);
      }
    }

    // Format open items section
    if (items && items.length > 0) {
      process.stdout.write(`[ouroboros] Open items (top ${items.length}):\n`);
      for (const item of items) {
        const id = item.id || item.item_id || '?';
        const priority = item.priority || '?';
        const title = item.title || '(no title)';
        process.stdout.write(`  ${id} ${priority} ${title}\n`);
      }
    }

    process.exit(0);
  } catch (e) {
    process.exit(0);
  }
}

main();
