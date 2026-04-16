#!/usr/bin/env node

const { readStdin, projectFromPath, getProject, isWithinCooldown, touchFile, logHookEvent } = require(__dirname + '/lib');

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
    const session_id = data.session_id;
    logHookEvent({ hook: 'post_commit_nudge', kind: 'fire', session_id, project });

    // Check if this is a git commit command
    const command = data.tool_input?.command || '';
    if (!command.toLowerCase().includes('git commit')) {
      logHookEvent({ hook: 'post_commit_nudge', kind: 'noop', session_id, project });
      process.exit(0);
    }

    // Check cooldown
    if (isWithinCooldown(COOLDOWN_FILE, COOLDOWN_MS)) {
      logHookEvent({ hook: 'post_commit_nudge', kind: 'noop', session_id, project });
      process.exit(0);
    }

    // Touch the cooldown file
    touchFile(COOLDOWN_FILE);

    // Write to stderr
    console.log('[ouroboros] /persist to save decisions');
    logHookEvent({ hook: 'post_commit_nudge', kind: 'nudge', session_id, project });

    process.exit(0);
  } catch (e) {
    // Graceful error handling - exit silently
    process.exit(0);
  }
}

main();
