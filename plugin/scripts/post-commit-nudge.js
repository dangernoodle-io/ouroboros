#!/usr/bin/env node

const { readStdin, isWithinCooldown, touchFile } = require(__dirname + '/lib');

const COOLDOWN_FILE = '/tmp/.ouroboros-commit-nudge';
const COOLDOWN_MS = 300000; // 5 minutes

async function main() {
  try {
    // Read all stdin
    const input = await readStdin();

    // Parse JSON
    let data;
    try {
      data = JSON.parse(input);
    } catch (e) {
      // If JSON parse fails, exit silently
      process.exit(0);
    }

    // Check if this is a git commit command
    const command = data.tool_input?.command || '';
    if (!command.toLowerCase().includes('git commit')) {
      process.exit(0);
    }

    // Check cooldown
    if (isWithinCooldown(COOLDOWN_FILE, COOLDOWN_MS)) {
      process.exit(0);
    }

    // Touch the cooldown file
    touchFile(COOLDOWN_FILE);

    // Write to stdout
    console.log('[ouroboros] /persist to save decisions');

    process.exit(0);
  } catch (e) {
    // Graceful error handling - exit silently
    process.exit(0);
  }
}

main();
