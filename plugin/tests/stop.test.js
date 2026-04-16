const test = require('node:test');
const assert = require('node:assert/strict');
const { spawnSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const os = require('os');

const SCRIPT_PATH = path.join(__dirname, '..', 'scripts', 'stop.js');
const FIXTURES_PATH = path.join(__dirname, 'fixtures');

let tempDir;
let stubPath;
let homeDir;

test('setup: create temp stub dir and HOME isolation', () => {
  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-stop-test-'));
  homeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-stop-home-'));
  stubPath = path.join(tempDir, 'ouroboros');
  fs.copyFileSync(path.join(FIXTURES_PATH, 'ouroboros-stub-capture.sh'), stubPath);
  fs.chmodSync(stubPath, 0o755);
});

// writeTranscript creates a JSONL transcript file with the given assistant turns.
// Each turn is { text: string, sidechain?: boolean }.
function writeTranscript(turns) {
  const file = path.join(tempDir, `transcript-${Date.now()}-${Math.random()}.jsonl`);
  const lines = turns.map(turn => JSON.stringify({
    type: 'assistant',
    isSidechain: turn.sidechain === true,
    message: { content: [{ type: 'text', text: turn.text }] },
  }));
  fs.writeFileSync(file, lines.join('\n') + '\n');
  return file;
}

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

test('stop: stop_hook_active=true → exit 0, no stdout (avoid infinite loop)', () => {
  const transcript = writeTranscript([{ text: 'we decided to adopt approach X for architectural reasons spanning multiple sentences here' }]);
  const input = JSON.stringify({
    session_id: 'sess1234abcd',
    transcript_path: transcript,
    stop_hook_active: true,
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('stop: missing transcript_path → exit 0, no stdout', () => {
  const input = JSON.stringify({ session_id: 'sess1234abcd' });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('stop: short message (<80 chars) → exit 0, no stdout', () => {
  const transcript = writeTranscript([{ text: 'short' }]);
  const input = JSON.stringify({ session_id: 'sess1234abcd', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('stop: kb block + stub put succeeds → log includes count, project, ids, session', () => {
  const transcript = writeTranscript([{
    text: 'This is a long main-context message with enough content to pass the minimum length check and a kb block at the end:\n```kb\n[{"type":"decision","title":"adopt cobra"}]\n```',
  }]);
  const input = JSON.stringify({ session_id: 'sess1234abcd', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /persisted 1 entries/);
  assert.match(result.stdout, /\[ids: 1\]/);
  assert(result.stdout.includes('sess1234'));
  assert(result.stdout.includes('main'));
});

test('stop: kb block + stub put fails → log says put failed', () => {
  const transcript = writeTranscript([{
    text: 'This is a long main-context message with enough content to pass the minimum length check and includes a kb block:\n```kb\n[{"type":"fact"}]\n```',
  }]);
  const input = JSON.stringify({ session_id: 'sessXYZ12345', transcript_path: transcript });
  const result = runScript(input, { OUROBOROS_STUB_PUT_FAIL: '1' });
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /put failed/);
  assert(result.stdout.includes('sessXYZ1'));
});

test('stop: kb block with malformed JSON → logs parse error, does NOT fall through', () => {
  const transcript = writeTranscript([{
    text: 'Long message with malformed kb block:\n```kb\n{invalid json}\n```\nAnd we decided to adopt approach X with strong rationale here',
  }]);
  const input = JSON.stringify({ session_id: 'sessparse123', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /kb block JSON parse error/);
  assert(!result.stdout.includes('tier-1'));
  assert(!result.stdout.includes('persisted'));
});

test('stop: no kb block + tier-2 self-claim → logs tier-2 detection', () => {
  const transcript = writeTranscript([{
    text: 'This is a long main-context message that mentions the knowledge base which is a tier-2 pattern and should be logged as a self-claim',
  }]);
  const input = JSON.stringify({ session_id: 'sesst2abc123', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /tier-2 self-claim/);
  assert(result.stdout.includes('sesst2ab'));
});

test('stop: no kb block + tier-1 decision language → tier-1 nudge log', () => {
  const transcript = writeTranscript([{
    text: 'This is a long main-context message with enough content where we decided to adopt a new architecture for the system based on rationale',
  }]);
  const input = JSON.stringify({ session_id: 'sesst1abc123', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /tier-1 nudge fired/);
  assert.match(result.stdout, /call put now/);
  assert(result.stdout.includes('sesst1ab'));
});

test('stop: no kb block + neutral content → exit 0, no stdout', () => {
  const transcript = writeTranscript([{
    text: 'This is just a simple neutral message that talks about how the weather is nice and contains no decision language or kb blocks',
  }]);
  const input = JSON.stringify({ session_id: 'sessneutral1', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('stop: skips sidechain (subagent) turns and uses last main-context turn', () => {
  const transcript = writeTranscript([
    { text: 'Earlier main turn with enough length to be considered for processing but not the most recent', sidechain: false },
    { text: 'Subagent sidechain turn that we decided to ignore for architectural reasons of separation between main and sub', sidechain: true },
    { text: 'Latest main-context turn that we decided to adopt the new architecture in for clear rationale and decision making', sidechain: false },
  ]);
  const input = JSON.stringify({ session_id: 'sessmain1234', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stdout, /tier-1 nudge fired/);
});

test('stop: only sidechain turns present → exit 0, no stdout', () => {
  const transcript = writeTranscript([
    { text: 'Subagent sidechain turn one with we decided to adopt and architectural language that should not trigger main hook', sidechain: true },
    { text: 'Subagent sidechain turn two with more decided to and architectural rationale that should also not trigger', sidechain: true },
  ]);
  const input = JSON.stringify({ session_id: 'sessideonly1', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.strictEqual(result.stdout.trim(), '');
});

test('stop: hook fire event logged with hook:stop and session_id', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-stop-fire-home-'));
  try {
    const transcript = writeTranscript([{
      text: 'This is a long main-context message with enough content to pass the minimum length check but no kb block',
    }]);
    const input = JSON.stringify({ session_id: 'sess-fire-test-123', transcript_path: transcript });

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
        return entry.hook === 'stop' && entry.kind === 'fire';
      } catch (e) { return false; }
    });
    assert(fireEvent, 'should have a fire event with hook=stop');
    const parsed = JSON.parse(fireEvent);
    assert.strictEqual(parsed.session_id, 'sess-fire-test-123');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('stop: persist event logged when kb block present and put succeeds', () => {
  const transcript = writeTranscript([{
    text: 'This is a long main-context message with enough content to pass the minimum length check and includes a kb block:\n```kb\n[{"type":"decision","title":"test decision"}]\n```',
  }]);
  const input = JSON.stringify({ session_id: 'sess-persist-123', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);

  const logFile = path.join(homeDir, '.ouroboros', 'hooks.log');
  const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
  const persistEvent = lines.find(line => {
    try {
      const entry = JSON.parse(line);
      return entry.kind === 'persist';
    } catch (e) { return false; }
  });
  assert(persistEvent, 'should have a persist event');
  const parsed = JSON.parse(persistEvent);
  assert.strictEqual(parsed.entries, 1, 'entries count should match KB block size');
});

test('stop: metadata injection → hook:stop source and session_id injected, metadata merged', () => {
  const captureFile = path.join(tempDir, `capture-${Date.now()}-${Math.random()}.json`);
  const transcript = writeTranscript([{
    text: 'This is a long main-context message with enough content to pass the minimum length check and includes a kb block:\n```kb\n[{"type":"decision","title":"first"},{"type":"note","title":"second","metadata":{"custom":"value"}}]\n```',
  }]);
  const input = JSON.stringify({ session_id: 'sess-metadata-test', transcript_path: transcript });
  const result = runScript(input, { OUROBOROS_PUT_CAPTURE_FILE: captureFile });
  assert.strictEqual(result.status, 0);
  assert(fs.existsSync(captureFile), `capture file should exist at ${captureFile}`);

  const captured = fs.readFileSync(captureFile, 'utf-8');
  const entries = JSON.parse(captured);
  assert.strictEqual(Array.isArray(entries), true, 'captured content should be JSON array');
  assert.strictEqual(entries.length, 2, 'should have 2 entries');

  entries.forEach((e, idx) => {
    assert(e.metadata, `entry ${idx} should have metadata`);
    assert.strictEqual(e.metadata.source, 'hook:stop', `entry ${idx} should have source=hook:stop`);
    assert(Object.prototype.hasOwnProperty.call(e.metadata, 'session_id'), `entry ${idx} should have session_id key`);
  });

  const secondEntry = entries[1];
  assert.strictEqual(secondEntry.metadata.custom, 'value', 'second entry should preserve custom metadata');
  assert.strictEqual(secondEntry.metadata.source, 'hook:stop', 'second entry should also have injected source');
});

test('stop: nudge event logged on tier-1 decision language detection', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-stop-nudge-home-'));
  try {
    const transcript = writeTranscript([{
      text: 'This is a long main-context message with enough content where we decided to adopt a new architecture for the system based on solid rationale',
    }]);
    const input = JSON.stringify({ session_id: 'sess-nudge-123', transcript_path: transcript });

    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);

    const logFile = path.join(testHomeDir, '.ouroboros', 'hooks.log');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    const nudgeEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'nudge';
      } catch (e) { return false; }
    });
    assert(nudgeEvent, 'should have a nudge event');
    const parsed = JSON.parse(nudgeEvent);
    assert.strictEqual(parsed.reason, 'tier-1');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('stop: noop event logged for neutral content', () => {
  const transcript = writeTranscript([{
    text: 'This is just a simple neutral message that talks about the weather and contains no decision language or kb blocks',
  }]);
  const input = JSON.stringify({ session_id: 'sess-noop-123', transcript_path: transcript });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);

  const logFile = path.join(homeDir, '.ouroboros', 'hooks.log');
  const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
  const noopEvent = lines.find(line => {
    try {
      const entry = JSON.parse(line);
      return entry.kind === 'noop';
    } catch (e) { return false; }
  });
  assert(noopEvent, 'should have a noop event for neutral content');
});

test('stop: cwd hint resolves project for git repo', () => {
  const gitRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), 'stop-git-repo-'));
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-stop-cwd-home-'));
  try {
    fs.mkdirSync(path.join(gitRepoDir, '.git'));
    const transcript = writeTranscript([{
      text: 'This is a long main-context message with enough content but no kb block or decision language',
    }]);
    const cwdPath = path.join(gitRepoDir, 'src');
    fs.mkdirSync(cwdPath, { recursive: true });

    const input = JSON.stringify({
      session_id: 'sess-cwd-test',
      transcript_path: transcript,
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

    const logFile = path.join(testHomeDir, '.ouroboros', 'hooks.log');
    assert(fs.existsSync(logFile), 'hooks.log should exist');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    const fireEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.hook === 'stop' && entry.kind === 'fire';
      } catch (e) { return false; }
    });
    assert(fireEvent, 'fire event should be logged');
    const parsed = JSON.parse(fireEvent);
    // Project should be resolved from cwd via git root
    assert.strictEqual(parsed.project, path.basename(gitRepoDir));
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
