#!/usr/bin/env node

const { readStdin, projectFromPath, logHookEvent, isSkippedAgentType, persistKbBlock } = require(__dirname + '/lib');

async function main() {
  try {
    const input = await readStdin();

    // Parse JSON
    let data;
    try {
      data = JSON.parse(input);
    } catch (e) {
      process.exit(0);
    }

    const agent_id = data.agent_id || '';
    const agent_type = data.agent_type || '';
    const session_id = data.session_id;
    let message = data.last_assistant_message || '';

    // Determine project, prefer cwd-based resolution
    const cwd = data.cwd || '';
    let project = null;
    if (cwd) {
      project = projectFromPath(cwd);
    }

    // Log fire event
    logHookEvent({ hook: 'subagent_stop', kind: 'fire', session_id, project });

    // Log subagent_stop event unconditionally, before any early exits
    const excerpt = (message || '').substring(0, 120).replace(/\n/g, ' ');
    logHookEvent({ hook: 'subagent_stop', kind: 'subagent_stop', session_id, agent_id, agent_type, last_message_excerpt: excerpt });

    // Early exit: skip list
    if (isSkippedAgentType(agent_type)) {
      process.exit(0);
    }

    // Early exit: empty or too short message
    if (!message || typeof message !== 'string' || message.length < 80) {
      process.exit(0);
    }

    // Truncate to 5000 chars for matching
    message = message.substring(0, 5000);

    const agent_id_short = agent_id.substring(0, 8);

    // KB block extraction: try to extract and persist fenced kb block
    const extraMeta = {};
    if (agent_id) extraMeta.agent_id = agent_id;
    if (agent_type) extraMeta.agent_type = agent_type;
    const kbResult = persistKbBlock(message, {
      label: 'subagent',
      idShort: agent_id_short,
      hookName: 'subagent_stop',
      sessionId: session_id,
      project,
      extraMeta,
    });
    if (kbResult.handled) {
      process.exit(kbResult.exitCode);
    }

    // No decision-language nudge here (unlike stop.js): a subagent's final
    // message IS its return value to the orchestrator. A blocking (exit 2)
    // nudge forces the subagent to take another turn to address it, and that
    // meta-acknowledgement turn overwrites the real report as the "final"
    // message — destroying the report the caller actually needs (OU-222).
    // Persistence nudging is the main session's job (see stop.js); a subagent
    // that deliberately emits a ```kb block still gets it persisted above.

    // Default: exit silently (exploratory output)
    logHookEvent({ hook: 'subagent_stop', kind: 'noop', session_id, project });

    process.exit(0);
  } catch (e) {
    // Graceful error handling
    process.exit(0);
  }
}

main();
