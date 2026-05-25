#!/usr/bin/env node

const { readStdin, projectFromPath, checkNudgePatterns, logHookEvent, readLastMainAssistantText, persistKbBlock } = require(__dirname + '/lib');

async function main() {
  try {
    const input = await readStdin();

    let data;
    try {
      data = JSON.parse(input);
    } catch (e) {
      process.exit(0);
    }

    // CRITICAL: avoid infinite loop when this hook causes the next turn.
    if (data.stop_hook_active === true) {
      process.exit(0);
    }

    const transcriptPath = data.transcript_path || '';
    if (!transcriptPath) {
      process.exit(0);
    }

    // Determine project, prefer cwd-based resolution
    const cwd = data.cwd || '';
    let project = null;
    if (cwd) {
      project = projectFromPath(cwd);
    }

    const sessionId = data.session_id;
    logHookEvent({ hook: 'stop', kind: 'fire', session_id: sessionId, project });

    let message = readLastMainAssistantText(transcriptPath);
    if (!message || message.length < 80) {
      process.exit(0);
    }

    // Truncate to 5000 chars for matching
    message = message.substring(0, 5000);

    const sessionShort = (data.session_id || 'main').substring(0, 8);

    // KB block extraction: try to extract and persist fenced kb block
    const kbResult = persistKbBlock(message, {
      label: 'main',
      idShort: sessionShort,
      hookName: 'stop',
      sessionId,
      project,
      extraMeta: {},
    });
    if (kbResult.handled) {
      process.exit(kbResult.exitCode);
    }

    const nudge = checkNudgePatterns(message, {
      label: 'main',
      idShort: sessionShort,
      sessionId,
      project,
      hookName: 'stop',
    });
    if (nudge) {
      process.stdout.write(JSON.stringify(nudge) + '\n');
      process.exit(2);
    }

    // Default: exit silently (exploratory output)
    logHookEvent({ hook: 'stop', kind: 'noop', session_id: sessionId, project });
    process.exit(0);
  } catch (e) {
    process.exit(0);
  }
}

main();
