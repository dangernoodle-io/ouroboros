#!/usr/bin/env node

const { execSync } = require('child_process');
const { readStdin, projectFromPath, logHookEvent, extractAllKbBlocks, getBinaryPath } = require(__dirname + '/lib');

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

      // Heuristic path: check for decision language
      const decisionTurns = turns.filter(t => t.hasDecisionLanguage).length;
      const trigger = data.trigger || 'manual';
      const threshold = 3;

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

    const unpersisted = blocks.length - persistedCount;
    const reason = `[ouroboros] ${unpersisted} of ${blocks.length} kb-blocks unpersisted — persist before compacting`;
    process.stdout.write(JSON.stringify({ decision: 'block', reason }));
    logHookEvent({
      hook: 'pre_compact',
      kind: 'block',
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
  const binary = getBinaryPath();
  if (!binary) {
    return null;
  }

  try {
    const cmd = `"${binary}" query --project "${project}" --session-id "${sessionId}" --limit 500`;
    const output = execSync(cmd, { timeout: 3000, encoding: 'utf-8' });
    const parsed = JSON.parse(output.trim());
    if (!Array.isArray(parsed)) return null;
    return parsed.length;
  } catch (e) {
    return null;
  }
}

main();
