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
let homeDir;

test('setup: create temp stub dir and HOME isolation', () => {
  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-test-'));
  homeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-subagent-start-home-'));
  stubPath = path.join(tempDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);
});

function runScript(input, env = {}) {
  const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: homeDir };
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

test('subagent-start: agent_type "Explore" in skip list → exit 0, no stdout, but fire+subagent_start events logged', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-subagent-start-skip-home-'));
  try {
    const input = JSON.stringify({ agent_type: 'Explore', session_id: 'skip-test-sess' });
    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.strictEqual(result.stdout.trim(), '');

    const logFile = path.join(testHomeDir, '.ouroboros', 'hooks.log');
    assert(fs.existsSync(logFile), 'hooks.log should exist even for skip-list agent_type');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    const fireEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.hook === 'subagent_start' && entry.kind === 'fire';
      } catch (e) { return false; }
    });
    const startEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'subagent_start';
      } catch (e) { return false; }
    });
    assert(fireEvent, 'fire event should be logged even for skip-list agent_type');
    assert(startEvent, 'subagent_start event should be logged even for skip-list agent_type');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('subagent-start: agent_type "knowledge-explorer" in skip list → exit 0, no stdout', () => {
  const input = JSON.stringify({ agent_type: 'knowledge-explorer' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('subagent-start: fire event logged with hook:subagent_start', () => {
  const input = JSON.stringify({ agent_type: 'general' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);

  const logFile = path.join(homeDir, '.ouroboros', 'hooks.log');
  assert(fs.existsSync(logFile), 'hooks.log should exist');

  const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
  const fireEvent = lines.find(line => {
    try {
      const entry = JSON.parse(line);
      return entry.hook === 'subagent_start' && entry.kind === 'fire';
    } catch (e) { return false; }
  });
  assert(fireEvent, 'should have a fire event with hook=subagent_start');
});

test('subagent-start: subagent_start event logged with agent_type', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-subagent-start-event-home-'));
  try {
    const input = JSON.stringify({ agent_type: 'general', session_id: 'event-test-sess' });

    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);

    const logFile = path.join(testHomeDir, '.ouroboros', 'hooks.log');
    assert(fs.existsSync(logFile), 'hooks.log should exist');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    const startEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'subagent_start';
      } catch (e) { return false; }
    });
    assert(startEvent, 'subagent_start event should be logged unconditionally');
    const parsed = JSON.parse(startEvent);
    assert.strictEqual(parsed.agent_type, 'general');
    assert.strictEqual(parsed.session_id, 'event-test-sess');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('subagent-start: KB context still injected to stdout (regression)', () => {
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

test('subagent-start: plugin-qualified knowledge-explorer agent skipped (regression test)', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-plugin-qualified-skip-'));
  try {
    const input = JSON.stringify({ agent_type: 'ouroboros-mcp:knowledge-explorer', session_id: 'plugin-skip-test' });
    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.strictEqual(result.stdout.trim(), '', 'no KB context should be injected for skipped plugin agent');

    const logFile = path.join(testHomeDir, '.ouroboros', 'hooks.log');
    assert(fs.existsSync(logFile), 'hooks.log should exist');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    const fireEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.hook === 'subagent_start' && entry.kind === 'fire';
      } catch (e) { return false; }
    });
    const startEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'subagent_start';
      } catch (e) { return false; }
    });
    assert(fireEvent, 'fire event should be logged');
    assert(startEvent, 'subagent_start event should be logged');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('cleanup: remove temp stub dir and HOME', () => {
  if (tempDir && fs.existsSync(tempDir)) {
    fs.rmSync(tempDir, { recursive: true });
  }
  if (homeDir && fs.existsSync(homeDir)) {
    fs.rmSync(homeDir, { recursive: true });
  }
});
