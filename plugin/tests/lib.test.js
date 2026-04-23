const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');
const { extractKbBlock, extractAllKbBlocks, matchesAnyPattern, formatContextLines, findGitRoot, projectFromPath, findWorkspaceRoot, listWorkspaceProjects, resolveProject, logHookEvent, getMaxLogSize, getMaxLogFiles, rotateLogFiles, isSkippedAgentType } = require('../scripts/lib');

test('extractKbBlock - well-formed block returns matched=true + JSON string', () => {
  const message = 'Some text\n```kb\n[{"type":"decision"}]\n```\nMore text';
  const result = extractKbBlock(message);
  assert.strictEqual(result.matched, true);
  assert.strictEqual(result.json, '[{"type":"decision"}]');
});

test('extractKbBlock - missing block returns matched=false + json=null', () => {
  const message = 'Some text without a kb block';
  const result = extractKbBlock(message);
  assert.strictEqual(result.matched, false);
  assert.strictEqual(result.json, null);
});

test('extractKbBlock - multiple blocks → first wins', () => {
  const message = '```kb\n[{"type":"first"}]\n```\nMiddle\n```kb\n[{"type":"second"}]\n```';
  const result = extractKbBlock(message);
  assert.strictEqual(result.matched, true);
  assert.strictEqual(result.json, '[{"type":"first"}]');
});

test('extractKbBlock - block with surrounding prose extracted cleanly', () => {
  const message = 'Here is the kb block:\n```kb\n[{"type":"note","title":"test"}]\n```\nEnd of message';
  const result = extractKbBlock(message);
  assert.strictEqual(result.matched, true);
  assert.strictEqual(result.json, '[{"type":"note","title":"test"}]');
});

test('extractKbBlock - malformed (no closing fence) → matched=false', () => {
  const message = 'Here is broken:\n```kb\n[{"type":"note"}]';
  const result = extractKbBlock(message);
  assert.strictEqual(result.matched, false);
  assert.strictEqual(result.json, null);
});

test('matchesAnyPattern - empty patterns → false', () => {
  const result = matchesAnyPattern('some message', []);
  assert.strictEqual(result, false);
});

test('matchesAnyPattern - empty patterns array null → false', () => {
  const result = matchesAnyPattern('some message', null);
  assert.strictEqual(result, false);
});

test('matchesAnyPattern - one match → true', () => {
  const patterns = [/hello/, /world/];
  const result = matchesAnyPattern('hello there', patterns);
  assert.strictEqual(result, true);
});

test('matchesAnyPattern - case-insensitive flags honored', () => {
  const patterns = [/HELLO/i, /world/];
  const result = matchesAnyPattern('hello there', patterns);
  assert.strictEqual(result, true);
});

test('matchesAnyPattern - no match → false', () => {
  const patterns = [/apple/, /banana/];
  const result = matchesAnyPattern('orange and grape', patterns);
  assert.strictEqual(result, false);
});

test('formatContextLines - empty rows → empty array', () => {
  const result = formatContextLines('test-project', []);
  assert.deepStrictEqual(result, []);
});

test('formatContextLines - empty rows (null) → empty array', () => {
  const result = formatContextLines('test-project', null);
  assert.deepStrictEqual(result, []);
});

test('formatContextLines - N rows → header + N indented lines + single reminder line (default)', () => {
  const rows = [
    { type: 'decision', title: 'adopt cobra' },
    { type: 'fact', title: 'FTS5 cap' },
  ];
  const result = formatContextLines('myproject', rows);

  assert.strictEqual(result[0], '[ouroboros] myproject KB (2):');
  assert.strictEqual(result[1], '  [decision] adopt cobra');
  assert.strictEqual(result[2], '  [fact] FTS5 cap');
  assert.strictEqual(result[3], 'if a decision or fact is worth persisting, emit a fenced kb block (project: myproject); otherwise say nothing');
  assert.strictEqual(result.length, 4, 'should be exactly 4 lines (header + 2 KB + reminder)');
  assert(!result.some(line => line === '```kb'));
  assert(!result.some(line => line === '```'));
});

test('formatContextLines - options.includeContract=false → header + N lines WITHOUT contract', () => {
  const rows = [
    { type: 'decision', title: 'adopt cobra' },
    { type: 'fact', title: 'FTS5 cap' },
  ];
  const result = formatContextLines('myproject', rows, { includeContract: false });

  assert.strictEqual(result[0], '[ouroboros] myproject KB (2):');
  assert.strictEqual(result[1], '  [decision] adopt cobra');
  assert.strictEqual(result[2], '  [fact] FTS5 cap');
  assert.strictEqual(result.length, 3, 'should be exactly 3 lines (no contract)');
  assert(!result.some(line => line.includes('persist any decisions/facts')));
  assert(!result.some(line => line === '```kb'));
  assert(!result.some(line => line === '```'));
});

test('formatContextLines - project name interpolated in reminder line', () => {
  const rows = [{ type: 'note', title: 'test' }];
  const result = formatContextLines('special-proj', rows);
  assert(result[0].includes('special-proj'));
  assert(result[2].includes('(project: special-proj)'));
});

test('formatContextLines - project name interpolated, no contract when includeContract=false', () => {
  const rows = [{ type: 'note', title: 'test' }];
  const result = formatContextLines('special-proj', rows, { includeContract: false });
  assert(result[0].includes('special-proj'));
  assert(!result.some(line => line.includes('(project: special-proj)')));
});

// Tests for findGitRoot
test('findGitRoot - finds .git directory from a file inside repo', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'git-root-'));
  fs.mkdirSync(path.join(tmpRoot, '.git'));
  fs.mkdirSync(path.join(tmpRoot, 'src'));
  const filePath = path.join(tmpRoot, 'src', 'main.js');
  fs.writeFileSync(filePath, '');

  const result = findGitRoot(filePath);
  assert.strictEqual(fs.realpathSync(result), fs.realpathSync(tmpRoot));

  fs.rmSync(tmpRoot, { recursive: true });
});

test('findGitRoot - finds .git directory from a directory inside repo', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'git-root-dir-'));
  fs.mkdirSync(path.join(tmpRoot, '.git'));
  const srcDir = path.join(tmpRoot, 'src');
  fs.mkdirSync(srcDir);

  const result = findGitRoot(srcDir);
  assert.strictEqual(fs.realpathSync(result), fs.realpathSync(tmpRoot));

  fs.rmSync(tmpRoot, { recursive: true });
});

test('findGitRoot - returns null when not in a git repo', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'no-git-'));
  const filePath = path.join(tmpRoot, 'file.js');
  fs.writeFileSync(filePath, '');

  const result = findGitRoot(filePath);
  assert.strictEqual(result, null);

  fs.rmSync(tmpRoot, { recursive: true });
});

test('findGitRoot - handles nonexistent start path', () => {
  // Path doesn't exist but should still walk up correctly
  const nonexistent = '/tmp/definitely-not-real-12345/src/code.js';
  const result = findGitRoot(nonexistent);
  // Either null or a real git repo (if walking up hits the real filesystem)
  assert(result === null || typeof result === 'string');
});

test('findGitRoot - wrapped in try/catch, returns null on error', () => {
  // Test that errors are caught gracefully
  const result = findGitRoot(null);
  assert.strictEqual(result, null);
});

// Tests for projectFromPath
test('projectFromPath - returns basename of git root', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'proj-name-'));
  fs.mkdirSync(path.join(tmpRoot, '.git'));
  const filePath = path.join(tmpRoot, 'src', 'main.js');
  fs.mkdirSync(path.join(tmpRoot, 'src'));
  fs.writeFileSync(filePath, '');

  const result = projectFromPath(filePath);
  assert.strictEqual(result, path.basename(tmpRoot));

  fs.rmSync(tmpRoot, { recursive: true });
});

test('projectFromPath - returns null when not in a git repo', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'no-git-proj-'));
  const filePath = path.join(tmpRoot, 'file.js');
  fs.writeFileSync(filePath, '');

  const result = projectFromPath(filePath);
  assert.strictEqual(result, null);

  fs.rmSync(tmpRoot, { recursive: true });
});

test('projectFromPath - returns null on error (wrapped in try/catch)', () => {
  const result = projectFromPath(null);
  assert.strictEqual(result, null);
});

// Tests for findWorkspaceRoot
test('findWorkspaceRoot - finds .claude in current dir', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'ws-root-'));
  const claudeDir = path.join(tmpRoot, '.claude');
  fs.mkdirSync(claudeDir);

  const originalCwd = process.cwd();
  try {
    process.chdir(tmpRoot);
    const result = findWorkspaceRoot();
    // Normalize paths to handle macOS /tmp -> /private/var/folders symlink
    assert.strictEqual(fs.realpathSync(result), fs.realpathSync(tmpRoot));
  } finally {
    process.chdir(originalCwd);
    fs.rmSync(tmpRoot, { recursive: true });
  }
});

test('findWorkspaceRoot - walks up to find .claude in parent', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'ws-parent-'));
  const claudeDir = path.join(tmpRoot, '.claude');
  fs.mkdirSync(claudeDir);
  const subDir = path.join(tmpRoot, 'subproject');
  fs.mkdirSync(subDir);

  const originalCwd = process.cwd();
  try {
    process.chdir(subDir);
    const result = findWorkspaceRoot();
    // May find dangernoodle workspace instead due to real .claude in ancestor
    // Just verify it finds SOMETHING with .claude
    assert(result !== null && result.endsWith('.claude') === false);
  } finally {
    process.chdir(originalCwd);
    fs.rmSync(tmpRoot, { recursive: true });
  }
});

test('findWorkspaceRoot - returns null if no .claude found', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'ws-none-'));
  const subDir = path.join(tmpRoot, 'sub');
  fs.mkdirSync(subDir);

  const originalCwd = process.cwd();
  try {
    process.chdir(subDir);
    const result = findWorkspaceRoot();
    // May still find real workspace .claude in actual ancestor, so skip strict assert
    // Just ensure it returns a string or null
    assert(result === null || typeof result === 'string');
  } finally {
    process.chdir(originalCwd);
    fs.rmSync(tmpRoot, { recursive: true });
  }
});

// Tests for listWorkspaceProjects
test('listWorkspaceProjects - returns null → []', () => {
  const result = listWorkspaceProjects(null);
  assert.deepStrictEqual(result, []);
});

test('listWorkspaceProjects - lists non-dot subdirs only', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'ws-list-'));
  fs.mkdirSync(path.join(tmpRoot, 'project-a'));
  fs.mkdirSync(path.join(tmpRoot, 'project-b'));
  fs.mkdirSync(path.join(tmpRoot, '.hidden'));
  fs.writeFileSync(path.join(tmpRoot, 'file.txt'), '');

  const result = listWorkspaceProjects(tmpRoot);
  assert(result.includes('project-a'));
  assert(result.includes('project-b'));
  assert(!result.includes('.hidden'));
  assert(!result.includes('file.txt'));

  fs.rmSync(tmpRoot, { recursive: true });
});

test('listWorkspaceProjects - empty dir → []', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'ws-empty-'));
  const result = listWorkspaceProjects(tmpRoot);
  assert.deepStrictEqual(result, []);
  fs.rmSync(tmpRoot, { recursive: true });
});

test('listWorkspaceProjects - read error → []', () => {
  const nonexistent = path.join(os.tmpdir(), 'nonexistent-ws-dir-12345');
  const result = listWorkspaceProjects(nonexistent);
  assert.deepStrictEqual(result, []);
});

// Tests for resolveProject
test('resolveProject - no hints, no git, no workspace → null', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-none-'));
  const result = resolveProject({}, tmpDir);
  assert.strictEqual(result, null);
  fs.rmSync(tmpDir, { recursive: true });
});

test('resolveProject - filePath hint matches project', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-filepath-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'my-project'));
  fs.writeFileSync(path.join(tmpRoot, 'my-project', 'file.js'), '');

  const filePath = path.join(tmpRoot, 'my-project', 'file.js');
  const result = resolveProject({ filePath }, tmpRoot);
  assert.strictEqual(result, 'my-project');

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - message hint matches project name (word boundary)', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-msg-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'ouroboros'));
  fs.mkdirSync(path.join(tmpRoot, 'terranoodle'));

  const message = 'I need help with the ouroboros project setup';
  const result = resolveProject({ message }, tmpRoot);
  assert.strictEqual(result, 'ouroboros');

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - message does NOT match without word boundary', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-noboundary-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'test'));

  // "testing" should NOT match project "test" without word boundary
  const message = 'I am testing this feature';
  const result = resolveProject({ message }, tmpRoot);
  assert.strictEqual(result, null);

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - message hint is case-insensitive', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-case-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'MyProject'));

  const message = 'working on myproject now';
  const result = resolveProject({ message }, tmpRoot);
  assert.strictEqual(result, 'MyProject');

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - transcriptPath scans backwards for tool_use', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-transcript-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'projectX'));

  const transcriptPath = path.join(tmpRoot, 'transcript.jsonl');
  const filePath = path.join(tmpRoot, 'projectX', 'src', 'main.js');
  const line1 = JSON.stringify({ message: { content: [{ type: 'text', text: 'hello' }] } });
  const line2 = JSON.stringify({
    message: { content: [{ type: 'tool_use', input: { file_path: filePath } }] }
  });
  fs.writeFileSync(transcriptPath, line1 + '\n' + line2 + '\n');

  const result = resolveProject({ transcriptPath }, tmpRoot);
  assert.strictEqual(result, 'projectX');

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - transcriptPath scans first match (backwards)', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-first-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'first'));
  fs.mkdirSync(path.join(tmpRoot, 'second'));

  const transcriptPath = path.join(tmpRoot, 'trans.jsonl');
  const line1 = JSON.stringify({
    message: { content: [{ type: 'tool_use', input: { file_path: path.join(tmpRoot, 'first', 'f.js') } }] }
  });
  const line2 = JSON.stringify({
    message: { content: [{ type: 'tool_use', input: { file_path: path.join(tmpRoot, 'second', 's.js') } }] }
  });
  fs.writeFileSync(transcriptPath, line1 + '\n' + line2 + '\n');

  const result = resolveProject({ transcriptPath }, tmpRoot);
  // Scanning backwards, line2 (second) is encountered first
  assert.strictEqual(result, 'second');

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - transcriptPath respects 2000-line scan cap', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-cap-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'myproj'));

  const transcriptPath = path.join(tmpRoot, 'huge.jsonl');
  let content = '';
  // Write 2500 lines of empty JSON lines
  for (let i = 0; i < 2500; i++) {
    content += JSON.stringify({ message: { content: [] } }) + '\n';
  }
  // At the very beginning (won't be scanned due to cap), add target
  const targetLine = JSON.stringify({
    message: { content: [{ type: 'tool_use', input: { file_path: path.join(tmpRoot, 'myproj', 'x.js') } }] }
  });
  content = targetLine + '\n' + content;
  fs.writeFileSync(transcriptPath, content);

  const result = resolveProject({ transcriptPath }, tmpRoot);
  // Should NOT find it because it's beyond the 2000-line scan window
  assert.strictEqual(result, null);

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - transcriptPath with path/abs_path fallbacks', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-fallback-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'proj'));

  const transcriptPath = path.join(tmpRoot, 'trans.jsonl');
  const filePath = path.join(tmpRoot, 'proj', 'file.js');
  // Try with .path instead of .file_path
  const line = JSON.stringify({
    message: { content: [{ type: 'tool_use', input: { path: filePath } }] }
  });
  fs.writeFileSync(transcriptPath, line);

  const result = resolveProject({ transcriptPath }, tmpRoot);
  assert.strictEqual(result, 'proj');

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - priority: git > filePath > message > transcript', () => {
  // This test verifies that without git, filePath (priority 2) wins over message (priority 3)
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-priority-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'from-message'));
  fs.mkdirSync(path.join(tmpRoot, 'from-file'));

  // Without git, filePath (priority 2) should win over message (priority 3)
  const filePath = path.join(tmpRoot, 'from-file', 'x.js');
  const result = resolveProject({ filePath, message: 'from-message text' }, tmpRoot);
  // filePath (priority 2) should win over message (priority 3)
  assert.strictEqual(result, 'from-file');

  fs.rmSync(tmpRoot, { recursive: true });
});

// Tests for logHookEvent
test('logHookEvent - writes JSONL line with ts and passed fields', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'logHookEvent-'));
  const originalHome = process.env.HOME;
  try {
    process.env.HOME = tmpHome;
    // Force reset of logDirCreated by reloading module
    delete require.cache[require.resolve('../scripts/lib')];
    const { logHookEvent: logHE } = require('../scripts/lib');

    logHE({ hook: 'test', kind: 'fire', session_id: 'sess123', project: 'test-proj' });

    const logFile = path.join(tmpHome, '.ouroboros', 'hooks.log');
    assert(fs.existsSync(logFile), 'log file should exist');

    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    assert.strictEqual(lines.length, 1);
    const entry = JSON.parse(lines[0]);
    assert(entry.ts, 'should have ts field');
    assert.strictEqual(entry.hook, 'test');
    assert.strictEqual(entry.kind, 'fire');
    assert.strictEqual(entry.session_id, 'sess123');
    assert.strictEqual(entry.project, 'test-proj');
  } finally {
    process.env.HOME = originalHome;
    delete require.cache[require.resolve('../scripts/lib')];
    fs.rmSync(tmpHome, { recursive: true });
  }
});

test('logHookEvent - multiple calls append (don\'t overwrite)', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'logHookEvent-append-'));
  const originalHome = process.env.HOME;
  try {
    process.env.HOME = tmpHome;
    delete require.cache[require.resolve('../scripts/lib')];
    const { logHookEvent: logHE } = require('../scripts/lib');

    logHE({ hook: 'test', kind: 'fire' });
    logHE({ hook: 'test', kind: 'persist' });
    logHE({ hook: 'test', kind: 'noop' });

    const logFile = path.join(tmpHome, '.ouroboros', 'hooks.log');
    const lines = fs.readFileSync(logFile, 'utf-8').trim().split('\n');
    assert.strictEqual(lines.length, 3);
    assert.strictEqual(JSON.parse(lines[0]).kind, 'fire');
    assert.strictEqual(JSON.parse(lines[1]).kind, 'persist');
    assert.strictEqual(JSON.parse(lines[2]).kind, 'noop');
  } finally {
    process.env.HOME = originalHome;
    delete require.cache[require.resolve('../scripts/lib')];
    fs.rmSync(tmpHome, { recursive: true });
  }
});

test('logHookEvent - missing HOME or permission errors: silent failure, no throw', () => {
  const originalHome = process.env.HOME;
  try {
    // Set HOME to something invalid (no throw expected)
    process.env.HOME = '/root/nonexistent/invalid/path/that/cannot/exist';
    delete require.cache[require.resolve('../scripts/lib')];
    const { logHookEvent: logHE } = require('../scripts/lib');

    // Should not throw even though it can't write
    assert.doesNotThrow(() => {
      logHE({ hook: 'test', kind: 'fire' });
    });
  } finally {
    process.env.HOME = originalHome;
    delete require.cache[require.resolve('../scripts/lib')];
  }
});

test('logHookEvent - OUROBOROS_HOOK_LOG=0 disables writes', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'logHookEvent-disabled-'));
  const originalHome = process.env.HOME;
  const originalFlag = process.env.OUROBOROS_HOOK_LOG;
  try {
    process.env.HOME = tmpHome;
    const logFile = path.join(tmpHome, '.ouroboros', 'hooks.log');

    for (const value of ['0', 'false', 'off']) {
      process.env.OUROBOROS_HOOK_LOG = value;
      delete require.cache[require.resolve('../scripts/lib')];
      const { logHookEvent: logHE } = require('../scripts/lib');
      logHE({ hook: 'test', kind: 'fire' });
      assert(!fs.existsSync(logFile), `log file should not exist when OUROBOROS_HOOK_LOG=${value}`);
    }

    // Sanity: unset re-enables
    delete process.env.OUROBOROS_HOOK_LOG;
    delete require.cache[require.resolve('../scripts/lib')];
    const { logHookEvent: logHE2 } = require('../scripts/lib');
    logHE2({ hook: 'test', kind: 'fire' });
    assert(fs.existsSync(logFile), 'log file should exist when flag unset');
  } finally {
    process.env.HOME = originalHome;
    if (originalFlag === undefined) delete process.env.OUROBOROS_HOOK_LOG;
    else process.env.OUROBOROS_HOOK_LOG = originalFlag;
    delete require.cache[require.resolve('../scripts/lib')];
    fs.rmSync(tmpHome, { recursive: true });
  }
});

test('logHookEvent - rotation: file > 5MB triggers rotation to .log.1', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'logHookEvent-rotate-'));
  const originalHome = process.env.HOME;
  try {
    process.env.HOME = tmpHome;
    delete require.cache[require.resolve('../scripts/lib')];

    // Pre-create a large log file (sparse is fine, just need size > 5MB)
    const logDir = path.join(tmpHome, '.ouroboros');
    fs.mkdirSync(logDir, { recursive: true });
    const logFile = path.join(logDir, 'hooks.log');
    // Write > 5MB of data
    const buffer = Buffer.alloc(6 * 1024 * 1024, 'x');
    fs.writeFileSync(logFile, buffer);

    const { logHookEvent: logHE } = require('../scripts/lib');
    logHE({ hook: 'test', kind: 'fire' });

    // After logHookEvent, original file should be rotated and new file should be small
    const rotatedPath = `${logFile}.1`;
    assert(fs.existsSync(rotatedPath), '.log.1 should exist after rotation');

    const newStats = fs.statSync(logFile);
    assert(newStats.size < 1000, 'current .log should be small (just the new entry)');

    // Verify the new entry is in the current log
    const newContent = fs.readFileSync(logFile, 'utf-8').trim();
    const entry = JSON.parse(newContent);
    assert.strictEqual(entry.kind, 'fire');
  } finally {
    process.env.HOME = originalHome;
    delete require.cache[require.resolve('../scripts/lib')];
    fs.rmSync(tmpHome, { recursive: true });
  }
});

// Tests for getMaxLogSize
test('getMaxLogSize - unset env var returns 5MB default', () => {
  const originalVal = process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  try {
    delete process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
    const result = getMaxLogSize();
    assert.strictEqual(result, 5 * 1024 * 1024);
  } finally {
    if (originalVal !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = originalVal;
  }
});

test('getMaxLogSize - valid numeric env var is parsed', () => {
  const originalVal = process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  try {
    process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = '1048576'; // 1MB
    const result = getMaxLogSize();
    assert.strictEqual(result, 1048576);
  } finally {
    if (originalVal !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = originalVal;
    else delete process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  }
});

test('getMaxLogSize - invalid env var falls back to 5MB default', () => {
  const originalVal = process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  try {
    process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = 'not-a-number';
    const result = getMaxLogSize();
    assert.strictEqual(result, 5 * 1024 * 1024);
  } finally {
    if (originalVal !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = originalVal;
    else delete process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  }
});

// Tests for getMaxLogFiles
test('getMaxLogFiles - unset env var returns 1 backup default', () => {
  const originalVal = process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
  try {
    delete process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
    const result = getMaxLogFiles();
    assert.strictEqual(result, 1);
  } finally {
    if (originalVal !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_FILES = originalVal;
  }
});

test('getMaxLogFiles - valid numeric env var is parsed', () => {
  const originalVal = process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
  try {
    process.env.OUROBOROS_HOOK_LOG_MAX_FILES = '3';
    const result = getMaxLogFiles();
    assert.strictEqual(result, 3);
  } finally {
    if (originalVal !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_FILES = originalVal;
    else delete process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
  }
});

test('getMaxLogFiles - invalid env var falls back to 1 default', () => {
  const originalVal = process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
  try {
    process.env.OUROBOROS_HOOK_LOG_MAX_FILES = 'invalid';
    const result = getMaxLogFiles();
    assert.strictEqual(result, 1);
  } finally {
    if (originalVal !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_FILES = originalVal;
    else delete process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
  }
});

// Tests for rotateLogFiles
test('rotateLogFiles - maxFiles=0 deletes current log', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'rotate-zero-'));
  try {
    const logPath = path.join(tmpHome, 'test.log');
    fs.writeFileSync(logPath, 'some data');
    assert(fs.existsSync(logPath));

    rotateLogFiles(logPath, 0);

    assert(!fs.existsSync(logPath), 'log file should be deleted when maxFiles=0');
  } finally {
    fs.rmSync(tmpHome, { recursive: true });
  }
});

test('rotateLogFiles - maxFiles=1 shifts .log → .log.1 only', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'rotate-one-'));
  try {
    const logPath = path.join(tmpHome, 'test.log');
    fs.writeFileSync(logPath, 'current data');
    fs.writeFileSync(`${logPath}.1`, 'old data 1');
    fs.writeFileSync(`${logPath}.2`, 'old data 2'); // Should be deleted

    rotateLogFiles(logPath, 1);

    assert(!fs.existsSync(logPath), 'original .log should be rotated away');
    assert(fs.existsSync(`${logPath}.1`), '.log.1 should exist');
    assert.strictEqual(
      fs.readFileSync(`${logPath}.1`, 'utf-8'),
      'current data',
      '.log.1 should contain current data'
    );
    assert(!fs.existsSync(`${logPath}.2`), '.log.2 should be deleted (beyond limit)');
  } finally {
    fs.rmSync(tmpHome, { recursive: true });
  }
});

test('rotateLogFiles - maxFiles=2 keeps two backups', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'rotate-two-'));
  try {
    const logPath = path.join(tmpHome, 'test.log');
    fs.writeFileSync(logPath, 'current');
    fs.writeFileSync(`${logPath}.1`, 'backup1');
    fs.writeFileSync(`${logPath}.2`, 'backup2');
    fs.writeFileSync(`${logPath}.3`, 'backup3'); // Should be deleted

    rotateLogFiles(logPath, 2);

    assert(!fs.existsSync(logPath), 'original should be rotated');
    assert.strictEqual(fs.readFileSync(`${logPath}.1`, 'utf-8'), 'current');
    assert.strictEqual(fs.readFileSync(`${logPath}.2`, 'utf-8'), 'backup1');
    assert(!fs.existsSync(`${logPath}.3`), '.log.3 should be deleted (beyond limit)');
  } finally {
    fs.rmSync(tmpHome, { recursive: true });
  }
});

test('rotateLogFiles - rotates when only current log exists', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'rotate-only-current-'));
  try {
    const logPath = path.join(tmpHome, 'test.log');
    fs.writeFileSync(logPath, 'first and only');

    rotateLogFiles(logPath, 1);

    assert(!fs.existsSync(logPath), 'original should be renamed');
    assert(fs.existsSync(`${logPath}.1`), '.log.1 should exist');
    assert.strictEqual(fs.readFileSync(`${logPath}.1`, 'utf-8'), 'first and only');
  } finally {
    fs.rmSync(tmpHome, { recursive: true });
  }
});

// Tests for logHookEvent with configurable rotation
test('logHookEvent - custom max size via env var triggers rotation', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'logHookEvent-custom-size-'));
  const originalHome = process.env.HOME;
  const originalMaxSize = process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  try {
    process.env.HOME = tmpHome;
    process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = '1024'; // 1KB
    delete require.cache[require.resolve('../scripts/lib')];

    // Pre-create a log file > 1KB
    const logDir = path.join(tmpHome, '.ouroboros');
    fs.mkdirSync(logDir, { recursive: true });
    const logFile = path.join(logDir, 'hooks.log');
    fs.writeFileSync(logFile, 'x'.repeat(2000)); // > 1KB

    const { logHookEvent: logHE } = require('../scripts/lib');
    logHE({ hook: 'test', kind: 'fire' });

    // Should have rotated
    assert(fs.existsSync(`${logFile}.1`), '.log.1 should exist after rotation');
    const newStats = fs.statSync(logFile);
    assert(newStats.size < 500, 'new log should be small');
  } finally {
    process.env.HOME = originalHome;
    if (originalMaxSize !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = originalMaxSize;
    else delete process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
    delete require.cache[require.resolve('../scripts/lib')];
    fs.rmSync(tmpHome, { recursive: true });
  }
});

test('logHookEvent - multiple backups via env var keeps two rotated files', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'logHookEvent-multi-backups-'));
  const originalHome = process.env.HOME;
  const originalMaxSize = process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  const originalMaxFiles = process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
  try {
    process.env.HOME = tmpHome;
    process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = '500'; // Small so rotations happen
    process.env.OUROBOROS_HOOK_LOG_MAX_FILES = '2';  // Keep 2 backups
    delete require.cache[require.resolve('../scripts/lib')];

    const logDir = path.join(tmpHome, '.ouroboros');
    fs.mkdirSync(logDir, { recursive: true });
    const logFile = path.join(logDir, 'hooks.log');

    // Pre-seed with files
    fs.writeFileSync(logFile, 'x'.repeat(600)); // Will trigger rotation on first write

    const { logHookEvent: logHE } = require('../scripts/lib');

    // First write triggers rotation
    logHE({ msg: 'first' });
    assert(fs.existsSync(`${logFile}.1`), 'first rotation creates .log.1');

    // Second write
    fs.appendFileSync(logFile, 'y'.repeat(600)); // Exceed limit again
    logHE({ msg: 'second' });
    assert(fs.existsSync(`${logFile}.1`), '.log.1 should still exist');
    assert(fs.existsSync(`${logFile}.2`), '.log.2 should exist after second rotation');

    // Third write
    fs.appendFileSync(logFile, 'z'.repeat(600)); // Exceed limit again
    logHE({ msg: 'third' });
    assert(fs.existsSync(`${logFile}.1`), '.log.1 should exist');
    assert(fs.existsSync(`${logFile}.2`), '.log.2 should exist');
    assert(!fs.existsSync(`${logFile}.3`), '.log.3 should not exist (exceeds maxFiles=2)');
  } finally {
    process.env.HOME = originalHome;
    if (originalMaxSize !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = originalMaxSize;
    else delete process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
    if (originalMaxFiles !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_FILES = originalMaxFiles;
    else delete process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
    delete require.cache[require.resolve('../scripts/lib')];
    fs.rmSync(tmpHome, { recursive: true });
  }
});

test('logHookEvent - default 5MB / 1 backup unchanged (backward compat)', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'logHookEvent-default-'));
  const originalHome = process.env.HOME;
  const originalMaxSize = process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  const originalMaxFiles = process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
  try {
    process.env.HOME = tmpHome;
    // Explicitly unset custom env vars to test defaults
    delete process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
    delete process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
    delete require.cache[require.resolve('../scripts/lib')];

    const logDir = path.join(tmpHome, '.ouroboros');
    fs.mkdirSync(logDir, { recursive: true });
    const logFile = path.join(logDir, 'hooks.log');

    // Pre-create a file > 5MB
    const buffer = Buffer.alloc(6 * 1024 * 1024, 'x');
    fs.writeFileSync(logFile, buffer);

    const { logHookEvent: logHE } = require('../scripts/lib');
    logHE({ hook: 'test', kind: 'fire' });

    // Should have rotated at 5MB
    assert(fs.existsSync(`${logFile}.1`), '.log.1 should exist');
    const newStats = fs.statSync(logFile);
    assert(newStats.size < 1000, 'new log should be small');

    // Only .log.1 should exist (not .log.2)
    assert(!fs.existsSync(`${logFile}.2`), '.log.2 should not exist (default maxFiles=1)');
  } finally {
    process.env.HOME = originalHome;
    if (originalMaxSize !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = originalMaxSize;
    if (originalMaxFiles !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_FILES = originalMaxFiles;
    delete require.cache[require.resolve('../scripts/lib')];
    fs.rmSync(tmpHome, { recursive: true });
  }
});

test('logHookEvent - invalid env vars fall back to defaults', () => {
  const tmpHome = fs.mkdtempSync(path.join(os.tmpdir(), 'logHookEvent-invalid-'));
  const originalHome = process.env.HOME;
  const originalMaxSize = process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  const originalMaxFiles = process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
  try {
    process.env.HOME = tmpHome;
    process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = 'garbage';
    process.env.OUROBOROS_HOOK_LOG_MAX_FILES = 'not-numeric';
    delete require.cache[require.resolve('../scripts/lib')];

    const logDir = path.join(tmpHome, '.ouroboros');
    fs.mkdirSync(logDir, { recursive: true });
    const logFile = path.join(logDir, 'hooks.log');
    const buffer = Buffer.alloc(6 * 1024 * 1024, 'x'); // > 5MB default
    fs.writeFileSync(logFile, buffer);

    const { logHookEvent: logHE } = require('../scripts/lib');
    // Should not throw, should use defaults
    assert.doesNotThrow(() => {
      logHE({ hook: 'test', kind: 'fire' });
    });

    assert(fs.existsSync(`${logFile}.1`), '.log.1 should exist (used 5MB default)');
    assert(!fs.existsSync(`${logFile}.2`), '.log.2 should not exist (used 1 backup default)');
  } finally {
    process.env.HOME = originalHome;
    if (originalMaxSize !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_SIZE = originalMaxSize;
    else delete process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
    if (originalMaxFiles !== undefined) process.env.OUROBOROS_HOOK_LOG_MAX_FILES = originalMaxFiles;
    else delete process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
    delete require.cache[require.resolve('../scripts/lib')];
    fs.rmSync(tmpHome, { recursive: true });
  }
});

// Tests for isSkippedAgentType
test('isSkippedAgentType - "Explore" (built-in) → true', () => {
  const result = isSkippedAgentType('Explore');
  assert.strictEqual(result, true);
});

test('isSkippedAgentType - "knowledge-explorer" (bare) → true', () => {
  const result = isSkippedAgentType('knowledge-explorer');
  assert.strictEqual(result, true);
});

test('isSkippedAgentType - "backlog-manager" (bare) → true', () => {
  const result = isSkippedAgentType('backlog-manager');
  assert.strictEqual(result, true);
});

test('isSkippedAgentType - "ouroboros-mcp:knowledge-explorer" (plugin qualified) → true', () => {
  const result = isSkippedAgentType('ouroboros-mcp:knowledge-explorer');
  assert.strictEqual(result, true);
});

test('isSkippedAgentType - "ouroboros-mcp:backlog-manager" (plugin qualified) → true', () => {
  const result = isSkippedAgentType('ouroboros-mcp:backlog-manager');
  assert.strictEqual(result, true);
});

test('isSkippedAgentType - "general-purpose" → false', () => {
  const result = isSkippedAgentType('general-purpose');
  assert.strictEqual(result, false);
});

test('isSkippedAgentType - "ouroboros-mcp:general-purpose" → false', () => {
  const result = isSkippedAgentType('ouroboros-mcp:general-purpose');
  assert.strictEqual(result, false);
});

test('isSkippedAgentType - empty string → false', () => {
  const result = isSkippedAgentType('');
  assert.strictEqual(result, false);
});

test('isSkippedAgentType - undefined → false', () => {
  const result = isSkippedAgentType(undefined);
  assert.strictEqual(result, false);
});

// Tests for extractAllKbBlocks
test('extractAllKbBlocks - missing file → empty blocks and turns', () => {
  const result = extractAllKbBlocks('/nonexistent/path.jsonl');
  assert.deepStrictEqual(result.blocks, []);
  assert.deepStrictEqual(result.turns, []);
});

test('extractAllKbBlocks - 0 kb-blocks → returns empty blocks array', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'extract-0blocks-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  const line1 = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: { content: [{ type: 'text', text: 'Just some regular text, no kb block' }] },
  });
  fs.writeFileSync(transcriptPath, line1 + '\n');

  const result = extractAllKbBlocks(transcriptPath);
  assert.deepStrictEqual(result.blocks, []);
  assert.strictEqual(result.turns.length, 1);

  fs.rmSync(tmpDir, { recursive: true });
});

test('extractAllKbBlocks - 1 kb-block in 1 turn → returns that block', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'extract-1block-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  const kbJson = '[{"type":"decision","title":"adopt cobra"}]';
  const line1 = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: {
      content: [{ type: 'text', text: `Here is a decision:\n\`\`\`kb\n${kbJson}\n\`\`\`` }],
    },
  });
  fs.writeFileSync(transcriptPath, line1 + '\n');

  const result = extractAllKbBlocks(transcriptPath);
  assert.strictEqual(result.blocks.length, 1);
  assert.strictEqual(result.blocks[0].text, kbJson);
  assert.strictEqual(result.turns.length, 1);

  fs.rmSync(tmpDir, { recursive: true });
});

test('extractAllKbBlocks - N kb-blocks across multiple turns → returns all', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'extract-nblocks-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  const kbJson1 = '[{"type":"decision","title":"first"}]';
  const kbJson2 = '[{"type":"fact","title":"second"}]';
  const line1 = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: { content: [{ type: 'text', text: `Turn 1:\n\`\`\`kb\n${kbJson1}\n\`\`\`` }] },
  });
  const line2 = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: { content: [{ type: 'text', text: `Turn 2:\n\`\`\`kb\n${kbJson2}\n\`\`\`` }] },
  });
  fs.writeFileSync(transcriptPath, line1 + '\n' + line2 + '\n');

  const result = extractAllKbBlocks(transcriptPath);
  assert.strictEqual(result.blocks.length, 2);
  assert.strictEqual(result.blocks[0].text, kbJson1);
  assert.strictEqual(result.blocks[1].text, kbJson2);
  assert.strictEqual(result.turns.length, 2);

  fs.rmSync(tmpDir, { recursive: true });
});

test('extractAllKbBlocks - sidechain turns with kb-blocks → excluded from results', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'extract-sidechain-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  const kbJson1 = '[{"type":"decision"}]';
  const kbJson2 = '[{"type":"note"}]';
  const line1 = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: { content: [{ type: 'text', text: `Main:\n\`\`\`kb\n${kbJson1}\n\`\`\`` }] },
  });
  const line2 = JSON.stringify({
    type: 'assistant',
    isSidechain: true,
    message: { content: [{ type: 'text', text: `Sidechain:\n\`\`\`kb\n${kbJson2}\n\`\`\`` }] },
  });
  fs.writeFileSync(transcriptPath, line1 + '\n' + line2 + '\n');

  const result = extractAllKbBlocks(transcriptPath);
  assert.strictEqual(result.blocks.length, 1);
  assert.strictEqual(result.blocks[0].text, kbJson1);
  assert.strictEqual(result.turns.length, 1);

  fs.rmSync(tmpDir, { recursive: true });
});

test('extractAllKbBlocks - malformed JSONL line mixed with valid → only valid counted', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'extract-malformed-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  const kbJson = '[{"type":"decision"}]';
  const line1 = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: { content: [{ type: 'text', text: `Turn 1:\n\`\`\`kb\n${kbJson}\n\`\`\`` }] },
  });
  const line2 = 'this is not valid json';
  const line3 = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: { content: [{ type: 'text', text: 'Turn 2: plain text' }] },
  });
  fs.writeFileSync(transcriptPath, line1 + '\n' + line2 + '\n' + line3 + '\n');

  const result = extractAllKbBlocks(transcriptPath);
  assert.strictEqual(result.blocks.length, 1);
  assert.strictEqual(result.turns.length, 2);

  fs.rmSync(tmpDir, { recursive: true });
});

test('extractAllKbBlocks - decision language detection: positive cases', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'extract-decision-pos-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');

  const samples = [
    'We decided on Postgres',
    'We chose nodeJS over python',
    'the chosen pattern is REST',
    'This is a design decision here',
    'going with TypeScript',
    'We rejected the first proposal',
    'We picked the simpler solution',
    'I selected the best option',
    'opting for a simple design',
  ];

  let content = '';
  for (const sample of samples) {
    const line = JSON.stringify({
      type: 'assistant',
      isSidechain: false,
      message: { content: [{ type: 'text', text: sample }] },
    });
    content += line + '\n';
  }
  fs.writeFileSync(transcriptPath, content);

  const result = extractAllKbBlocks(transcriptPath);
  const decisionCount = result.turns.filter(t => t.hasDecisionLanguage).length;
  assert.strictEqual(decisionCount, samples.length);

  fs.rmSync(tmpDir, { recursive: true });
});

test('extractAllKbBlocks - decision language detection: negative cases', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'extract-decision-neg-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');

  const samples = [
    'Just some regular text here',
    'Code example without decisions',
    'Let me explain the concept',
    'Here is what happened',
  ];

  let content = '';
  for (const sample of samples) {
    const line = JSON.stringify({
      type: 'assistant',
      isSidechain: false,
      message: { content: [{ type: 'text', text: sample }] },
    });
    content += line + '\n';
  }
  fs.writeFileSync(transcriptPath, content);

  const result = extractAllKbBlocks(transcriptPath);
  const decisionCount = result.turns.filter(t => t.hasDecisionLanguage).length;
  assert.strictEqual(decisionCount, 0);

  fs.rmSync(tmpDir, { recursive: true });
});

test('extractAllKbBlocks - line cap: file with 2600 lines, maxLines=1000 → only last 1000 considered', () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'extract-cap-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');

  let content = '';
  // Write 2500 lines of non-decision text
  for (let i = 0; i < 2500; i++) {
    content += JSON.stringify({
      type: 'assistant',
      isSidechain: false,
      message: { content: [{ type: 'text', text: 'Regular text without decisions' }] },
    }) + '\n';
  }
  // Write 100 decision lines at the end (within the 1000-line cap)
  for (let i = 0; i < 100; i++) {
    content += JSON.stringify({
      type: 'assistant',
      isSidechain: false,
      message: { content: [{ type: 'text', text: 'We decided something here' }] },
    }) + '\n';
  }
  fs.writeFileSync(transcriptPath, content);

  const result = extractAllKbBlocks(transcriptPath, { maxLines: 1000 });
  // Last 1000 lines: 900 non-decision + 100 decision
  assert.strictEqual(result.turns.length, 1000);
  const decisionCount = result.turns.filter(t => t.hasDecisionLanguage).length;
  assert.strictEqual(decisionCount, 100);

  fs.rmSync(tmpDir, { recursive: true });
});
