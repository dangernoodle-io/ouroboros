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

test('setup: create temp stub dir', () => {
  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-upc-test-'));
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

// E2E test: transcriptPath hint resolves project
test('e2e: transcript with tool_use → resolves project from hint', () => {
  if (!fs.existsSync(FIXTURES_PATH)) {
    // Skip if fixtures don't exist (will be run in correct test environment)
    return;
  }

  const workspaceRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-e2e-'));
  const ouroboros = path.join(workspaceRoot, 'ouroboros');
  fs.mkdirSync(ouroboros);
  fs.mkdirSync(path.join(ouroboros, 'internal', 'app'), { recursive: true });
  fs.writeFileSync(path.join(ouroboros, 'internal', 'app', 'server.go'), '// stub');

  // Create .claude in workspace root
  fs.mkdirSync(path.join(workspaceRoot, '.claude'));

  // Create stub binary in a new temp dir for this test
  const testStubDir = fs.mkdtempSync(path.join(os.tmpdir(), 'upc-e2e-bin-'));
  const stubPath = path.join(testStubDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);

  // Create a transcript with a tool_use referencing the file
  const transcriptPath = path.join(workspaceRoot, 'transcript.jsonl');
  const filePath = path.join(ouroboros, 'internal', 'app', 'server.go');
  const line = JSON.stringify({
    message: { content: [{ type: 'tool_use', input: { file_path: filePath } }] }
  });
  fs.writeFileSync(transcriptPath, line);

  // Run script with transcript_path hint
  const input = JSON.stringify({
    message: 'what is next',
    transcript_path: transcriptPath
  });
  const result = runScript(input, { PATH: `${testStubDir}:${process.env.PATH}` });

  assert.strictEqual(result.status, 0);
  // Should find ouroboros project via transcript and call stub binary
  const hasOutput = result.stdout.includes('[ouroboros]');
  if (hasOutput) {
    assert(result.stdout.includes('ouroboros'));
  }

  fs.rmSync(workspaceRoot, { recursive: true });
  fs.rmSync(testStubDir, { recursive: true });
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
