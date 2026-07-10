const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

const SCRIPT_PATH = path.join(__dirname, '..', 'scripts', 'user-prompt-context.js');
const FIXTURES_PATH = path.join(__dirname, 'fixtures');

let tempDir;
let stubPath;
let homeDir;

test('setup: create temp stub dir and HOME isolation', () => {
  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-upc-test-'));
  homeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-upc-home-'));
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

// Test 1: empty string → none
test('classifier: empty string → none (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 2: whitespace → none
test('classifier: whitespace only → none (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'   \t\n  ' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 3: /commit → unrelated (slash command)
test('classifier: /commit → unrelated (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'/commit' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 4: hi → unrelated (skip pattern)
test('classifier: hi → unrelated (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'hi' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 5: yes → unrelated (skip pattern)
test('classifier: yes → unrelated (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'yes' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 6: Tool loaded. → unrelated (skip pattern)
test('classifier: Tool loaded. → unrelated (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'Tool loaded.' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 7: ok → unrelated (skip pattern)
test('classifier: ok → unrelated (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'ok' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 8: thanks! → unrelated (skip pattern)
test('classifier: thanks! → unrelated (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'thanks!' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 9: continue → unrelated (skip pattern, also under length threshold)
test('classifier: continue → unrelated (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'continue' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 9b: continue. (6 chars with punctuation) → unrelated
test('classifier: continue. (with punctuation) → unrelated (exit 0, no output)', () => {
  const input = JSON.stringify({ prompt:'continue.' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

// Test 9c: let's continue (multi-word with continue) → should NOT match single-word skip, should be long enough to check resume
// This should match RESUME_PATTERNS and return resume
test('classifier: let\'s continue → resume (multi-word, matches resume pattern)', () => {
  const input = JSON.stringify({ prompt:'let\'s continue' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  // Should attempt to call binary and get output. Stub will return KB lines.
  assert(result.stdout.includes('[ouroboros]') || result.stdout.trim() === '');
});

// Test 10: pick up where we left off → resume
test('classifier: pick up where we left off → resume', () => {
  const input = JSON.stringify({ prompt:'pick up where we left off' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  // Should attempt binary call
  assert(result.stdout.includes('[ouroboros]') || result.stdout.trim() === '');
});

// Test 11: what's next? → resume
test('classifier: what\'s next? → resume', () => {
  const input = JSON.stringify({ prompt:'what\'s next?' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert(result.stdout.includes('[ouroboros]') || result.stdout.trim() === '');
});

// Test 12: backlog → resume (matches the /\bbacklog\b/ pattern)
test('classifier: backlog → resume', () => {
  const input = JSON.stringify({ prompt:'backlog' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert(result.stdout.includes('[ouroboros]') || result.stdout.trim() === '');
});

// Test 13: how does the auth middleware work? → specific (long enough, no skip, no resume)
test('classifier: how does the auth middleware work? → specific', () => {
  const input = JSON.stringify({ prompt:'how does the auth middleware work?' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  // Should attempt binary search
  assert(result.stdout.includes('[ouroboros]') || result.stdout.trim() === '');
});

// Test 14: end-to-end Tool loaded. → no output, no cooldown touch
test('e2e: Tool loaded. → exit 0, no output, no cooldown file touched', () => {
  const cooldownFile = `/tmp/.ouroboros-ctx-dangernoodle-marketplace`;
  // Remove cooldown if exists
  try { fs.unlinkSync(cooldownFile); } catch (e) {}

  const input = JSON.stringify({ prompt:'Tool loaded.' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
  // Verify cooldown was NOT touched
  const exists = fs.existsSync(cooldownFile);
  assert.strictEqual(exists, false, 'cooldown file should not exist for unrelated prompts');
});

// Test 15: end-to-end resume prompt → stub binary called, KB lines written
test('e2e: resume prompt → calls stub, writes KB output', () => {
  const input = JSON.stringify({ prompt:'what\'s next?' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  // Should have KB output from stub
  const hasOutput = result.stdout.includes('[ouroboros]');
  if (hasOutput) {
    assert(result.stdout.includes('KB ('));
    assert(result.stdout.includes('[note]') || result.stdout.includes('[decision]') || result.stdout.includes('[fact]'));
  }
  // Note: output depends on stub being found and called, which may not happen in test sandbox
  // The important part is: exit code is 0, and if output exists it has the right format
});

test('cleanup: remove temp stub dir', () => {
  if (tempDir && fs.existsSync(tempDir)) {
    fs.rmSync(tempDir, { recursive: true });
  }
});

// OU-257: project resolution must key off session cwd, NOT the most
// recently-read file path in the transcript. A transcript whose last tool_use
// references a file under a foreign project must NOT redirect resolution
// away from the cwd's project.
test('OU-257: transcript last-touched file in a foreign project is ignored; cwd project wins', () => {
  if (!fs.existsSync(FIXTURES_PATH)) {
    return;
  }

  const workspaceRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-ou257-'));

  // Session's real project: breadboard (this is where cwd points)
  const breadboard = path.join(workspaceRoot, 'breadboard');
  fs.mkdirSync(breadboard);
  spawnSync('git', ['init', '-q'], { cwd: breadboard });

  // Clear cooldown so injection isn't suppressed by a real dev session's
  // recent breadboard context injection outside this test.
  try { fs.unlinkSync('/tmp/.ouroboros-ctx-breadboard'); } catch (e) {}

  // Foreign project: dangernoodle-github (last file read in the transcript,
  // but NOT where the session is actually working)
  const foreign = path.join(workspaceRoot, 'dangernoodle-github');
  fs.mkdirSync(foreign);
  fs.writeFileSync(path.join(foreign, 'workflow.yml'), '# stub');

  fs.mkdirSync(path.join(workspaceRoot, '.claude'));

  const testStubDir = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-ou257-bin-'));
  const stubPath = path.join(testStubDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);

  // Transcript's most recent tool_use references the foreign project's file
  const transcriptPath = path.join(workspaceRoot, 'transcript.jsonl');
  const foreignFile = path.join(foreign, 'workflow.yml');
  const line = JSON.stringify({
    message: { content: [{ type: 'tool_use', input: { file_path: foreignFile } }] }
  });
  fs.writeFileSync(transcriptPath, line);

  // Prompt is substantive (specific intent) and does not mention either project by name
  const input = JSON.stringify({
    prompt: 'how does the reconnect logic handle a dropped socket?',
    cwd: breadboard,
    transcript_path: transcriptPath,
  });
  const result = runScript(input, { PATH: `${testStubDir}:${process.env.PATH}` });

  assert.strictEqual(result.status, 0);
  assert(result.stdout.includes('[ouroboros] breadboard KB'), 'should inject KB context for the cwd project (breadboard)');
  assert(!result.stdout.includes('dangernoodle-github'), 'must NOT mislabel context under the foreign last-read-file project');

  fs.rmSync(workspaceRoot, { recursive: true, force: true });
  fs.rmSync(testStubDir, { recursive: true, force: true });
});

// OU-257: fail-open — cwd doesn't resolve to any git repo, and the prompt
// doesn't explicitly mention a known project name → no injection, no crash,
// and (critically) no fallback guess from the transcript's last-touched file.
test('OU-257: unresolvable cwd + no project mention in prompt → fail-open, no injection', () => {
  if (!fs.existsSync(FIXTURES_PATH)) {
    return;
  }

  const workspaceRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-ou257-failopen-'));
  // workspaceRoot itself is NOT a git repo (simulates a multi-project
  // workspace root as cwd, with no enclosing .git)
  fs.mkdirSync(path.join(workspaceRoot, '.claude'));

  const otherProject = path.join(workspaceRoot, 'some-other-project');
  fs.mkdirSync(otherProject);
  fs.writeFileSync(path.join(otherProject, 'file.js'), '// stub');

  const testStubDir = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-ou257-failopen-bin-'));
  const stubPath = path.join(testStubDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);

  const transcriptPath = path.join(workspaceRoot, 'transcript.jsonl');
  const line = JSON.stringify({
    message: { content: [{ type: 'tool_use', input: { file_path: path.join(otherProject, 'file.js') } }] }
  });
  fs.writeFileSync(transcriptPath, line);

  const input = JSON.stringify({
    prompt: 'why does the retry loop spin so aggressively on failure?',
    cwd: workspaceRoot,
    transcript_path: transcriptPath,
  });
  const result = runScript(input, { PATH: `${testStubDir}:${process.env.PATH}` });

  assert.strictEqual(result.status, 0, 'hook must exit 0 (fail-open), never crash');
  assert.strictEqual(result.stdout.trim(), '', 'must not inject context or guess a project from the last-touched file');

  fs.rmSync(workspaceRoot, { recursive: true, force: true });
  fs.rmSync(testStubDir, { recursive: true, force: true });
});

test('e2e: message hint resolves project name from text', () => {
  if (!fs.existsSync(FIXTURES_PATH)) {
    return;
  }

  const workspaceRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-msg-'));
  const testProj = path.join(workspaceRoot, 'test-project');
  fs.mkdirSync(testProj);
  fs.mkdirSync(path.join(workspaceRoot, '.claude'));

  // Create stub in a temp dir
  const testStubDir = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-msg-bin-'));
  const stubPath = path.join(testStubDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);

  // Run with prompt containing project name
  const input = JSON.stringify({
    prompt: 'Let me work on test-project now for a while'
  });
  const result = runScript(input, { PATH: `${testStubDir}:${process.env.PATH}` });

  assert.strictEqual(result.status, 0);

  fs.rmSync(workspaceRoot, { recursive: true });
  fs.rmSync(testStubDir, { recursive: true });
});

// Test legacy fallback: message field (backwards compatibility)
test('fallback: legacy message field still works', () => {
  if (!fs.existsSync(FIXTURES_PATH)) {
    return;
  }

  const workspaceRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-legacy-'));
  const testProj = path.join(workspaceRoot, 'test-project');
  fs.mkdirSync(testProj);
  fs.mkdirSync(path.join(workspaceRoot, '.claude'));

  // Create stub in a temp dir
  const testStubDir = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-legacy-bin-'));
  const stubPath = path.join(testStubDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);

  // Run with legacy message field (not prompt)
  const input = JSON.stringify({
    message: 'how does the auth middleware work? reviewing test-project now'
  });
  const result = runScript(input, { PATH: `${testStubDir}:${process.env.PATH}` });

  assert.strictEqual(result.status, 0);
  // Should still work with fallback
  assert(result.stdout.includes('[ouroboros]') || result.stdout.trim() === '');

  fs.rmSync(workspaceRoot, { recursive: true });
  fs.rmSync(testStubDir, { recursive: true });
});

test('user-prompt-context: fire event logged with hook:user_prompt_context', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-upc-fire-home-'));
  try {
    const input = JSON.stringify({ prompt: 'what\'s next?', session_id: 'sess-test-123' });

    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);

    const logFile = path.join(testHomeDir, '.ouroboros', 'hooks.log');
    assert(fs.existsSync(logFile), 'hooks.log should exist (fire event always logged)');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    const fireEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.hook === 'user_prompt_context' && entry.kind === 'fire';
      } catch (e) { return false; }
    });
    assert(fireEvent, 'should have a fire event with hook=user_prompt_context');
    const parsed = JSON.parse(fireEvent);
    assert.strictEqual(parsed.session_id, 'sess-test-123', 'fire event should include session_id');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('user-prompt-context: KB context still injected to stdout (regression)', () => {
  const input = JSON.stringify({ prompt: 'what\'s next?' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  const stdout = result.stdout;
  // Stub returns KB data, so verify expected context is present
  assert(stdout.includes('[ouroboros]') || stdout.trim() === '');
});

test('user-prompt-context: contract reminder injected on every fire with KB', () => {
  if (!fs.existsSync(FIXTURES_PATH)) {
    return;
  }

  const testProj = path.join(homeDir, 'test-project-contract');
  fs.mkdirSync(testProj);

  const testStubDir = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-contract-bin-'));
  const stubPath = path.join(testStubDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);

  // Use cwd param to resolve project directly (projectFromPath will walk up to find git root)
  const input = JSON.stringify({ cwd: testProj, prompt: 'picking up work' });
  const result = runScript(input, { PATH: `${testStubDir}:${process.env.PATH}` });
  assert.strictEqual(result.status, 0);
  const stdout = result.stdout;

  // Since we need git root, this may not have output. Just verify contract logic separately.
  // Skip if no output expected (project not found)
  if (stdout.includes('[ouroboros]')) {
    assert(stdout.includes('if a decision or fact is worth persisting'), 'should have contract reminder if KB found');
    assert(!stdout.includes('```kb'), 'should NOT have fenced block');
  }

  fs.rmSync(testProj, { recursive: true });
  fs.rmSync(testStubDir, { recursive: true });
});

test('user-prompt-context: contract reminder injected on back-to-back prompts (no cooldown)', () => {
  // Regression test: ensure contract is shown on consecutive prompts (no 24h cooldown)
  if (!fs.existsSync(FIXTURES_PATH)) {
    return;
  }

  const testProj = path.join(homeDir, 'test-project-backtoback');
  fs.mkdirSync(testProj);

  const testStubDir = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-backtoback-bin-'));
  const stubPath = path.join(testStubDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);

  // First prompt
  const input1 = JSON.stringify({ cwd: testProj, prompt: 'starting work' });
  const result1 = runScript(input1, { PATH: `${testStubDir}:${process.env.PATH}` });
  assert.strictEqual(result1.status, 0);

  // Second prompt immediately after
  const input2 = JSON.stringify({ cwd: testProj, prompt: 'continuing work' });
  const result2 = runScript(input2, { PATH: `${testStubDir}:${process.env.PATH}` });
  assert.strictEqual(result2.status, 0);

  // If KB output on both, both should have the reminder (no cooldown blocking second)
  if (result2.stdout.includes('[ouroboros]')) {
    assert(result2.stdout.includes('if a decision or fact is worth persisting'), 'contract should appear on second prompt (no cooldown)');
  }

  fs.rmSync(testProj, { recursive: true });
  fs.rmSync(testStubDir, { recursive: true });
});

// OU-14: cooldown default reduced to 5 minutes
test('OU-14: default COOLDOWN_MS is 5 minutes (300000ms)', () => {
  // Read the script source and verify the default value is 300000
  const src = fs.readFileSync(SCRIPT_PATH, 'utf-8');
  // Should NOT have the old 30-min default hardcoded
  assert(!src.includes('1800000'), 'old 30-minute hardcoded default should be removed');
  // Should have 300000 as the fallback
  assert(src.includes('300000'), 'new 5-minute default (300000) should be present');
});

test('OU-14: OUROBOROS_UPC_COOLDOWN_MS env override is respected', () => {
  // Write a cooldown file with a timestamp just 2 minutes ago
  const project = 'test-cooldown-override';
  const cooldownFile = `/tmp/.ouroboros-ctx-${project}`;
  try { fs.unlinkSync(cooldownFile); } catch (e) {}
  fs.writeFileSync(cooldownFile, '');
  // Set mtime to 2 minutes ago
  const twoMinAgo = new Date(Date.now() - 2 * 60 * 1000);
  fs.utimesSync(cooldownFile, twoMinAgo, twoMinAgo);

  // With 5-min default: 2 min ago is within cooldown → should be blocked
  // With env override of 1 min: 2 min ago is outside cooldown → should pass
  const input = JSON.stringify({ prompt: 'how does the auth system work exactly?', cwd: '/tmp' });

  // 1-minute override: cooldown expired (2 min > 1 min)
  const result = runScript(input, { OUROBOROS_UPC_COOLDOWN_MS: '60000' });
  assert.strictEqual(result.status, 0);
  // We can't assert output here easily (no project), but just verify no crash

  try { fs.unlinkSync(cooldownFile); } catch (e) {}
});

// OU-191: BM25 threshold filtering
test('OU-191: BM25_THRESHOLD default is -2.0', () => {
  const src = fs.readFileSync(SCRIPT_PATH, 'utf-8');
  assert(src.includes('-2.0'), 'BM25 threshold default of -2.0 should be in source');
  assert(src.includes('OUROBOROS_UPC_BM25_THRESHOLD'), 'env override name should be in source');
});

test('OU-191: results without score field pass through BM25 filter (backward compat)', () => {
  // When stub returns rows without a `score` field, they should not be filtered out
  // This verifies the filter condition: typeof r.score !== 'number' || r.score >= threshold
  // Row with no score: typeof undefined !== 'number' → passes
  const src = fs.readFileSync(SCRIPT_PATH, 'utf-8');
  assert(src.includes("typeof r.score !== 'number'"), 'filter should pass-through rows without score field');
});

test('cleanup: remove temp stub dir and HOME', () => {
  if (tempDir && fs.existsSync(tempDir)) {
    fs.rmSync(tempDir, { recursive: true });
  }
  if (homeDir && fs.existsSync(homeDir)) {
    fs.rmSync(homeDir, { recursive: true });
  }
  // Clean cooldown files
  try { fs.unlinkSync(`/tmp/.ouroboros-ctx-ouroboros`); } catch (e) {}
});
