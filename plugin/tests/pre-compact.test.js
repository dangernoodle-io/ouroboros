const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');
const { spawn } = require('child_process');

function runPreCompact(inputData, env = {}) {
  return new Promise((resolve, reject) => {
    const proc = spawn('node', [path.join(__dirname, '../scripts/pre-compact.js')], {
      env: { ...process.env, ...env },
    });

    let stdout = '';
    let stderr = '';

    proc.stdout.on('data', (data) => {
      stdout += data.toString();
    });

    proc.stderr.on('data', (data) => {
      stderr += data.toString();
    });

    proc.on('close', (code) => {
      resolve({ code, stdout, stderr });
    });

    proc.on('error', reject);

    proc.stdin.write(JSON.stringify(inputData));
    proc.stdin.end();
  });
}

// Helper: write a fake ouroboros binary under pluginDataDir/bin/ouroboros.
// Returns the pluginDataDir to pass as CLAUDE_PLUGIN_DATA env var.
function fakeBinary(pluginDataDir, jsonResponse) {
  const binDir = path.join(pluginDataDir, 'bin');
  fs.mkdirSync(binDir, { recursive: true });
  const binPath = path.join(binDir, 'ouroboros');
  fs.writeFileSync(binPath, `#!/usr/bin/env node\nprocess.stdout.write(JSON.stringify(${JSON.stringify(jsonResponse)}));\n`);
  fs.chmodSync(binPath, '755');
  return pluginDataDir;
}

function makeTmpGit() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-git-'));
  fs.mkdirSync(path.join(dir, '.git'));
  return dir;
}

function makeTranscriptWithKbBlocks(count) {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-tx-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  let content = '';
  for (let i = 0; i < count; i++) {
    const kbJson = `[{"type":"decision","title":"Decision ${i}","content":"content ${i}"}]`;
    content += JSON.stringify({
      type: 'assistant',
      isSidechain: false,
      message: {
        content: [{ type: 'text', text: `Decision:\n\`\`\`kb\n${kbJson}\n\`\`\`` }],
      },
    }) + '\n';
  }
  fs.writeFileSync(transcriptPath, content);
  return { tmpDir, transcriptPath };
}

function makeTranscriptNoBlocks(decisionTurns, customText = null) {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-nodec-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  let content = '';
  for (let i = 0; i < decisionTurns; i++) {
    const text = customText || 'We decided to use TypeScript';
    content += JSON.stringify({
      type: 'assistant',
      isSidechain: false,
      message: { content: [{ type: 'text', text }] },
    }) + '\n';
  }
  if (decisionTurns === 0) {
    content += JSON.stringify({
      type: 'assistant',
      isSidechain: false,
      message: { content: [{ type: 'text', text: 'Just some regular text' }] },
    }) + '\n';
  }
  fs.writeFileSync(transcriptPath, content);
  return { tmpDir, transcriptPath };
}

// --- Tool path (no kb-blocks, but persisted via tool) ---

test('pre-compact - no kb-blocks, ≥1 persisted docs via tool → allow', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(0);
  const gitDir = makeTmpGit();
  const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-bin-'));

  // Fake binary returns 1 entry (persisted via /persist tool)
  fakeBinary(binDir, [{ id: 1 }]);

  const result = await runPreCompact(
    {
      transcript_path: transcriptPath,
      cwd: gitDir,
      session_id: 'sess-tool-001',
      trigger: 'manual',
    },
    { CLAUDE_PLUGIN_DATA: binDir }
  );

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
  fs.rmSync(binDir, { recursive: true });
});

test('pre-compact - no kb-blocks, 0 persisted docs, decision language present → block', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(3);
  const gitDir = makeTmpGit();
  const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-bin-'));

  // Fake binary returns empty array (query succeeded, no docs found)
  fakeBinary(binDir, []);

  const result = await runPreCompact(
    {
      transcript_path: transcriptPath,
      cwd: gitDir,
      session_id: 'sess-tool-002',
      trigger: 'manual',
    },
    { CLAUDE_PLUGIN_DATA: binDir }
  );

  assert.strictEqual(result.code, 0);
  assert(result.stdout.length > 0);
  const output = JSON.parse(result.stdout);
  assert.strictEqual(output.decision, 'block');
  assert(output.reason.includes('unpersisted decisions'));

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
  fs.rmSync(binDir, { recursive: true });
});

test('pre-compact - no kb-blocks, no session_id → heuristic (decision language)', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(3);
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: gitDir,
    // no session_id
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert(result.stdout.length > 0);
  const output = JSON.parse(result.stdout);
  assert.strictEqual(output.decision, 'block');
  assert(output.reason.includes('unpersisted decisions'));

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
});

test('pre-compact - no kb-blocks, query error → heuristic (decision language)', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(3);
  const gitDir = makeTmpGit();
  const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-bin-'));

  // Fake binary that exits with error
  const binSubDir = path.join(binDir, 'bin');
  fs.mkdirSync(binSubDir, { recursive: true });
  const binPath = path.join(binSubDir, 'ouroboros');
  fs.writeFileSync(binPath, '#!/usr/bin/env node\nprocess.exit(1);\n');
  fs.chmodSync(binPath, '755');

  const result = await runPreCompact(
    {
      transcript_path: transcriptPath,
      cwd: gitDir,
      session_id: 'sess-error-tool-001',
      trigger: 'manual',
    },
    { CLAUDE_PLUGIN_DATA: binDir }
  );

  assert.strictEqual(result.code, 0);
  assert(result.stdout.length > 0);
  const output = JSON.parse(result.stdout);
  assert.strictEqual(output.decision, 'block');
  assert(output.reason.includes('unpersisted decisions'));

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
  fs.rmSync(binDir, { recursive: true });
});

// --- Heuristic path (no kb-blocks) ---

test('pre-compact - no kb-blocks, no decision language → allow', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(0);
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: gitDir,
    session_id: 'test-2',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
});

test('pre-compact - no kb-blocks, 1 decision turn, manual trigger (threshold=3) → allow', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(1);
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: gitDir,
    session_id: 'test-3',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
});

test('pre-compact - no kb-blocks, 2 decision turns, auto trigger (threshold=3) → allow', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(2);
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: gitDir,
    session_id: 'test-4',
    trigger: 'auto',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
});

test('pre-compact - no kb-blocks, 3 decision turns, auto trigger (threshold=3) → block', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(3);
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: gitDir,
    session_id: 'test-5',
    trigger: 'auto',
  });

  assert.strictEqual(result.code, 0);
  assert(result.stdout.length > 0);
  const output = JSON.parse(result.stdout);
  assert.strictEqual(output.decision, 'block');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
});

// --- New tests for loosened decision detection (removed /\buse\b/ and /\bapproach\b/ patterns) ---

test('pre-compact - only "use"/"approach" language, no true decision words → allow', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(2, 'I will use the Read tool. This approach is sound.');
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: gitDir,
    session_id: 'test-use-approach',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
});

test('pre-compact - no kb-blocks, 1-2 decision turns, manual trigger (threshold=3) → allow', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(2, 'We decided to use TypeScript');
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: gitDir,
    session_id: 'test-2-decisions-manual',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
});

test('pre-compact - no kb-blocks, 3+ decision turns, manual trigger (threshold=3) → block', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptNoBlocks(3, 'We decided to use TypeScript');
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: gitDir,
    session_id: 'test-3-decisions-manual',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert(result.stdout.length > 0);
  const output = JSON.parse(result.stdout);
  assert.strictEqual(output.decision, 'block');
  assert(output.reason.includes('unpersisted decisions'));

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
});

// --- Session_id diffing path (kb-blocks present) ---

test('pre-compact - kb-blocks present, all persisted (persisted >= blocks) → allow', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptWithKbBlocks(2);
  const gitDir = makeTmpGit();
  const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-bin-'));

  // Fake binary returns 2 entries (matches block count)
  fakeBinary(binDir, [{ id: 1 }, { id: 2 }]);

  const result = await runPreCompact(
    {
      transcript_path: transcriptPath,
      cwd: gitDir,
      session_id: 'sess-allow-001',
      trigger: 'manual',
    },
    { CLAUDE_PLUGIN_DATA: binDir }
  );

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
  fs.rmSync(binDir, { recursive: true });
});

test('pre-compact - kb-blocks present, more persisted than blocks → allow', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptWithKbBlocks(1);
  const gitDir = makeTmpGit();
  const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-bin-'));

  // Fake binary returns 3 entries (more than block count of 1)
  fakeBinary(binDir, [{ id: 1 }, { id: 2 }, { id: 3 }]);

  const result = await runPreCompact(
    {
      transcript_path: transcriptPath,
      cwd: gitDir,
      session_id: 'sess-allow-002',
      trigger: 'manual',
    },
    { CLAUDE_PLUGIN_DATA: binDir }
  );

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
  fs.rmSync(binDir, { recursive: true });
});

test('pre-compact - kb-blocks present, fewer persisted than blocks → block', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptWithKbBlocks(3);
  const gitDir = makeTmpGit();
  const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-bin-'));

  // Fake binary returns 1 entry (less than block count of 3)
  fakeBinary(binDir, [{ id: 1 }]);

  const result = await runPreCompact(
    {
      transcript_path: transcriptPath,
      cwd: gitDir,
      session_id: 'sess-block-001',
      trigger: 'manual',
    },
    { CLAUDE_PLUGIN_DATA: binDir }
  );

  assert.strictEqual(result.code, 0);
  assert(result.stdout.length > 0);
  const output = JSON.parse(result.stdout);
  assert.strictEqual(output.decision, 'block');
  assert(output.reason.includes('2 of 3 kb-blocks unpersisted'));

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
  fs.rmSync(binDir, { recursive: true });
});

test('pre-compact - kb-blocks present, zero persisted → block', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptWithKbBlocks(2);
  const gitDir = makeTmpGit();
  const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-bin-'));

  // Fake binary returns empty array
  fakeBinary(binDir, []);

  const result = await runPreCompact(
    {
      transcript_path: transcriptPath,
      cwd: gitDir,
      session_id: 'sess-block-002',
      trigger: 'manual',
    },
    { CLAUDE_PLUGIN_DATA: binDir }
  );

  assert.strictEqual(result.code, 0);
  assert(result.stdout.length > 0);
  const output = JSON.parse(result.stdout);
  assert.strictEqual(output.decision, 'block');
  assert(output.reason.includes('2 of 2 kb-blocks unpersisted'));

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
  fs.rmSync(binDir, { recursive: true });
});

test('pre-compact - kb-blocks present, query error → fail-open (allow)', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptWithKbBlocks(2);
  const gitDir = makeTmpGit();
  const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-bin-'));

  // Fake binary that exits with error
  const binSubDir = path.join(binDir, 'bin');
  fs.mkdirSync(binSubDir, { recursive: true });
  const binPath = path.join(binSubDir, 'ouroboros');
  fs.writeFileSync(binPath, '#!/usr/bin/env node\nprocess.exit(1);\n');
  fs.chmodSync(binPath, '755');

  const result = await runPreCompact(
    {
      transcript_path: transcriptPath,
      cwd: gitDir,
      session_id: 'sess-error-001',
      trigger: 'manual',
    },
    { CLAUDE_PLUGIN_DATA: binDir }
  );

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
  fs.rmSync(binDir, { recursive: true });
});

test('pre-compact - kb-blocks present, no session_id → allow (cannot diff)', async () => {
  const { tmpDir, transcriptPath } = makeTranscriptWithKbBlocks(2);
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: gitDir,
    // no session_id
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(gitDir, { recursive: true });
});

// --- Edge cases ---

test('pre-compact - missing transcript_path → exit 0, no stdout', async () => {
  const gitDir = makeTmpGit();

  const result = await runPreCompact({
    cwd: gitDir,
    session_id: 'test-6',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(gitDir, { recursive: true });
});

test('pre-compact - missing cwd / project → exit 0, no stdout', async () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-noproj-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  fs.writeFileSync(transcriptPath, '');

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    session_id: 'test-7',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
});

test('pre-compact - internal error → exit 0 (fail-open)', async () => {
  const result = await runPreCompact({
    transcript_path: null,
    cwd: null,
    session_id: 'test-8',
  });

  assert.strictEqual(result.code, 0);
});
