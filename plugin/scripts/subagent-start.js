#!/usr/bin/env node

const { execSync } = require('child_process');
const { readStdin, getProject, getBinaryPath, formatContextLines, SKIP_AGENT_TYPES } = require(__dirname + '/lib');

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

    // Early exit: skip list
    const agent_type = data.agent_type || '';
    if (SKIP_AGENT_TYPES.includes(agent_type)) {
      process.exit(0);
    }

    // Determine project from cwd
    const project = getProject();
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
