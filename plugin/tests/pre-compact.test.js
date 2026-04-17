const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');
const { spawn } = require('child_process');

function runPreCompact(inputData) {
  return new Promise((resolve, reject) => {
    const proc = spawn('node', [path.join(__dirname, '../scripts/pre-compact.js')]);

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

test('pre-compact - transcript with kb-blocks present → exit 0, no stdout', async () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-kb-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  const kbJson = '[{"type":"decision"}]';
  const line = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: {
      content: [{ type: 'text', text: `Decision:\n\`\`\`kb\n${kbJson}\n\`\`\`` }],
    },
  });
  fs.writeFileSync(transcriptPath, line + '\n');

  const tmpGit = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-git-'));
  fs.mkdirSync(path.join(tmpGit, '.git'));

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: tmpGit,
    session_id: 'test-1',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(tmpGit, { recursive: true });
});

test('pre-compact - no kb-blocks, no decision language, manual trigger → exit 0, no stdout', async () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-no-dec-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  const line = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: { content: [{ type: 'text', text: 'Just some regular text' }] },
  });
  fs.writeFileSync(transcriptPath, line + '\n');

  const tmpGit = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-git2-'));
  fs.mkdirSync(path.join(tmpGit, '.git'));

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: tmpGit,
    session_id: 'test-2',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(tmpGit, { recursive: true });
});

test('pre-compact - 1 decision turn, manual trigger (threshold=1) → stdout contains decision:block', async () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-1dec-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  const line = JSON.stringify({
    type: 'assistant',
    isSidechain: false,
    message: { content: [{ type: 'text', text: 'We decided to use TypeScript' }] },
  });
  fs.writeFileSync(transcriptPath, line + '\n');

  const tmpGit = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-git3-'));
  fs.mkdirSync(path.join(tmpGit, '.git'));

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: tmpGit,
    session_id: 'test-3',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert(result.stdout.length > 0);
  const output = JSON.parse(result.stdout);
  assert.strictEqual(output.decision, 'block');
  assert(output.reason.includes('unpersisted decisions'));

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(tmpGit, { recursive: true });
});

test('pre-compact - 2 decision turns, auto trigger (threshold=3) → exit 0, no stdout', async () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-2dec-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  let content = '';
  for (let i = 0; i < 2; i++) {
    content += JSON.stringify({
      type: 'assistant',
      isSidechain: false,
      message: { content: [{ type: 'text', text: 'We decided to use something' }] },
    }) + '\n';
  }
  fs.writeFileSync(transcriptPath, content);

  const tmpGit = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-git4-'));
  fs.mkdirSync(path.join(tmpGit, '.git'));

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: tmpGit,
    session_id: 'test-4',
    trigger: 'auto',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(tmpGit, { recursive: true });
});

test('pre-compact - 3 decision turns, auto trigger (threshold=3) → stdout contains decision:block', async () => {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-3dec-'));
  const transcriptPath = path.join(tmpDir, 'trans.jsonl');
  let content = '';
  for (let i = 0; i < 3; i++) {
    content += JSON.stringify({
      type: 'assistant',
      isSidechain: false,
      message: { content: [{ type: 'text', text: 'We chose the best approach' }] },
    }) + '\n';
  }
  fs.writeFileSync(transcriptPath, content);

  const tmpGit = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-git5-'));
  fs.mkdirSync(path.join(tmpGit, '.git'));

  const result = await runPreCompact({
    transcript_path: transcriptPath,
    cwd: tmpGit,
    session_id: 'test-5',
    trigger: 'auto',
  });

  assert.strictEqual(result.code, 0);
  assert(result.stdout.length > 0);
  const output = JSON.parse(result.stdout);
  assert.strictEqual(output.decision, 'block');

  fs.rmSync(tmpDir, { recursive: true });
  fs.rmSync(tmpGit, { recursive: true });
});

test('pre-compact - missing transcript_path → exit 0, no stdout', async () => {
  const tmpGit = fs.mkdtempSync(path.join(os.tmpdir(), 'precompact-git6-'));
  fs.mkdirSync(path.join(tmpGit, '.git'));

  const result = await runPreCompact({
    cwd: tmpGit,
    session_id: 'test-6',
    trigger: 'manual',
  });

  assert.strictEqual(result.code, 0);
  assert.strictEqual(result.stdout, '');

  fs.rmSync(tmpGit, { recursive: true });
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

  // Internal errors should cause silent fail
  assert.strictEqual(result.code, 0);
});
