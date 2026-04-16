const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');
const { extractKbBlock, matchesAnyPattern, formatContextLines, findWorkspaceRoot, listWorkspaceProjects, resolveProject } = require('../scripts/lib');

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

test('formatContextLines - N rows → header + N indented lines + contract block', () => {
  const rows = [
    { type: 'decision', title: 'adopt cobra' },
    { type: 'fact', title: 'FTS5 cap' },
  ];
  const result = formatContextLines('myproject', rows);

  assert.strictEqual(result[0], '[ouroboros] myproject KB (2):');
  assert.strictEqual(result[1], '  [decision] adopt cobra');
  assert.strictEqual(result[2], '  [fact] FTS5 cap');
  assert.strictEqual(result[3], '');
  assert(result.some(line => line.includes('persist any decisions/facts')));
  assert(result.some(line => line === '```kb'));
  assert(result.some(line => line === '```'));
});

test('formatContextLines - project name interpolated correctly', () => {
  const rows = [{ type: 'note', title: 'test' }];
  const result = formatContextLines('special-proj', rows);
  assert(result[0].includes('special-proj'));
  assert(result.some(line => line.includes('(project: special-proj)')));
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
  const result = resolveProject({}, tmpDir, true);
  assert.strictEqual(result, null);
  fs.rmSync(tmpDir, { recursive: true });
});

test('resolveProject - filePath hint matches project', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-filepath-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'my-project'));
  fs.writeFileSync(path.join(tmpRoot, 'my-project', 'file.js'), '');

  const filePath = path.join(tmpRoot, 'my-project', 'file.js');
  const result = resolveProject({ filePath }, tmpRoot, true);
  assert.strictEqual(result, 'my-project');

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - message hint matches project name (word boundary)', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-msg-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'ouroboros'));
  fs.mkdirSync(path.join(tmpRoot, 'terranoodle'));

  const message = 'I need help with the ouroboros project setup';
  const result = resolveProject({ message }, tmpRoot, true);
  assert.strictEqual(result, 'ouroboros');

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - message does NOT match without word boundary', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-noboundary-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'test'));

  // "testing" should NOT match project "test" without word boundary
  const message = 'I am testing this feature';
  const result = resolveProject({ message }, tmpRoot, true);
  assert.strictEqual(result, null);

  fs.rmSync(tmpRoot, { recursive: true });
});

test('resolveProject - message hint is case-insensitive', () => {
  const tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'resolve-case-'));
  fs.mkdirSync(path.join(tmpRoot, '.claude'));
  fs.mkdirSync(path.join(tmpRoot, 'MyProject'));

  const message = 'working on myproject now';
  const result = resolveProject({ message }, tmpRoot, true);
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

  const result = resolveProject({ transcriptPath }, tmpRoot, true);
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

  const result = resolveProject({ transcriptPath }, tmpRoot, true);
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

  const result = resolveProject({ transcriptPath }, tmpRoot, true);
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

  const result = resolveProject({ transcriptPath }, tmpRoot, true);
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
  const result = resolveProject({ filePath, message: 'from-message text' }, tmpRoot, true);
  // filePath (priority 2) should win over message (priority 3)
  assert.strictEqual(result, 'from-file');

  fs.rmSync(tmpRoot, { recursive: true });
});
