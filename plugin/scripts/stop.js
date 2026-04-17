#!/usr/bin/env node

const fs = require('fs');
const { execSync } = require('child_process');
const { readStdin, projectFromPath, getBinaryPath, extractKbBlock, matchesAnyPattern, logHookEvent } = require(__dirname + '/lib');

// Tier-2 check: patterns indicating the main context already persisted
const ALREADY_PERSISTED_PATTERNS = [
  /\bouroboros\b/i,
  /\bknowledge base\b/i,
  /\bput MCP\b/i,
  /\bpersisted?\b/i,
  /\bmcp__.*__put\b/i,
];

// Tier-1 check: decision language patterns that warrant a nudge
const DECISION_PATTERNS = [
  /\bdecided to\b/i,
  /\bchose .+ over\b/i,
  /\btrade-?off/i,
  /\barchitect(ure|ural)\b/i,
  /\bdesign decision/i,
  /\bgoing with\b/i,
  /\bapproach(?: is|:)/i,
  /\bwe('|')ll use\b/i,
  /\binstead of\b.{0,30}\bbecause\b/i,
  /\brationale\b/i,
];

// readLastMainAssistantText scans a Claude Code transcript JSONL backwards and
// returns the concatenated text of the most recent main-context (non-sidechain)
// assistant turn. Returns '' if not found.
function readLastMainAssistantText(transcriptPath) {
  let raw;
  try {
    raw = fs.readFileSync(transcriptPath, 'utf-8');
  } catch (e) {
    return '';
  }
  const lines = raw.split('\n');
  for (let i = lines.length - 1; i >= 0; i--) {
    const line = lines[i];
    if (!line) continue;
    let obj;
    try {
      obj = JSON.parse(line);
    } catch (e) {
      continue;
    }
    if (obj.type !== 'assistant') continue;
    if (obj.isSidechain === true) continue;
    const content = (obj.message && obj.message.content) || [];
    const text = content
      .filter(c => c && c.type === 'text' && typeof c.text === 'string')
      .map(c => c.text)
      .join('\n');
    if (text) return text;
  }
  return '';
}

async function main() {
  try {
    const input = await readStdin();

    let data;
    try {
      data = JSON.parse(input);
    } catch (e) {
      process.exit(0);
    }

    // CRITICAL: avoid infinite loop when this hook causes the next turn.
    if (data.stop_hook_active === true) {
      process.exit(0);
    }

    const transcriptPath = data.transcript_path || '';
    if (!transcriptPath) {
      process.exit(0);
    }

    // Determine project, prefer cwd-based resolution
    const cwd = data.cwd || '';
    let project = null;
    if (cwd) {
      project = projectFromPath(cwd);
    }

    const sessionId = data.session_id;
    logHookEvent({ hook: 'stop', kind: 'fire', session_id: sessionId, project });

    let message = readLastMainAssistantText(transcriptPath);
    if (!message || message.length < 80) {
      process.exit(0);
    }

    // Truncate to 5000 chars for matching
    message = message.substring(0, 5000);

    const sessionShort = (data.session_id || 'main').substring(0, 8);

    // KB block extraction: try to extract and persist fenced kb block
    const { matched, json } = extractKbBlock(message);
    if (matched) {
      try {
        JSON.parse(json);
      } catch (parseErr) {
        console.error(`[ouroboros] main ${sessionShort}: kb block JSON parse error: ${parseErr.message}`);
        process.exit(0);
      }
      if (!project) {
        console.error(`[ouroboros] main ${sessionShort}: kb block found but no project (run inside a git repo)`);
        process.exit(0);
      }
      const binary = getBinaryPath();
      if (!binary) {
        console.error(`[ouroboros] main ${sessionShort}: kb block found but ouroboros binary not available`);
        process.exit(0);
      }
      try {
        // Parse entries and inject metadata
        let entries = JSON.parse(json);
        if (!Array.isArray(entries)) {
          entries = [entries];
        }
        const source = {
          source: 'hook:stop',
          session_id: sessionId || '',
        };
        entries.forEach(e => {
          e.metadata = { ...(e.metadata || {}), ...source };
        });
        const injectedJson = JSON.stringify(entries);

        const cmd = `"${binary}" put --stdin --project "${project}"`;
        const result = execSync(cmd, { input: injectedJson, timeout: 3000, encoding: 'utf-8' });
        const parsed = JSON.parse(result);
        const resultEntries = Array.isArray(parsed) ? parsed : [parsed];
        const ids = resultEntries.map(e => e.id).filter(id => id !== undefined);
        console.error(`[ouroboros] main ${sessionShort}: persisted ${resultEntries.length} entries to ${project} [ids: ${ids.join(',')}]`);
        logHookEvent({ hook: 'stop', kind: 'persist', session_id: sessionId, project, entries: resultEntries.length, ids });
        process.exit(0);
      } catch (execErr) {
        console.error(`[ouroboros] main ${sessionShort}: put failed: ${execErr.message}`);
        logHookEvent({ hook: 'stop', kind: 'error', detail: execErr.message, session_id: sessionId, project });
        process.exit(0);
      }
    }

    // Default: exit silently (exploratory output)
    logHookEvent({ hook: 'stop', kind: 'noop', session_id: sessionId, project });
    process.exit(0);
  } catch (e) {
    process.exit(0);
  }
}

main();
