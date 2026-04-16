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
let homeDir;

test('setup: create temp stub dir and HOME isolation', () => {
  tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-test-'));
  homeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-subagent-stop-home-'));
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

test('subagent-stop: short message (<80 chars) → exit 0, no stdout, but fire+subagent_stop events logged', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-subagent-stop-short-home-'));
  try {
    const input = JSON.stringify({
      agent_type: 'general',
      agent_id: 'short-msg-123',
      session_id: 'short-test-sess',
      last_assistant_message: 'short',
    });
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
    assert(fs.existsSync(logFile), 'hooks.log should exist even for short messages');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    const fireEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.hook === 'subagent_stop' && entry.kind === 'fire';
      } catch (e) { return false; }
    });
    const stopEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'subagent_stop';
      } catch (e) { return false; }
    });
    assert(fireEvent, 'fire event should be logged even for short messages');
    assert(stopEvent, 'subagent_stop event should be logged even for short messages');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('subagent-stop: kb block + stub put succeeds → log includes count, project, ids, agent_id', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'abc12345678',
    last_assistant_message: 'This is a long message with enough content to pass the minimum length check and then includes a kb block at the end:\n```kb\n[{"type":"decision","title":"adopt cobra"}]\n```',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stderr, /persisted 1 entries/);
  assert.match(result.stderr, /\[ids: 1\]/);
  assert(result.stderr.includes('abc12345'));
});

test('subagent-stop: kb block + stub put fails → log says put failed', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'def87654321',
    last_assistant_message: 'This is a long message with enough content to pass the minimum length check and includes a kb block:\n```kb\n[{"type":"fact"}]\n```',
  });
  const result = runScript(input, { OUROBOROS_STUB_PUT_FAIL: '1' });
  assert.strictEqual(result.status, 0);
  assert.match(result.stderr, /put failed/);
  assert(result.stderr.includes('def87654'));
});

test('subagent-stop: kb block with malformed JSON → logs parse error, does NOT fall through', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'xyz99999999',
    last_assistant_message: 'Long message to pass minimum length with malformed kb block:\n```kb\n{invalid json}\n```\nAnd we decided to adopt approach X',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stderr, /kb block JSON parse error/);
  assert(!result.stderr.includes('tier-1'));
  assert(!result.stderr.includes('persisted'));
});

test('subagent-stop: no kb block + tier-2 self-claim → logs tier-2 detection', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'abc12345678',
    last_assistant_message: 'This is a long message that mentions the knowledge base which is a tier-2 pattern and should be logged as a self-claim',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stderr, /tier-2 self-claim/);
  assert(result.stderr.includes('abc12345'));
});

test('subagent-stop: no kb block + tier-1 decision language → tier-1 nudge log', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'abc12345678',
    last_assistant_message: 'This is a long message with enough content where we decided to adopt a new architecture for the system',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);
  assert.match(result.stderr, /tier-1 nudge fired/);
  assert(result.stderr.includes('abc12345'));
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

test('subagent-stop: fire event logged with hook:subagent_stop', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'agent-fire-123',
    last_assistant_message: 'This is a long message with enough content to pass the minimum length check without any kb blocks',
  });
  const result = runScript(input);
  assert.strictEqual(result.status, 0);

  const logFile = path.join(homeDir, '.ouroboros', 'hooks.log');
  assert(fs.existsSync(logFile), 'hooks.log should exist');

  const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
  const fireEvent = lines.find(line => {
    try {
      const entry = JSON.parse(line);
      return entry.hook === 'subagent_stop' && entry.kind === 'fire';
    } catch (e) { return false; }
  });
  assert(fireEvent, 'should have a fire event with hook=subagent_stop');
});

test('subagent-stop: subagent_stop event logged with agent_id, agent_type, session_id, and last_message_excerpt', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-subagent-stop-event-home-'));
  try {
    const input = JSON.stringify({
      agent_type: 'general',
      agent_id: 'agent-stop-abc789xyz',
      session_id: 'stop-event-sess',
      last_assistant_message: 'This is a long message with enough content to pass the minimum length check and includes multiple lines\nLine 2 with more content\nLine 3 with even more text for testing truncation of long messages',
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
    const stopEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'subagent_stop';
      } catch (e) { return false; }
    });
    assert(stopEvent, 'should have a subagent_stop event');
    const parsed = JSON.parse(stopEvent);
    assert.strictEqual(parsed.agent_id, 'agent-stop-abc789xyz');
    assert.strictEqual(parsed.agent_type, 'general');
    assert.strictEqual(parsed.session_id, 'stop-event-sess');
    assert(parsed.last_message_excerpt, 'should have last_message_excerpt');
    assert(parsed.last_message_excerpt.length <= 120, 'excerpt should be <= 120 chars');
    assert(!parsed.last_message_excerpt.includes('\n'), 'excerpt should have newlines stripped');
  } finally {
    fs.rmSync(testHomeDir, { recursive: true });
  }
});

test('subagent-stop: persist event logged when kb block present', () => {
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'agent-persist-456',
    last_assistant_message: 'This is a long message with enough content to pass the minimum length check and includes a kb block:\n```kb\n[{"type":"fact","title":"test fact"}]\n```',
  });
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

test('subagent-stop: metadata injection → hook:subagent_stop source, agent_id, agent_type, session_id injected, metadata merged', () => {
  const captureFile = path.join(tempDir, `capture-${Date.now()}-${Math.random()}.json`);
  const input = JSON.stringify({
    agent_type: 'general',
    agent_id: 'agent-meta-test-789',
    last_assistant_message: 'This is a long message with enough content to pass the minimum length check and includes a kb block:\n```kb\n[{"type":"decision","title":"first"},{"type":"fact","title":"second","metadata":{"custom":"preserved"}}]\n```',
  });
  const result = runScript(input, { OUROBOROS_PUT_CAPTURE_FILE: captureFile });
  assert.strictEqual(result.status, 0);
  assert(fs.existsSync(captureFile), `capture file should exist at ${captureFile}`);

  const captured = fs.readFileSync(captureFile, 'utf-8');
  const entries = JSON.parse(captured);
  assert.strictEqual(Array.isArray(entries), true, 'captured content should be JSON array');
  assert.strictEqual(entries.length, 2, 'should have 2 entries');

  entries.forEach((e, idx) => {
    assert(e.metadata, `entry ${idx} should have metadata`);
    assert.strictEqual(e.metadata.source, 'hook:subagent_stop', `entry ${idx} should have source=hook:subagent_stop`);
    assert.strictEqual(e.metadata.agent_id, 'agent-meta-test-789', `entry ${idx} should have correct agent_id`);
    assert.strictEqual(e.metadata.agent_type, 'general', `entry ${idx} should have correct agent_type`);
    assert(Object.prototype.hasOwnProperty.call(e.metadata, 'session_id'), `entry ${idx} should have session_id key`);
  });

  const secondEntry = entries[1];
  assert.strictEqual(secondEntry.metadata.custom, 'preserved', 'second entry should preserve custom metadata');
  assert.strictEqual(secondEntry.metadata.source, 'hook:subagent_stop', 'second entry should also have injected source');
});

test('subagent-stop: plugin-qualified knowledge-explorer agent skipped (regression test)', () => {
  const testHomeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-plugin-qualified-stop-skip-'));
  try {
    const input = JSON.stringify({
      agent_type: 'ouroboros-mcp:knowledge-explorer',
      agent_id: 'plugin-kb-explorer',
      session_id: 'plugin-stop-skip-test',
      last_assistant_message: 'This is a long message with persistence keywords like knowledge base and ouroboros references. We decided to persist everything. The message contains more than 80 characters to pass minimum length check.',
    });
    const envVars = { ...process.env, PATH: `${tempDir}:${process.env.PATH}`, HOME: testHomeDir };
    const result = spawnSync('node', [SCRIPT_PATH], {
      input: input,
      encoding: 'utf-8',
      env: envVars,
      cwd: path.join(__dirname, '..'),
    });
    assert.strictEqual(result.status, 0);
    assert.strictEqual(result.stdout.trim(), '', 'no stdout should be emitted for skipped plugin agent');

    const logFile = path.join(testHomeDir, '.ouroboros', 'hooks.log');
    assert(fs.existsSync(logFile), 'hooks.log should exist');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    const fireEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.hook === 'subagent_stop' && entry.kind === 'fire';
      } catch (e) { return false; }
    });
    const stopEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'subagent_stop';
      } catch (e) { return false; }
    });
    const nudgeEvent = lines.find(line => {
      try {
        const entry = JSON.parse(line);
        return entry.kind === 'nudge';
      } catch (e) { return false; }
    });
    assert(fireEvent, 'fire event should be logged');
    assert(stopEvent, 'subagent_stop event should be logged');
    assert(!nudgeEvent, 'no nudge event should be fired for skipped plugin agent');
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
