#!/usr/bin/env node

const { execSync } = require('child_process');
const { readStdin, getProject, projectFromPath, getBinaryPath, extractKbBlock, matchesAnyPattern, logHookEvent, isSkippedAgentType } = require(__dirname + '/lib');

// Tier-2 check: patterns indicating the subagent already persisted
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

async function main() {
  try {
    const input = await readStdin();

    // Parse JSON
    let data;
    try {
      data = JSON.parse(input);
    } catch (e) {
      process.exit(0);
    }

    const agent_id = data.agent_id || '';
    const agent_type = data.agent_type || '';
    const session_id = data.session_id;
    let message = data.last_assistant_message || '';

    // Determine project, prefer cwd-based resolution
    const cwd = data.cwd || '';
    let project = null;
    if (cwd) {
      project = projectFromPath(cwd);
    }
    if (!project) {
      project = getProject();
    }

    // Log fire event
    logHookEvent({ hook: 'subagent_stop', kind: 'fire', session_id, project });

    // Log subagent_stop event unconditionally, before any early exits
    const excerpt = (message || '').substring(0, 120).replace(/\n/g, ' ');
    logHookEvent({ hook: 'subagent_stop', kind: 'subagent_stop', session_id, agent_id, agent_type, last_message_excerpt: excerpt });

    // Early exit: skip list
    if (isSkippedAgentType(agent_type)) {
      process.exit(0);
    }

    // Early exit: empty or too short message
    if (!message || typeof message !== 'string' || message.length < 80) {
      process.exit(0);
    }

    // Truncate to 5000 chars for matching
    message = message.substring(0, 5000);

    const agent_id_short = agent_id.substring(0, 8);

    // KB block extraction: try to extract and persist fenced kb block
    const { matched, json } = extractKbBlock(message);
    if (matched) {
      try {
        JSON.parse(json);
      } catch (parseErr) {
        console.error(`[ouroboros] subagent ${agent_id_short}: kb block JSON parse error: ${parseErr.message}`);
        process.exit(0);
      }
      if (!project) {
        console.error(`[ouroboros] subagent ${agent_id_short}: kb block found but no project (run inside a git repo)`);
        process.exit(0);
      }
      const binary = getBinaryPath();
      if (!binary) {
        console.error(`[ouroboros] subagent ${agent_id_short}: kb block found but ouroboros binary not available`);
        process.exit(0);
      }
      try {
        // Parse entries and inject metadata
        let entries = JSON.parse(json);
        if (!Array.isArray(entries)) {
          entries = [entries];
        }
        const source = {
          source: 'hook:subagent_stop',
          session_id: session_id || '',
        };
        if (agent_id) source.agent_id = agent_id;
        if (agent_type) source.agent_type = agent_type;
        entries.forEach(e => {
          e.metadata = { ...(e.metadata || {}), ...source };
        });
        const injectedJson = JSON.stringify(entries);

        const cmd = `"${binary}" put --stdin --project "${project}"`;
        const result = execSync(cmd, { input: injectedJson, timeout: 3000, encoding: 'utf-8' });
        const parsed = JSON.parse(result);
        const resultEntries = Array.isArray(parsed) ? parsed : [parsed];
        const ids = resultEntries.map(e => e.id).filter(id => id !== undefined);
        console.error(`[ouroboros] subagent ${agent_id_short}: persisted ${resultEntries.length} entries to ${project} [ids: ${ids.join(',')}]`);
        logHookEvent({ hook: 'subagent_stop', kind: 'persist', session_id, project, entries: resultEntries.length, ids });

        process.exit(0);
      } catch (execErr) {
        console.error(`[ouroboros] subagent ${agent_id_short}: put failed: ${execErr.message}`);
        logHookEvent({ hook: 'subagent_stop', kind: 'error', detail: execErr.message, session_id, project });
        process.exit(0);
      }
    }

    // Tier-2 check: already persisted
    if (matchesAnyPattern(message, ALREADY_PERSISTED_PATTERNS)) {
      console.error(`[ouroboros] subagent ${agent_id_short}: tier-2 self-claim detected (no kb block, but message references persistence)`);
      logHookEvent({ hook: 'subagent_stop', kind: 'nudge', session_id, project, reason: 'tier-2' });

      process.exit(0);
    }

    // Tier-1 check: decision language
    if (matchesAnyPattern(message, DECISION_PATTERNS)) {
      console.error(`[ouroboros] subagent ${agent_id_short}: tier-1 nudge fired (decision language present, no kb block)`);
      logHookEvent({ hook: 'subagent_stop', kind: 'nudge', session_id, project, reason: 'tier-1' });

      process.exit(0);
    }

    // Default: exit silently (exploratory output)
    logHookEvent({ hook: 'subagent_stop', kind: 'noop', session_id, project });

    process.exit(0);
  } catch (e) {
    // Graceful error handling
    process.exit(0);
  }
}

main();
