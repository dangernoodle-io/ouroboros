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
let workspaceRoot;
let projectDir;

test('setup: create temp stub dir, workspace, and HOME isolation', () => {
  const { execSync } = require('child_process');

  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-test-'));
  homeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-subagent-start-home-'));
  stubPath = path.join(tempDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);

  // Create workspace with .claude marker and a project dir with git repo
  workspaceRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-workspace-'));
  fs.mkdirSync(path.join(workspaceRoot, '.claude'));
  projectDir = path.join(workspaceRoot, 'ouroboros');
  fs.mkdirSync(projectDir);

  // Initialize git repo in projectDir so projectFromPath works
  try {
    execSync('git init', { cwd: projectDir, stdio: 'ignore' });
  } catch (e) {
    // Ignore git init errors
  }
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

test('subagent-start: stub query returns 3 rows → stdout has KB header + 3 lines WITHOUT contract block', () => {
  const input = JSON.stringify({ cwd: projectDir, session_id: 'sess-3rows-test' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  const stdout = result.stdout;
  assert(stdout.includes('[ouroboros]'));
  assert(stdout.includes('KB context'), 'KB context section header should be present');
  assert(stdout.includes('[note] sample one'));
  assert(stdout.includes('[decision] sample two'));
  assert(stdout.includes('[fact] sample three'));
  // Contract block should NOT be present for subagents
  assert(!stdout.includes('```kb'), 'contract block (```kb) should not appear for subagents');
  assert(!stdout.includes('persist any decisions/facts'), 'contract preamble should not appear for subagents');
});

test('subagent-start: stub query returns empty array and no items → exit 0, no stdout', () => {
  const input = JSON.stringify({ cwd: projectDir, session_id: 'sess-empty-test' });
  const result = runScript(input, { OUROBOROS_STUB_QUERY_EMPTY: '1', OUROBOROS_STUB_ITEMS_EMPTY: '1' });
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

test('subagent-start: KB summary still injected, contract block absent (regression)', () => {
  const input = JSON.stringify({ cwd: projectDir, session_id: 'sess-regression-test' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  const stdout = result.stdout;
  assert(stdout.includes('[ouroboros]'));
  assert(stdout.includes('KB context'), 'KB context section header should be present');
  assert(stdout.includes('[note] sample one'));
  assert(stdout.includes('[decision] sample two'));
  assert(stdout.includes('[fact] sample three'));
  // Contract block should be absent
  assert(!stdout.includes('```kb'), 'contract block should not appear');
  assert(!stdout.includes('persist any decisions/facts'), 'contract preamble should not appear');
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

test('subagent-start: cwd resolves project via projectFromPath, KB injected', () => {
  const gitRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), 'subagent-start-cwd-git-'));
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'subagent-start-cwd-home-'));
  try {
    fs.mkdirSync(path.join(gitRepoDir, '.git'));
    const cwdPath = path.join(gitRepoDir, 'src');
    fs.mkdirSync(cwdPath, { recursive: true });

    const input = JSON.stringify({
      session_id: 'cwd-resolve-test',
      agent_type: 'general',
      cwd: cwdPath
    });

    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);

    const stdout = result.stdout;
    assert(stdout.includes('[ouroboros]'), 'KB header should be injected');
    assert(stdout.includes('KB context'), 'KB context section header should be present');
    assert(stdout.includes('[note] sample one'), 'KB entries should be formatted');

    const logFile = path.join(testHomeDir, '.ouroboros', 'hooks.log');
    assert(fs.existsSync(logFile), 'hooks.log should exist');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    const fireEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.hook === 'subagent_start' && entry.kind === 'fire' && entry.project;
      } catch (e) { return false; }
    });
    assert(fireEvent, 'fire event should be logged with project from cwd');
  } finally {
    fs.rmSync(gitRepoDir, { recursive: true });
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('subagent-start: no cwd → no project → silent exit with events logged', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'subagent-start-no-cwd-home-'));
  try {
    const input = JSON.stringify({
      session_id: 'no-cwd-test',
      agent_type: 'general'
    });

    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.strictEqual(result.stdout.trim(), '', 'no KB context should be output when no cwd/project');

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
    assert(fireEvent, 'fire event should be logged even without project');
    assert(startEvent, 'subagent_start event should be logged even without project');
    assert(!fireEvent.includes('"project":"'), 'fire event should not have project field when cwd is missing');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

// OU-199: subagent context injection tests

test('subagent-start: prompt present → search-based KB results injected (relevant label)', () => {
  const input = JSON.stringify({
    cwd: projectDir,
    session_id: 'sess-ou199-prompt',
    agent_type: 'general',
    prompt: 'How does the authentication middleware work?',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  const stdout = result.stdout;
  assert(stdout.includes('[ouroboros] KB context (relevant):'), 'should show relevant label when prompt given');
  assert(stdout.includes('[note] sample one'), 'KB entries should be listed');
});

test('subagent-start: no prompt → recent KB results injected (recent label)', () => {
  const input = JSON.stringify({
    cwd: projectDir,
    session_id: 'sess-ou199-noprompt',
    agent_type: 'general',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  const stdout = result.stdout;
  assert(stdout.includes('[ouroboros] KB context (recent):'), 'should show recent label when no prompt');
  assert(stdout.includes('[note] sample one'), 'KB entries should be listed');
});

test('subagent-start: items query returns items → Open items section appears', () => {
  const input = JSON.stringify({
    cwd: projectDir,
    session_id: 'sess-ou199-items',
    agent_type: 'general',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  const stdout = result.stdout;
  assert(stdout.includes('[ouroboros] Open items'), 'Open items section should appear');
  assert(stdout.includes('OU-1'), 'item ID should appear');
  assert(stdout.includes('P2'), 'item priority should appear');
  assert(stdout.includes('test item'), 'item title should appear');
});

test('subagent-start: items empty → no Open items section', () => {
  const input = JSON.stringify({
    cwd: projectDir,
    session_id: 'sess-ou199-noitems',
    agent_type: 'general',
  });
  const result = runScript(input, { OUROBOROS_STUB_ITEMS_EMPTY: '1' });
  assert.strictEqual(result.status, 0);
  const stdout = result.stdout;
  assert(!stdout.includes('Open items'), 'Open items section should not appear when empty');
});

test('subagent-start: second fire within 60s → cooldown short-circuits (no context output)', () => {
  const gitRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), 'subagent-start-cooldown-'));
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'subagent-start-cooldown-home-'));
  const { execSync } = require('child_process');
  try {
    fs.mkdirSync(path.join(gitRepoDir, '.git'));
    try { execSync('git init', { cwd: gitRepoDir, stdio: 'ignore' }); } catch (e) {}

    const input = JSON.stringify({
      cwd: gitRepoDir,
      session_id: 'sess-cooldown-test',
      agent_type: 'general',
    });
    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };

    // First fire — should inject context
    const result1 = spawnSync('node', [SCRIPT_PATH], {
      input, encoding: 'utf-8', env: envVars, cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result1.status, 0);
    assert(result1.stdout.includes('[ouroboros]'), 'first fire should inject KB context');

    // Second fire in same session+project — cooldown should suppress
    const result2 = spawnSync('node', [SCRIPT_PATH], {
      input, encoding: 'utf-8', env: envVars, cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result2.status, 0);
    assert.strictEqual(result2.stdout.trim(), '', 'second fire within cooldown should not inject context');
  } finally {
    fs.rmSync(gitRepoDir, { recursive: true });
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('subagent-start: no project resolvable → fallback (silent exit)', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'subagent-start-fallback-home-'));
  try {
    const input = JSON.stringify({
      session_id: 'sess-fallback-test',
      agent_type: 'general',
      prompt: 'Does not matter, no project',
    });
    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input, encoding: 'utf-8', env: envVars, cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.strictEqual(result.stdout.trim(), '', 'no output when project cannot be resolved');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('subagent-start: only one fire+subagent_start event logged (no duplicate)', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'subagent-start-dedup-home-'));
  try {
    const input = JSON.stringify({
      session_id: 'sess-dedup-test',
      agent_type: 'general',
    });
    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input, encoding: 'utf-8', env: envVars, cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);

    const logFile = path.join(testHomeDir, '.ouroboros', 'hooks.log');
    if (fs.existsSync(logFile)) {
      const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n').filter(Boolean);
      const fireEvents = lines.filter(line => {
        try { const e = JSON.parse(line); return e.hook === 'subagent_start' && e.kind === 'fire'; } catch (e) { return false; }
      });
      assert.strictEqual(fireEvents.length, 1, 'exactly one fire event should be logged');
    }
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
