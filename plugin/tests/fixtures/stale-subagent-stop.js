#!/usr/bin/env node
// Fixture: a stale, pre-OU-222 subagent-stop.js. Unlike the current clean
// script, this copy nudges the subagent when its final message matches
// decision-language patterns and no ```kb block is present — the subagent
// then answers the nudge, overwriting its real report to the caller
// (the OU-222 report-destruction bug). Used only by bootstrap.test.js /
// subagent-stop.test.js to exercise the OU-254 stale-hook detection guard;
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
    console.log('tier-1 nudge fired (decision language present, no kb block)');
    process.exit(2);
  }

  process.exit(0);
}

main();
