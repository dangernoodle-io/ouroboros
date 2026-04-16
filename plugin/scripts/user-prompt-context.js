#!/usr/bin/env node

const { execSync } = require('child_process');
const { readStdin, getProject, getBinaryPath, isWithinCooldown, touchFile, matchesAnyPattern, resolveProject } = require(__dirname + '/lib');

const COOLDOWN_MS = 1800000; // 30 minutes per project
const RESUME_COOLDOWN_MS = 0; // no cooldown for resume prompts
const MAX_ENTRIES = 10;
const MAX_SEARCH = 5;

// Patterns for low-signal or unrelated prompts
const SKIP_PATTERNS = [
  /^\s*(hi|hey|hello|thanks|thank you|yes|no|ok|okay|yep|nope|sure|got it)[\s.!?]*$/i,
  /^\s*tool loaded\.?\s*$/i,
  /^\s*(next|continue|more)\s*[.!?]*$/i,
];

// Resume patterns — user is starting/continuing work on a project
const RESUME_PATTERNS = [
  /\b(pick up|picking up|continue|continuing|resume|resuming)\b/i,
  /\b(back to|get back|return to)\b/i,
  /\b(where were we|where did we leave|what's the status|what is the status)\b/i,
  /\b(let's work on|lets work on|start on|starting on)\b/i,
  /\b(what's next|what is next|what do we need)\b/i,
  /\bbacklog\b/i,
];

function classifyPrompt(message) {
  if (!message || !message.trim()) return 'none';

  const trimmed = message.trim();

  // Slash command
  if (trimmed.startsWith('/')) return 'unrelated';

  // Skip patterns (low-signal acks, greetings, etc)
  if (matchesAnyPattern(trimmed, SKIP_PATTERNS)) return 'unrelated';

  // Length check: too short to be substantive
  if (trimmed.length < 15) return 'unrelated';

  // Resume patterns
  if (matchesAnyPattern(message, RESUME_PATTERNS)) return 'resume';

  return 'specific';
}

async function main() {
  try {
    const input = await readStdin();

    // Parse message from stdin JSON
    let message = '';
    let transcriptPath = '';
    let cwd = '';
    try {
      const json = JSON.parse(input);
      // UserPromptSubmit hook sends `prompt`, fallback to legacy `message` for testing
      message = json.prompt || json.message || '';
      transcriptPath = json.transcript_path || '';
      cwd = json.cwd || '';
    } catch (e) {
      // If not JSON, treat as empty
    }

    const intent = classifyPrompt(message);
    if (intent === 'none' || intent === 'unrelated') {
      process.exit(0);
    }

    // Determine project with fallback chain
    // TODO: extend resolveProject to support cwd hint (would require git-in-cwd logic)
    const hints = { message, transcriptPath };
    const project = resolveProject(hints);
    if (!project) {
      process.exit(0);
    }

    // Check cooldown per project (resume prompts bypass cooldown)
    const cooldownFile = `/tmp/.ouroboros-ctx-${project}`;
    if (intent !== 'resume' && isWithinCooldown(cooldownFile, COOLDOWN_MS)) {
      process.exit(0);
    }

    // Find the ouroboros binary
    const binary = getBinaryPath();
    if (!binary) {
      process.exit(0);
    }

    // Build CLI command based on intent
    let cmd;
    if (intent === 'resume') {
      cmd = `"${binary}" query --project "${project}" --limit ${MAX_ENTRIES}`;
    } else {
      const escaped = message.replace(/'/g, '').substring(0, 200);
      cmd = `"${binary}" query --project "${project}" --search '${escaped}' --limit ${MAX_SEARCH}`;
    }

    // Query KB via CLI mode
    let rows;
    try {
      const out = execSync(cmd, { timeout: 3000, encoding: 'utf-8' });
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
    const lines = [`[ouroboros] ${project} KB (${rows.length}):`];
    for (const row of rows) {
      lines.push(`  [${row.type}] ${row.title}`);
    }

    // For resume intent, also query backlog items
    if (intent === 'resume') {
      try {
        const itemsCmd = `"${binary}" items --project "${project}" --status open --limit 10`;
        const itemsOut = execSync(itemsCmd, { timeout: 3000, encoding: 'utf-8' });
        const items = JSON.parse(itemsOut);
        if (items && items.length > 0) {
          lines.push(`[ouroboros] ${project} backlog (${items.length}):`);
          for (const item of items) {
            lines.push(`  [${item.priority}] ${item.id}: ${item.title}`);
          }
        }
      } catch (e) {
        // Silently skip if items command not supported or no backlog
      }
    }

    process.stdout.write(lines.join('\n') + '\n');
    process.exit(0);
  } catch (e) {
    process.exit(0);
  }
}

main();
