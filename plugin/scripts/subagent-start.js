#!/usr/bin/env node

const { execSync } = require('child_process');
const { readStdin, projectFromPath, getBinaryPath, formatContextLines, logHookEvent, isSkippedAgentType } = require(__dirname + '/lib');

const MAX_ENTRIES = 8;

async function main() {
  try {
    // Read stdin JSON
    const input = await readStdin();
    let data = {};
    try {
      data = JSON.parse(input);
    } catch (e) {
      // If not JSON, continue with empty data
    }

    const agent_type = data.agent_type || '';
    const session_id = data.session_id;

    // Determine project, prefer cwd-based resolution
    const cwd = data.cwd || '';
    let project = null;
    if (cwd) {
      project = projectFromPath(cwd);
    }

    // Log fire event immediately, before any early exits
    logHookEvent({ hook: 'subagent_start', kind: 'fire', session_id, project });

    // Log subagent_start event unconditionally, before any early exits
    logHookEvent({ hook: 'subagent_start', kind: 'subagent_start', session_id, agent_type });

    // Early exit: skip list
    if (isSkippedAgentType(agent_type)) {
      process.exit(0);
    }

    // Early exit: no project
    if (!project) {
      process.exit(0);
    }

    // Find the ouroboros binary
    const binary = getBinaryPath();
    if (!binary) {
      process.exit(0);
    }

    // Query KB: fetch recent entries for this project
    let rows;
    try {
      const cmd = `"${binary}" query --project "${project}" --limit ${MAX_ENTRIES}`;
      const out = execSync(cmd, { timeout: 3000, encoding: 'utf-8' });
      rows = JSON.parse(out);
    } catch (e) {
      process.exit(0);
    }

    // Silent exit if no KB entries
    if (!rows || rows.length === 0) {
      process.exit(0);
    }

    // Format and output context with contract block
    const lines = formatContextLines(project, rows);
    if (lines.length === 0) {
      process.exit(0);
    }

    for (const line of lines) {
      process.stdout.write(line + '\n');
    }

    process.exit(0);
  } catch (e) {
    process.exit(0);
  }
}

main();
