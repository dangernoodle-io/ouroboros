const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

const SCRIPT_PATH = path.join(__dirname, '..', 'scripts', 'subagent-start.js');
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

test('subagent-start: stub query returns 3 rows → stdout has KB header + 3 indented lines + contract block', () => {
  const input = JSON.stringify({});
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  const stdout = result.stdout;
  assert(stdout.includes('[ouroboros]'));
  assert(stdout.includes('KB (3)'));
  assert(stdout.includes('[note] sample one'));
  assert(stdout.includes('[decision] sample two'));
  assert(stdout.includes('[fact] sample three'));
  assert(stdout.includes('```kb'));
  assert(stdout.includes('persist any decisions/facts'));
});

test('subagent-start: stub query returns empty array → exit 0, no stdout', () => {
  const input = JSON.stringify({});
  const result = runScript(input, { OUROBOROS_STUB_QUERY_EMPTY: '1' });
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('subagent-start: agent_type "Explore" in skip list → exit 0, no stdout', () => {
  const input = JSON.stringify({ agent_type: 'Explore' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('subagent-start: agent_type "knowledge-explorer" in skip list → exit 0, no stdout', () => {
  const input = JSON.stringify({ agent_type: 'knowledge-explorer' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('cleanup: remove temp stub dir', () => {
  if (tempDir && fs.existsSync(tempDir)) {
    fs.rmSync(tempDir, { recursive: true });
  }
});
