#!/usr/bin/env node
// Fixture: a stale, pre-OU-222 subagent-stop.js with the nudge PROSE
// reworded — same structural shape (a blocking process.exit(2) gated behind
// decision-language detection) as fixtures/stale-subagent-stop.js, but the
// printed message uses different wording than the original fixture. Used
// only by bootstrap.test.js to exercise the OU-258 wording-agnostic guard;
// never executed as a real hook.

const { readStdin, projectFromPath, logHookEvent, isSkippedAgentType, persistKbBlock } = require(__dirname + '/lib');

const DECISION_PATTERN = /\b(decided|rationale|architecture|trade-?off)\b/i;

async function main() {
  const input = await readStdin();
  let data;
  try {
    data = JSON.parse(input);
  } catch (e) {
    process.exit(0);
  }

  const message = data.last_assistant_message || '';
  if (message.length >= 80 && DECISION_PATTERN.test(message)) {
    console.log('please capture this reasoning before finishing up');
    process.exit(2);
  }

  process.exit(0);
}

main();
