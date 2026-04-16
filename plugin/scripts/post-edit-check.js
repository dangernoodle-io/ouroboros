#!/usr/bin/env node

const path = require('path');
const crypto = require('crypto');
const { execSync } = require('child_process');
const { readStdin, getProject, projectFromPath, getBinaryPath, isWithinCooldown, touchFile, logHookEvent } = require(__dirname + '/lib');

const COOLDOWN_MS = 600000; // 10 minutes per file

async function main() {
  try {
    const input = await readStdin();

    let filePath = '';
    let session_id;
    try {
      const data = JSON.parse(input);
      filePath = (data.tool_input && data.tool_input.file_path) || '';
      session_id = data.session_id;
    } catch (e) {
      process.exit(0);
    }

    // Determine project for fire event, prefer path-based resolution
    let project = projectFromPath(filePath);
    if (!project) {
      project = getProject();
    }

    // Log fire event
    logHookEvent({ hook: 'post_edit_check', kind: 'fire', session_id, project });

    if (!filePath) {
      logHookEvent({ hook: 'post_edit_check', kind: 'noop', session_id, project });
      process.exit(0);
    }

    // Per-file cooldown
    const fileHash = crypto.createHash('md5').update(filePath).digest('hex').substring(0, 8);
    const cooldownFile = `/tmp/.ouroboros-stale-${fileHash}`;
    if (isWithinCooldown(cooldownFile, COOLDOWN_MS)) {
      logHookEvent({ hook: 'post_edit_check', kind: 'noop', session_id, project });
      process.exit(0);
    }

    if (!project) {
      logHookEvent({ hook: 'post_edit_check', kind: 'noop', session_id, project });
      process.exit(0);
    }

    // Find the ouroboros binary
    const binary = getBinaryPath();
    if (!binary) {
      process.exit(0);
    }

    // Extract basename stem (e.g., "crud" from "crud.go")
    const basename = path.basename(filePath);
    const stem = basename.replace(/\.[^.]+$/, '');
    if (!stem || stem.length < 3) {
      // Too short to be meaningful for search
      logHookEvent({ hook: 'post_edit_check', kind: 'noop', session_id, project });
      process.exit(0);
    }

    // Search KB for entries mentioning this file
    let rows;
    try {
      const escaped = stem.replace(/'/g, '');
      const out = execSync(
        `"${binary}" query --project "${project}" --search '${escaped}' --limit 5`,
        { timeout: 3000, encoding: 'utf-8' }
      );
      rows = JSON.parse(out);
    } catch (e) {
      logHookEvent({ hook: 'post_edit_check', kind: 'noop', session_id, project });
      process.exit(0);
    }

    if (!rows || rows.length === 0) {
      logHookEvent({ hook: 'post_edit_check', kind: 'noop', session_id, project });
      process.exit(0);
    }

    // Touch cooldown
    touchFile(cooldownFile);

    // Format and inject
    const titles = rows.map(r => `[${r.type}] ${r.title}`).join(', ');
    process.stderr.write(`[ouroboros] KB refs ${basename}: ${titles} — check staleness\n`);
    logHookEvent({ hook: 'post_edit_check', kind: 'nudge', session_id, project });
    process.exit(0);
  } catch (e) {
    process.exit(0);
  }
}

main();
