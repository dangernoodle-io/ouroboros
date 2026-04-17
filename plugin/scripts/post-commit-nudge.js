#!/usr/bin/env node

const { readStdin, projectFromPath, isWithinCooldown, touchFile, logHookEvent } = require(__dirname + '/lib');

const COOLDOWN_MS = 300000; // 5 minutes

function getCooldownFile(project) {
  if (!project) {
    return '/tmp/.ouroboros-commit-nudge-unknown';
  }
  // Sanitize project name: replace path-unsafe characters with hyphens
  const sanitized = project.replace(/[^a-zA-Z0-9._-]/g, '-');
  return `/tmp/.ouroboros-commit-nudge-${sanitized}`;
}

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
    const cooldownFile = getCooldownFile(project);
    if (isWithinCooldown(cooldownFile, COOLDOWN_MS)) {
      logHookEvent({ hook: 'post_commit_nudge', kind: 'noop', session_id, project });
      process.exit(0);
    }

    // Touch the cooldown file
    touchFile(cooldownFile);

    // Write to stderr
    console.error('[ouroboros] /persist to save decisions');
    logHookEvent({ hook: 'post_commit_nudge', kind: 'nudge', session_id, project });

    process.exit(0);
  } catch (e) {
    // Graceful error handling - exit silently
    process.exit(0);
  }
}

main();
