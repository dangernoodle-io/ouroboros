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
    // Remove per-project cooldown files to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-unknown'); } catch (e) {}

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
    // Remove per-project cooldown files to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-unknown'); } catch (e) {}

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
    // Remove per-project cooldown files to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-unknown'); } catch (e) {}

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
    // Remove per-project cooldown files to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-unknown'); } catch (e) {}

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
    // Remove per-project cooldown files to ensure test runs
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-unknown'); } catch (e) {}

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

test('post-commit-nudge: cooldown scoped per-project — project A cooldown does not suppress project B nudge', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-commit-nudge-per-proj-'));
  try {
    // Clean up any leftover cooldown files
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-project-a'); } catch (e) {}
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-project-b'); } catch (e) {}

    // Create project-a repo: testHomeDir/project-a/.git/ with a file inside
    const projectARoot = path.join(testHomeDir, 'project-a');
    fs.mkdirSync(projectARoot, { recursive: true });
    fs.mkdirSync(path.join(projectARoot, '.git'), { recursive: true });
    const projectAFile = path.join(projectARoot, 'test.txt');
    fs.writeFileSync(projectAFile, 'test');

    const inputA = JSON.stringify({
      session_id: 'sess-proj-a-test',
      cwd: projectAFile,
      tool_input: { command: 'git commit -m "test message"' }
    });

    const envVars = { ...process.env, HOME: testHomeDir };
    const resultA = spawnSync('node', [SCRIPT_PATH], {
      input: inputA,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(resultA.status, 0);
    assert.match(resultA.stderr, /\/persist to save decisions/, 'project-a should nudge');

    // Verify cooldown file was created for project-a
    assert(fs.existsSync('/tmp/.ouroboros-commit-nudge-project-a'), 'project-a cooldown should exist');

    // Create project-b repo: testHomeDir/project-b/.git/ with a file inside
    const projectBRoot = path.join(testHomeDir, 'project-b');
    fs.mkdirSync(projectBRoot, { recursive: true });
    fs.mkdirSync(path.join(projectBRoot, '.git'), { recursive: true });
    const projectBFile = path.join(projectBRoot, 'test.txt');
    fs.writeFileSync(projectBFile, 'test');

    const inputB = JSON.stringify({
      session_id: 'sess-proj-b-test',
      cwd: projectBFile,
      tool_input: { command: 'git commit -m "test message"' }
    });

    const resultB = spawnSync('node', [SCRIPT_PATH], {
      input: inputB,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(resultB.status, 0);
    assert.match(resultB.stderr, /\/persist to save decisions/, 'project-b should nudge despite project-a cooldown');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-project-a'); } catch (e) {}
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-project-b'); } catch (e) {}
  }
});

test('post-commit-nudge: project name sanitization replaces unsafe chars with hyphens', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-commit-nudge-sanitize-'));
  try {
    // Create a project directory with special chars in the name
    const unsafeProjectName = 'project@name#test';
    const expectedSanitized = 'project-name-test';
    const projectRoot = path.join(testHomeDir, unsafeProjectName);
    fs.mkdirSync(projectRoot, { recursive: true });
    fs.mkdirSync(path.join(projectRoot, '.git'), { recursive: true });
    const projectFile = path.join(projectRoot, 'test.txt');
    fs.writeFileSync(projectFile, 'test');

    // Clean up cooldown file with sanitized name
    const cooldownFile = `/tmp/.ouroboros-commit-nudge-${expectedSanitized}`;
    try { fs.unlinkSync(cooldownFile); } catch (e) {}

    const input = JSON.stringify({
      session_id: 'sess-sanitize-test',
      cwd: projectFile,
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
    assert.match(result.stderr, /\/persist to save decisions/, 'should nudge');

    // Verify cooldown file was created with sanitized name
    assert(fs.existsSync(cooldownFile), `cooldown file with sanitized name should exist at ${cooldownFile}`);
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
    try { fs.unlinkSync('/tmp/.ouroboros-commit-nudge-project-name-test'); } catch (e) {}
  }
});

test('cleanup: remove temp HOME', () => {
  if (homeDir && fs.existsSync(homeDir)) {
    fs.rmSync(homeDir, { recursive: true });
  }
});
