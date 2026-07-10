#!/usr/bin/env node

const { readStdin, projectFromPath, logHookEvent, extractAllKbBlocks, queryKb } = require(__dirname + '/lib');

async function main() {
  try {
    const input = await readStdin();

    // Log fire event at entry, before any early-exit logic.
    // Enables grepping hooks.log for fire/block/allow/error sequences.
    logHookEvent({ hook: 'pre_compact', kind: 'fire' });

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

    // No kb-blocks in transcript: check if docs exist via session_id, else fall back to heuristic
    if (blocks.length === 0) {
      const sessionId = data.session_id || '';

      // If session_id exists, query ouroboros for persisted docs in this session
      if (sessionId) {
        const persistedCount = queryPersistedCount(project, sessionId);

        // If query succeeded and found docs, allow (persisted via tool path)
        if (persistedCount !== null && persistedCount > 0) {
          logHookEvent({
            hook: 'pre_compact',
            kind: 'allow',
            project,
            reason: 'persisted_via_tool',
            persisted_count: persistedCount,
            session_id: sessionId,
          });
          process.exit(0);
        }

        // Query failed or no docs found: fall through to heuristic
      }

      // Heuristic path: check for decision language. Compaction is never
      // blocked on this — it's an advisory-only observability signal.
      const decisionTurns = turns.filter(t => t.hasDecisionLanguage).length;
      const trigger = data.trigger || 'manual';
      const threshold = 3;

      logHookEvent({
        hook: 'pre_compact',
        kind: 'allow',
        project,
        reason: decisionTurns >= threshold ? 'decisions_unpersisted_advisory' : 'no_decisions',
        trigger,
        decision_turns: decisionTurns,
        threshold,
      });
      process.exit(0);
    }

    // kb-blocks present: precise session_id diffing
    const sessionId = data.session_id || '';
    if (!sessionId) {
      // No session_id to diff against — allow (can't compare without it)
      logHookEvent({
        hook: 'pre_compact',
        kind: 'allow',
        project,
        reason: 'no_session_id',
        block_count: blocks.length,
      });
      process.exit(0);
    }

    const persistedCount = queryPersistedCount(project, sessionId);

    if (persistedCount === null) {
      // Query failed — fail-open
      logHookEvent({
        hook: 'pre_compact',
        kind: 'allow',
        project,
        reason: 'query_error',
        block_count: blocks.length,
      });
      process.exit(0);
    }

    if (persistedCount >= blocks.length) {
      logHookEvent({
        hook: 'pre_compact',
        kind: 'allow',
        project,
        reason: 'all_persisted',
        block_count: blocks.length,
        persisted_count: persistedCount,
        session_id: sessionId,
      });
      process.exit(0);
    }

    // Some kb-blocks in this transcript weren't persisted. This is a real
    // signal worth logging, but compaction is never blocked on it — advisory
    // only.
    const unpersisted = blocks.length - persistedCount;
    logHookEvent({
      hook: 'pre_compact',
      kind: 'unpersisted_advisory',
      project,
      reason: 'unpersisted_blocks',
      block_count: blocks.length,
      persisted_count: persistedCount,
      unpersisted_count: unpersisted,
      session_id: sessionId,
    });
  } catch (e) {
    logHookEvent({ hook: 'pre_compact', kind: 'error', error: String(e) });
  }

  process.exit(0);
}

// queryPersistedCount returns the number of documents persisted for the given
// project and session_id, or null if the query fails (fail-open).
function queryPersistedCount(project, sessionId) {
  const rows = queryKb(project, { sessionId, limit: 500 });
  if (rows === null) return null;
  return rows.length;
}

main();
