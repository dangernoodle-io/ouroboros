#!/usr/bin/env node

const path = require('path');
const crypto = require('crypto');
const { execSync } = require('child_process');
const { readStdin, getProject, getBinaryPath, isWithinCooldown, touchFile } = require(__dirname + '/lib');

const COOLDOWN_MS = 600000; // 10 minutes per file

async function main() {
  try {
    const input = await readStdin();

    let filePath = '';
    try {
      const data = JSON.parse(input);
      filePath = (data.tool_input && data.tool_input.file_path) || '';
    } catch (e) {
      process.exit(0);
    }

    if (!filePath) {
      process.exit(0);
    }

    // Per-file cooldown
    const fileHash = crypto.createHash('md5').update(filePath).digest('hex').substring(0, 8);
    const cooldownFile = `/tmp/.ouroboros-stale-${fileHash}`;
    if (isWithinCooldown(cooldownFile, COOLDOWN_MS)) {
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

    // Extract basename stem (e.g., "crud" from "crud.go")
    const basename = path.basename(filePath);
    const stem = basename.replace(/\.[^.]+$/, '');
    if (!stem || stem.length < 3) {
      // Too short to be meaningful for search
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
      process.exit(0);
    }

    if (!rows || rows.length === 0) {
      process.exit(0);
    }

    // Touch cooldown
    touchFile(cooldownFile);

    // Format and inject
    const titles = rows.map(r => `[${r.type}] ${r.title}`).join(', ');
    process.stdout.write(`[ouroboros] KB refs ${basename}: ${titles} — check staleness\n`);
    process.exit(0);
  } catch (e) {
    process.exit(0);
  }
}

main();
