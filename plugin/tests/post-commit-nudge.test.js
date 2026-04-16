const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

const SCRIPT_PATH = path.join(__dirname, '..', 'scripts', 'post-commit-nudge.js');

let homeDir;

test('setup: create HOME isolation', () => {
  homeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-commit-nudge-home-'));
});

function runScript(input, env = {}) {
  const envVars = { ...process.env, HOME: homeDir };
  Object.assign(envVars, env);
  return spawnSync('node', [SCRIPT_PATH], {
    input: input,
    encoding: 'utf-8',
    env: envVars,
    cwd: path.join(__dirname, '..'),
  });
}

test('post-commit-nudge: missing tool_input → exit 0, no stderr', () => {
  const input = JSON.stringify({ session_id: 'sess1234abcd' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stderr.trim(), '');
});

test('post-commit-nudge: command is not git commit → exit 0, no stderr', () => {
  const input = JSON.stringify({
    session_id: 'sess1234abcd',
    tool_input: { command: 'git status' }
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stderr.trim(), '');
});

test('post-commit-nudge: git commit with no cooldown → nudge on stderr', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-commit-nudge-work-'));
  try {
    // Remove cooldown file to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge'); } catch (e) {}

    const input = JSON.stringify({
      session_id: 'sess-nudge-test',
      tool_input: { command: 'git commit -m "test message"' }
    });

    const envVars = { ...process.env, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.match(result.stderr, /\/persist to save decisions/);
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('post-commit-nudge: git commit case-insensitive match', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-commit-nudge-case-'));
  try {
    // Remove cooldown file to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge'); } catch (e) {}

    const input = JSON.stringify({
      session_id: 'sess-case-test',
      tool_input: { command: 'GIT COMMIT -m "test"' }
    });

    const envVars = { ...process.env, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.match(result.stderr, /\/persist to save decisions/);
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('post-commit-nudge: fire event logged', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-commit-nudge-fire-'));
  try {
    // Remove cooldown file to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge'); } catch (e) {}

    const input = JSON.stringify({
      session_id: 'sess-fire-test',
      tool_input: { command: 'git commit -m "test"' }
    });

    const envVars = { ...process.env, HOME: testHomeDir };
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
    const fireEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.hook === 'post_commit_nudge' && entry.kind === 'fire';
      } catch (e) { return false; }
    });
    assert(fireEvent, 'should have a fire event with hook=post_commit_nudge');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('post-commit-nudge: nudge event logged when git commit detected', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-commit-nudge-nudge-'));
  try {
    // Remove cooldown file to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge'); } catch (e) {}

    const input = JSON.stringify({
      session_id: 'sess-nudge-event-test',
      tool_input: { command: 'git commit -m "test message"' }
    });

    const envVars = { ...process.env, HOME: testHomeDir };
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
    const nudgeEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'nudge';
      } catch (e) { return false; }
    });
    assert(nudgeEvent, 'should have a nudge event');
    const parsed = JSON.parse(nudgeEvent);
    assert.strictEqual(parsed.hook, 'post_commit_nudge');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('post-commit-nudge: nudge on stderr (no stdout)', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-commit-nudge-streams-'));
  try {
    // Remove cooldown file to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge'); } catch (e) {}

    const input = JSON.stringify({
      session_id: 'sess-streams-test',
      tool_input: { command: 'git commit -m "test"' }
    });

    const envVars = { ...process.env, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.strictEqual(result.stdout.trim(), '');
    assert.match(result.stderr, /\/persist to save decisions/);
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('cleanup: remove temp HOME', () => {
  if (homeDir && fs.existsSync(homeDir)) {
    fs.rmSync(homeDir, { recursive: true });
  }
});
