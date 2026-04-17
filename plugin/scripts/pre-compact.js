#!/usr/bin/env node

const { readStdin, projectFromPath, logHookEvent, extractAllKbBlocks } = require(__dirname + '/lib');

async function main() {
  try {
    const input = await readStdin();

    let data;
    try {
      data = JSON.parse(input);
    } catch (e) {
      logHookEvent({ hook: 'pre_compact', kind: 'parse_error' });
      process.exit(0);
    }

    const transcriptPath = data.transcript_path || '';
    if (!transcriptPath) {
      logHookEvent({ hook: 'pre_compact', kind: 'skip', reason: 'no_transcript' });
      process.exit(0);
    }

    const cwd = data.cwd || '';
    const project = projectFromPath(cwd);
    if (!project) {
      logHookEvent({ hook: 'pre_compact', kind: 'skip', reason: 'no_project' });
      process.exit(0);
    }

    const { blocks, turns } = extractAllKbBlocks(transcriptPath);

    // If KB blocks are present, allow compaction
    if (blocks.length > 0) {
      logHookEvent({
        hook: 'pre_compact',
        kind: 'allow',
        project,
        reason: 'blocks_present',
        block_count: blocks.length,
      });
      process.exit(0);
    }

    // Count turns with decision language
    const decisionTurns = turns.filter(t => t.hasDecisionLanguage).length;

    // Determine threshold based on trigger type
    const trigger = data.trigger || 'manual';
    const threshold = trigger === 'auto' ? 3 : 1;

    if (decisionTurns >= threshold) {
      const reason = '[ouroboros] unpersisted decisions detected — emit ```kb``` blocks for the key decisions from this session before compacting';
      process.stdout.write(JSON.stringify({ decision: 'block', reason }));
      logHookEvent({
        hook: 'pre_compact',
        kind: 'block',
        project,
        trigger,
        decision_turns: decisionTurns,
        threshold,
      });
      process.exit(0);
    }

    logHookEvent({
      hook: 'pre_compact',
      kind: 'allow',
      project,
      reason: 'no_decisions',
      trigger,
      decision_turns: decisionTurns,
    });
  } catch (e) {
    logHookEvent({ hook: 'pre_compact', kind: 'error', error: String(e) });
  }

  process.exit(0);
}

main();
