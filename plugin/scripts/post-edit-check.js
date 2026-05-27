#!/usr/bin/env node

const path = require('path');
const crypto = require('crypto');
const { readStdin, projectFromPath, isWithinCooldown, touchFile, logHookEvent, queryKb } = require(__dirname + '/lib');

const COOLDOWN_MS = 600000; // 10 minutes per file

async function main() {
  try {
    const input = await readStdin();

    let filePaths = [];
    let session_id;
    try {
      const data = JSON.parse(input);
      session_id = data.session_id;
      // MultiEdit: tool_input.edits is an array of {file_path, ...}
      if (data.tool_input && Array.isArray(data.tool_input.edits)) {
        filePaths = data.tool_input.edits
          .map(e => e.file_path)
          .filter(p => typeof p === 'string' && p.length > 0);
      } else {
        const fp = (data.tool_input && data.tool_input.file_path) || '';
        if (fp) filePaths = [fp];
      }
    } catch (e) {
      process.exit(0);
    }

    // Use first file path for project/fire resolution
    const firstFilePath = filePaths[0] || '';
    let project = projectFromPath(firstFilePath);

    // Log fire event
    logHookEvent({ hook: 'post_edit_check', kind: 'fire', session_id, project });

    if (filePaths.length === 0) {
      logHookEvent({ hook: 'post_edit_check', kind: 'noop', session_id, project });
      process.exit(0);
    }

    if (!project) {
      logHookEvent({ hook: 'post_edit_check', kind: 'noop', session_id, project });
      process.exit(0);
    }

    // Process each file path
    let nudgeFired = false;
    for (const filePath of filePaths) {
      // Per-file cooldown
      const fileHash = crypto.createHash('md5').update(filePath).digest('hex').substring(0, 8);
      const cooldownFile = `/tmp/.ouroboros-stale-${fileHash}`;
      if (isWithinCooldown(cooldownFile, COOLDOWN_MS)) {
        continue;
      }

      // Extract basename stem (e.g., "crud" from "crud.go")
      const basename = path.basename(filePath);
      const stem = basename.replace(/\.[^.]+$/, '');
      if (!stem || stem.length < 3) {
        continue;
      }

      // Search KB for entries mentioning this file
      const escaped = stem.replace(/'/g, '');
      const rows = queryKb(project, { search: escaped, limit: 5 });
      if (!rows || rows.length === 0) {
        continue;
      }

      // Touch cooldown
      touchFile(cooldownFile);

      // Format and inject
      const titles = rows.map(r => `[${r.type}] ${r.title}`).join(', ');
      process.stderr.write(`[ouroboros] KB refs ${basename}: ${titles} — check staleness\n`);
      nudgeFired = true;
    }

    if (nudgeFired) {
      logHookEvent({ hook: 'post_edit_check', kind: 'nudge', session_id, project });
    } else {
      logHookEvent({ hook: 'post_edit_check', kind: 'noop', session_id, project });
    }
    process.exit(0);
  } catch (e) {
    process.exit(0);
  }
}

main();
