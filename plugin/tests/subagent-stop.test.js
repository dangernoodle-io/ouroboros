const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

const SCRIPT_PATH = path.join(__dirname, '..', 'scripts', 'subagent-stop.js');
const FIXTURES_PATH = path.join(__dirname, 'fixtures');

let tempDir;
let stubPath;

test('setup: create temp stub dir', () => {
  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-test-'));
  stubPath = path.join(tempDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);
});

function runScript(input, env = {}) {
  const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}` };
  Object.assign(envVars, env);
  return spawnSync('node', [SCRIPT_PATH], {
    input: input,
    encoding: 'utf-8',
    env: envVars,
    cwd: path.join(__dirname, '..'),
  });
}

test('subagent-stop: skip-list agent_type → exit 0, no stdout', () => {
  const input = JSON.stringify({
    agent_type: 'Explore',
    agent_id: 'abc12345678',
    last_assistant_message: 'Some long message with many words and content that exceeds minimum length requirements for the script to process properly and continue with checking patterns',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('subagent-stop: short message (<80 chars) → exit 0, no stdout', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'abc12345678',
    last_assistant_message: 'short',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('subagent-stop: kb block + stub put succeeds → log includes count, project, ids, agent_id', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'abc12345678',
    last_assistant_message: 'This is a long message with enough content to pass the minimum length check and then includes a kb block at the end:\n```kb\n[{"type":"decision","title":"adopt cobra"}]\n```',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /persisted 1 entries/);
  assert.match(result.stdout, /\[ids: 1\]/);
  assert(result.stdout.includes('abc12345'));
});

test('subagent-stop: kb block + stub put fails → log says put failed', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'def87654321',
    last_assistant_message: 'This is a long message with enough content to pass the minimum length check and includes a kb block:\n```kb\n[{"type":"fact"}]\n```',
  });
  const result = runScript(input, { OUROBOROS_STUB_PUT_FAIL: '1' });
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /put failed/);
  assert(result.stdout.includes('def87654'));
});

test('subagent-stop: kb block with malformed JSON → logs parse error, does NOT fall through', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'xyz99999999',
    last_assistant_message: 'Long message to pass minimum length with malformed kb block:\n```kb\n{invalid json}\n```\nAnd we decided to adopt approach X',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /kb block JSON parse error/);
  assert(!result.stdout.includes('tier-1'));
  assert(!result.stdout.includes('persisted'));
});

test('subagent-stop: no kb block + tier-2 self-claim → logs tier-2 detection', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'abc12345678',
    last_assistant_message: 'This is a long message that mentions the knowledge base which is a tier-2 pattern and should be logged as a self-claim',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /tier-2 self-claim/);
  assert(result.stdout.includes('abc12345'));
});

test('subagent-stop: no kb block + tier-1 decision language → tier-1 nudge log', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'abc12345678',
    last_assistant_message: 'This is a long message with enough content where we decided to adopt a new architecture for the system',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /tier-1 nudge fired/);
  assert(result.stdout.includes('abc12345'));
});

test('subagent-stop: message with no kb block + neutral content → exit 0, no stdout', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'abc12345678',
    last_assistant_message: 'This is just a simple neutral message that talks about how the weather is nice and contains no decision language or kb blocks',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('cleanup: remove temp stub dir', () => {
  if (tempDir && fs.existsSync(tempDir)) {
    fs.rmSync(tempDir, { recursive: true });
  }
});
