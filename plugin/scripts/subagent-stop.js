#!/usr/bin/env node

const { execSync } = require('child_process');
const { readStdin, getProject, getBinaryPath, extractKbBlock, matchesAnyPattern, SKIP_AGENT_TYPES } = require(__dirname + '/lib');

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
    let message = data.last_assistant_message || '';

    // Early exit: skip list
    if (SKIP_AGENT_TYPES.includes(agent_type)) {
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
        console.log(`[ouroboros] subagent ${agent_id_short}: kb block JSON parse error: ${parseErr.message}`);
        process.exit(0);
      }
      const project = getProject();
      if (!project) {
        console.log(`[ouroboros] subagent ${agent_id_short}: kb block found but no project (run inside a git repo)`);
        process.exit(0);
      }
      const binary = getBinaryPath();
      if (!binary) {
        console.log(`[ouroboros] subagent ${agent_id_short}: kb block found but ouroboros binary not available`);
        process.exit(0);
      }
      try {
        const cmd = `"${binary}" put --stdin --project "${project}"`;
        const result = execSync(cmd, { input: json, timeout: 3000, encoding: 'utf-8' });
        const parsed = JSON.parse(result);
        const entries = Array.isArray(parsed) ? parsed : [parsed];
        const ids = entries.map(e => e.id).filter(id => id !== undefined).join(',');
        console.log(`[ouroboros] subagent ${agent_id_short}: persisted ${entries.length} entries to ${project} [ids: ${ids}]`);
        process.exit(0);
      } catch (execErr) {
        console.log(`[ouroboros] subagent ${agent_id_short}: put failed: ${execErr.message}`);
        process.exit(0);
      }
    }

    // Tier-2 check: already persisted
    if (matchesAnyPattern(message, ALREADY_PERSISTED_PATTERNS)) {
      console.log(`[ouroboros] subagent ${agent_id_short}: tier-2 self-claim detected (no kb block, but message references persistence)`);
      process.exit(0);
    }

    // Tier-1 check: decision language
    if (matchesAnyPattern(message, DECISION_PATTERNS)) {
      console.log(`[ouroboros] subagent ${agent_id_short}: tier-1 nudge fired (decision language present, no kb block)`);
      process.exit(0);
    }

    // Default: exit silently (exploratory output)
    process.exit(0);
  } catch (e) {
    // Graceful error handling
    process.exit(0);
  }
}

main();
