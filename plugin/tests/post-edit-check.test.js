const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

const SCRIPT_PATH = path.join(__dirname, '..', 'scripts', 'post-edit-check.js');
const FIXTURES_PATH = path.join(__dirname, 'fixtures');

let tempDir;
let stubPath;
let homeDir;

test('setup: create temp stub dir and HOME isolation', () => {
  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-edit-test-'));
  homeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-edit-home-'));
  stubPath = path.join(tempDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub-capture.sh'), stubPath);
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

test('post-edit-check: no file_path → exit 0, no stderr', () => {
  const input = JSON.stringify({ session_id: 'sess1234abcd' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stderr.trim(), '');
});

test('post-edit-check: file_path with no git repo but cwd in git repo → still processes', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'no-git-post-edit-'));
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'no-git-post-edit-home-'));
  try {
    const testFile = path.join(tmpDir, 'file.js');
    fs.writeFileSync(testFile, '');
    const input = JSON.stringify({ session_id: 'sess1234abcd', tool_input: { file_path: testFile } });

    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    // Even though file is not in a git repo, cwd is, so it should find matches
    assert.match(result.stderr, /KB refs file\.js:/);
  } finally {
    fs.rmSync(tmpDir, { recursive: true });
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('post-edit-check: short stem (<3 chars) → exit 0, no stderr', () => {
  const gitRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), 'post-edit-git-repo-'));
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'post-edit-short-home-'));
  try {
    fs.mkdirSync(path.join(gitRepoDir, '.git'));
    const srcDir = path.join(gitRepoDir, 'src');
    fs.mkdirSync(srcDir, { recursive: true });
    const testFile = path.join(srcDir, 'ab.js');
    fs.writeFileSync(testFile, '');
    const input = JSON.stringify({ session_id: 'sess1234abcd', tool_input: { file_path: testFile } });

    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.strictEqual(result.stderr.trim(), '');
  } finally {
    fs.rmSync(gitRepoDir, { recursive: true });
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('post-edit-check: KB match found → stderr contains nudge line with refs', () => {
  const gitRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), 'post-edit-git-kb-'));
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'post-edit-kb-home-'));
  try {
    fs.mkdirSync(path.join(gitRepoDir, '.git'));
    const srcDir = path.join(gitRepoDir, 'src');
    fs.mkdirSync(srcDir, { recursive: true });
    const testFile = path.join(srcDir, 'crud.js');
    fs.writeFileSync(testFile, '');
    const input = JSON.stringify({ session_id: 'sess1234abcd', tool_input: { file_path: testFile } });

    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.match(result.stderr, /KB refs crud\.js:/);
    assert.match(result.stderr, /check staleness/);
  } finally {
    fs.rmSync(gitRepoDir, { recursive: true });
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('post-edit-check: fire event logged', () => {
  const gitRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), 'post-edit-git-fire-'));
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-edit-fire-home-'));
  try {
    fs.mkdirSync(path.join(gitRepoDir, '.git'));
    const srcDir = path.join(gitRepoDir, 'src');
    fs.mkdirSync(srcDir, { recursive: true });
    const testFile = path.join(srcDir, 'index.js');
    fs.writeFileSync(testFile, '');
    const input = JSON.stringify({ session_id: 'sess-fire-test', tool_input: { file_path: testFile } });

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
    const fireEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.hook === 'post_edit_check' && entry.kind === 'fire';
      } catch (e) { return false; }
    });
    assert(fireEvent, 'should have a fire event with hook=post_edit_check');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
    fs.rmSync(gitRepoDir, { recursive: true });
  }
});

test('post-edit-check: nudge event logged when KB match found', () => {
  const gitRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), 'post-edit-git-nudge-'));
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-post-edit-nudge-home-'));
  try {
    fs.mkdirSync(path.join(gitRepoDir, '.git'));
    const srcDir = path.join(gitRepoDir, 'src');
    fs.mkdirSync(srcDir, { recursive: true });
    const testFile = path.join(srcDir, 'build.js');
    fs.writeFileSync(testFile, '');
    const input = JSON.stringify({ session_id: 'sess-nudge-test', tool_input: { file_path: testFile } });

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
    const nudgeEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'nudge';
      } catch (e) { return false; }
    });
    assert(nudgeEvent, 'should have a nudge event when KB matches');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
    fs.rmSync(gitRepoDir, { recursive: true });
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
