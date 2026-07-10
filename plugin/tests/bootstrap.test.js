const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('fs');
const path = require('path');
const os = require('os');
const zlib = require('zlib');
const crypto = require('crypto');
const { spawnSync } = require('child_process');

const scriptPath = path.resolve(__dirname, '..', 'scripts', 'bootstrap.js');
const bootstrap = require(scriptPath);

// withTmpDir supports both sync and async callbacks — when `fn` returns a
// promise, cleanup waits for it to settle before removing the tmp dir.
function withTmpDir(fn) {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ouroboros-bootstrap-'));
  const cleanup = () => {
    try {
      fs.rmSync(tmpDir, { recursive: true });
    } catch (e) {
      // ignore
    }
  };
  let result;
  try {
    result = fn(tmpDir);
  } catch (err) {
    cleanup();
    throw err;
  }
  if (result && typeof result.then === 'function') {
    return result.then(
      value => {
        cleanup();
        return value;
      },
      err => {
        cleanup();
        throw err;
      }
    );
  }
  cleanup();
  return result;
}

function withEnvVar(name, value, fn) {
  const prev = process.env[name];
  process.env[name] = value;
  try {
    return fn();
  } finally {
    if (prev === undefined) delete process.env[name];
    else process.env[name] = prev;
  }
}

// buildPluginRoot lays out a minimal <root>/hooks/hooks.json +
// <root>/scripts/bootstrap.js tree mirroring the real plugin, so path
// resolution against ${CLAUDE_PLUGIN_ROOT} can be exercised.
function buildPluginRoot(root, { scripts = ['bootstrap.js'] } = {}) {
  fs.mkdirSync(path.join(root, 'hooks'), { recursive: true });
  fs.mkdirSync(path.join(root, 'scripts'), { recursive: true });
  fs.writeFileSync(
    path.join(root, 'hooks', 'hooks.json'),
    JSON.stringify({
      hooks: {
        SessionStart: [{ hooks: [{ type: 'command', command: 'node ${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap.js' }] }],
      },
    })
  );
  for (const name of scripts) {
    fs.writeFileSync(path.join(root, 'scripts', name), '#!/usr/bin/env node\n');
  }
}

function writeExecutable(p, content = '#!/usr/bin/env bash\nexit 0\n') {
  fs.writeFileSync(p, content);
  fs.chmodSync(p, 0o755);
}

// buildUstarHeader constructs one valid 512-byte POSIX ustar header for a
// regular file entry.
function buildUstarHeader(name, size) {
  const header = Buffer.alloc(512);
  header.write(name, 0, 100, 'utf8');
  header.write('0000755', 100, 7, 'utf8'); // mode
  header.write('0000000', 108, 7, 'utf8'); // uid
  header.write('0000000', 116, 7, 'utf8'); // gid
  header.write(size.toString(8).padStart(11, '0'), 124, 11, 'utf8'); // size
  header.write('00000000000', 136, 11, 'utf8'); // mtime
  header.write('        ', 148, 8, 'utf8'); // checksum placeholder (spaces)
  header.write('0', 156, 1, 'utf8'); // typeflag: regular file
  header.write('ustar', 257, 6, 'utf8'); // magic
  header.write('00', 263, 2, 'utf8'); // version

  let sum = 0;
  for (let i = 0; i < 512; i++) sum += header[i];
  header.write(`${sum.toString(8).padStart(6, '0')}\0 `, 148, 8, 'utf8');
  return header;
}

// buildMultiTar packs a multi-entry ustar tar archive (+ end-of-archive
// padding) from an ordered list of { name, data } entries.
function buildMultiTar(entries) {
  const parts = [];
  for (const { name, data } of entries) {
    const header = buildUstarHeader(name, data.length);
    const dataPadded = Math.ceil(data.length / 512) * 512;
    const padded = Buffer.alloc(dataPadded);
    data.copy(padded);
    parts.push(header, padded);
  }
  parts.push(Buffer.alloc(1024)); // two zero blocks terminate the archive
  return Buffer.concat(parts);
}

// buildTar packs a single-entry ustar tar archive (+ end-of-archive
// padding) containing `content` at `name`.
function buildTar(name, content) {
  return buildMultiTar([{ name, data: content }]);
}

function sha256sumsLine(buf, name) {
  const hash = crypto.createHash('sha256').update(buf).digest('hex');
  return `${hash}  ${name}\n`;
}

// --- tar extractor --------------------------------------------------------

test('extractTarEntry: recovers a known-bytes file from a hand-built tar', () => {
  const content = Buffer.from('#!/bin/sh\necho hi\n');
  const tar = buildTar('ouroboros', content);
  const recovered = bootstrap.extractTarEntry(tar, 'ouroboros');
  assert.ok(recovered);
  assert.deepEqual(Buffer.from(recovered), content);
});

test('extractTarEntry: gunzip round-trip via zlib', () => {
  const content = Buffer.from('binary-content-stand-in');
  const tar = buildTar('ouroboros', content);
  const gz = zlib.gzipSync(tar);
  const tarBack = zlib.gunzipSync(gz);
  const recovered = bootstrap.extractTarEntry(tarBack, 'ouroboros');
  assert.deepEqual(Buffer.from(recovered), content);
});

test('extractTarEntry: recovers the right entry from a multi-entry archive (LICENSE, README.md, ouroboros)', () => {
  const licenseContent = Buffer.from('MIT License\ncopyright...\n');
  const readmeContent = Buffer.from('# ouroboros\nsee docs\n');
  const binContent = Buffer.from('#!/bin/sh\necho hi\n');
  const tar = buildMultiTar([
    { name: 'LICENSE', data: licenseContent },
    { name: 'README.md', data: readmeContent },
    { name: 'ouroboros', data: binContent },
  ]);
  const gz = zlib.gzipSync(tar);
  const tarBack = zlib.gunzipSync(gz);
  const recovered = bootstrap.extractTarEntry(tarBack, 'ouroboros');
  assert.ok(recovered);
  assert.deepEqual(Buffer.from(recovered), binContent);
});

test('extractTarEntry: returns null when entry absent', () => {
  const tar = buildTar('somethingelse', Buffer.from('x'));
  assert.equal(bootstrap.extractTarEntry(tar, 'ouroboros'), null);
});

// --- sha256 verify ---------------------------------------------------------

test('verifySha256: passes on a matching checksum', () => {
  const buf = Buffer.from('archive-bytes');
  const sums = sha256sumsLine(buf, 'ouroboros_1.2.3_darwin_arm64.tar.gz');
  assert.doesNotThrow(() => bootstrap.verifySha256(buf, sums, 'ouroboros_1.2.3_darwin_arm64.tar.gz'));
});

test('verifySha256: rejects on a mismatched checksum', () => {
  const buf = Buffer.from('archive-bytes');
  const badSums = `${'0'.repeat(64)}  ouroboros_1.2.3_darwin_arm64.tar.gz\n`;
  assert.throws(() => bootstrap.verifySha256(buf, badSums, 'ouroboros_1.2.3_darwin_arm64.tar.gz'), /checksum mismatch/);
});

test('verifySha256: rejects when no matching entry', () => {
  const buf = Buffer.from('archive-bytes');
  assert.throws(() => bootstrap.verifySha256(buf, 'deadbeef  other-file.tar.gz\n', 'missing.tar.gz'), /no checksum entry/);
});

// --- redirect following -----------------------------------------------------

test('followRedirects: follows a 302 -> 200 chain and returns the final body', async () => {
  const calls = [];
  const requestFn = async (url) => {
    calls.push(url);
    if (url === 'https://example.com/first') {
      return { statusCode: 302, headers: { location: 'https://example.com/second' }, body: Buffer.alloc(0) };
    }
    if (url === 'https://example.com/second') {
      return { statusCode: 200, headers: {}, body: Buffer.from('final-body') };
    }
    throw new Error(`unexpected url ${url}`);
  };
  const body = await bootstrap.followRedirects(requestFn, 'https://example.com/first', {});
  assert.equal(body.toString('utf8'), 'final-body');
  assert.deepEqual(calls, ['https://example.com/first', 'https://example.com/second']);
});

test('followRedirects: throws after exceeding the hop cap', async () => {
  const requestFn = async () => ({ statusCode: 302, headers: { location: 'https://example.com/loop' }, body: Buffer.alloc(0) });
  await assert.rejects(() => bootstrap.followRedirects(requestFn, 'https://example.com/loop', {}, 2), /too many redirects/);
});

test('followRedirects: throws on a non-200 terminal response', async () => {
  const requestFn = async () => ({ statusCode: 404, headers: {}, body: Buffer.alloc(0) });
  await assert.rejects(() => bootstrap.followRedirects(requestFn, 'https://example.com/missing', {}), /HTTP 404/);
});

// --- release install path (injected network) --------------------------------

function fakeReleaseGet({ tag, archiveName, archiveBuf, checksumName, checksumBuf }) {
  return async (url) => {
    if (url.endsWith('/releases/latest')) {
      return Buffer.from(JSON.stringify({ tag_name: tag }));
    }
    if (url.endsWith(`/${archiveName}`)) return archiveBuf;
    if (url.endsWith(`/${checksumName}`)) return checksumBuf;
    throw new Error(`unexpected url ${url}`);
  };
}

test('installFromRelease: downloads, verifies, extracts, and atomically installs', async () => {
  await withTmpDir(async tmpDir => {
    const pluginData = path.join(tmpDir, 'plugin-data');
    fs.mkdirSync(pluginData, { recursive: true });

    const platform = bootstrap.detectPlatform();
    assert.ok(platform, 'test host platform must be supported (darwin/linux, amd64/arm64)');
    const version = '9.9.9';
    const archiveName = `ouroboros_${version}_${platform.os}_${platform.arch}.${platform.ext}`;
    const checksumName = `ouroboros_${version}_SHA256SUMS`;
    const binaryContent = Buffer.from('#!/bin/sh\necho ouroboros-fake\n');
    const archiveBuf = zlib.gzipSync(buildTar('ouroboros', binaryContent));
    const checksumBuf = Buffer.from(sha256sumsLine(archiveBuf, archiveName));

    const get = fakeReleaseGet({ tag: `v${version}`, archiveName, archiveBuf, checksumName, checksumBuf });

    const result = await bootstrap.installFromRelease(pluginData, get);
    assert.equal(result.ok, true);
    assert.equal(result.version, version);

    const installedBinary = path.join(pluginData, 'bin', 'ouroboros');
    assert.ok(fs.existsSync(installedBinary));
    assert.deepEqual(fs.readFileSync(installedBinary), binaryContent);
    assert.equal(fs.statSync(installedBinary).mode & 0o777, 0o755);
    assert.equal(fs.readFileSync(path.join(pluginData, '.version'), 'utf8'), version);
    // no leftover .tmp.<pid> file from the atomic rename
    const leftover = fs.readdirSync(path.join(pluginData, 'bin')).filter(f => f.includes('.tmp.'));
    assert.deepEqual(leftover, []);
  });
});

test('installFromRelease: version-skip when .version already matches latest and binary present', async () => {
  await withTmpDir(async tmpDir => {
    const pluginData = path.join(tmpDir, 'plugin-data');
    fs.mkdirSync(path.join(pluginData, 'bin'), { recursive: true });
    writeExecutable(path.join(pluginData, 'bin', 'ouroboros'));
    fs.writeFileSync(path.join(pluginData, '.version'), '1.2.3');

    let archiveRequested = false;
    const get = async url => {
      if (url.endsWith('/releases/latest')) return Buffer.from(JSON.stringify({ tag_name: 'v1.2.3' }));
      archiveRequested = true;
      throw new Error(`should not fetch ${url} on version-skip`);
    };

    const result = await bootstrap.installFromRelease(pluginData, get);
    assert.equal(result.ok, true);
    assert.equal(result.skipped, true);
    assert.equal(archiveRequested, false);
  });
});

test('installFromRelease: offline fallback keeps existing binary when latest-tag fetch fails', async () => {
  await withTmpDir(async tmpDir => {
    const pluginData = path.join(tmpDir, 'plugin-data');
    fs.mkdirSync(path.join(pluginData, 'bin'), { recursive: true });
    writeExecutable(path.join(pluginData, 'bin', 'ouroboros'), 'existing-binary');

    const get = async () => {
      throw new Error('network unreachable');
    };
    const result = await bootstrap.installFromRelease(pluginData, get);
    assert.equal(result.ok, true);
    assert.equal(result.offline, true);
    assert.equal(fs.readFileSync(path.join(pluginData, 'bin', 'ouroboros'), 'utf8'), 'existing-binary');
  });
});

test('installFromRelease: mismatched checksum rejects install (binary not written)', async () => {
  await withTmpDir(async tmpDir => {
    const pluginData = path.join(tmpDir, 'plugin-data');
    fs.mkdirSync(pluginData, { recursive: true });

    const platform = bootstrap.detectPlatform();
    const version = '9.9.8';
    const archiveName = `ouroboros_${version}_${platform.os}_${platform.arch}.${platform.ext}`;
    const checksumName = `ouroboros_${version}_SHA256SUMS`;
    const archiveBuf = zlib.gzipSync(buildTar('ouroboros', Buffer.from('bytes')));
    const badChecksumBuf = Buffer.from(`${'f'.repeat(64)}  ${archiveName}\n`);

    const get = fakeReleaseGet({ tag: `v${version}`, archiveName, archiveBuf, checksumName, checksumBuf: badChecksumBuf });
    const result = await bootstrap.installFromRelease(pluginData, get);
    assert.equal(result.ok, false);
    assert.match(result.reason, /checksum/);
    assert.ok(!fs.existsSync(path.join(pluginData, 'bin', 'ouroboros')));
  });
});

// --- dev-binary and local-binary install paths ------------------------------

test('installDevBinary: copies dev binary atomically and writes version=dev', () => {
  withTmpDir(tmpDir => {
    const pluginData = path.join(tmpDir, 'plugin-data');
    fs.mkdirSync(pluginData, { recursive: true });
    const devBinary = path.join(tmpDir, 'dev-ouroboros');
    writeExecutable(devBinary, 'dev-binary-bytes');

    const result = bootstrap.installDevBinary(pluginData, devBinary);
    assert.equal(result.ok, true);
    assert.equal(fs.readFileSync(path.join(pluginData, 'bin', 'ouroboros'), 'utf8'), 'dev-binary-bytes');
    assert.equal(fs.readFileSync(path.join(pluginData, '.version'), 'utf8'), 'dev');
  });
});

test('installDevBinary: errors when dev binary path is not executable', () => {
  withTmpDir(tmpDir => {
    const pluginData = path.join(tmpDir, 'plugin-data');
    fs.mkdirSync(pluginData, { recursive: true });
    const result = bootstrap.installDevBinary(pluginData, path.join(tmpDir, 'nope'));
    assert.equal(result.ok, false);
  });
});

test('installLocalBinary: copies a resolved local binary and records --version output', () => {
  withTmpDir(tmpDir => {
    const pluginData = path.join(tmpDir, 'plugin-data');
    fs.mkdirSync(pluginData, { recursive: true });
    const localBin = path.join(tmpDir, 'local-ouroboros');
    writeExecutable(localBin, '#!/bin/sh\necho ouroboros-local-1.0.0\n');

    const result = bootstrap.installLocalBinary(pluginData, localBin);
    assert.equal(result.ok, true);
    assert.ok(fs.existsSync(path.join(pluginData, 'bin', 'ouroboros')));
    assert.equal(fs.readFileSync(path.join(pluginData, '.version'), 'utf8'), 'ouroboros-local-1.0.0');
  });
});

// --- install() precedence + validate() orchestration ------------------------

test('install: prefers OUROBOROS_DEV_BINARY over local/release', async () => {
  await withTmpDir(async tmpDir => {
    const pluginData = path.join(tmpDir, 'plugin-data');
    fs.mkdirSync(pluginData, { recursive: true });
    const devBinary = path.join(tmpDir, 'dev-ouroboros');
    writeExecutable(devBinary, 'dev-bytes');

    const result = await bootstrap.install({ CLAUDE_PLUGIN_DATA: pluginData, OUROBOROS_DEV_BINARY: devBinary });
    assert.equal(result.ok, true);
    assert.equal(fs.readFileSync(path.join(pluginData, '.version'), 'utf8'), 'dev');
  });
});

test('install: errors fail-open when CLAUDE_PLUGIN_DATA unset', async () => {
  const result = await bootstrap.install({});
  assert.equal(result.ok, false);
});

// --- checkBinary / checkHookScripts / checkStatusline (unchanged surface) ---

test('checkBinary: ok when binary exists and is executable', () => {
  withTmpDir(tmpDir => {
    fs.mkdirSync(path.join(tmpDir, 'bin'), { recursive: true });
    writeExecutable(path.join(tmpDir, 'bin', 'ouroboros'));
    assert.equal(bootstrap.checkBinary(tmpDir).ok, true);
  });
});

test('checkBinary: not ok when binary missing', () => {
  withTmpDir(tmpDir => {
    const result = bootstrap.checkBinary(tmpDir);
    assert.equal(result.ok, false);
    assert.match(result.reason, /missing or not executable/);
  });
});

test('checkBinary: not ok when CLAUDE_PLUGIN_DATA unset', () => {
  const result = bootstrap.checkBinary(undefined);
  assert.equal(result.ok, false);
  assert.match(result.reason, /CLAUDE_PLUGIN_DATA not set/);
});

test('checkHookScripts: ok when every referenced script exists', () => {
  withTmpDir(tmpDir => {
    buildPluginRoot(tmpDir);
    const result = bootstrap.checkHookScripts(tmpDir);
    assert.equal(result.ok, true);
    assert.deepEqual(result.missing, []);
  });
});

test('checkHookScripts: reports a missing referenced script', () => {
  withTmpDir(tmpDir => {
    buildPluginRoot(tmpDir, { scripts: [] }); // bootstrap.js referenced but not written
    const result = bootstrap.checkHookScripts(tmpDir);
    assert.equal(result.ok, false);
    assert.ok(result.missing.some(p => p.endsWith('bootstrap.js')));
  });
});

test('checkHookScripts: not ok when CLAUDE_PLUGIN_ROOT unset', () => {
  const result = bootstrap.checkHookScripts(undefined);
  assert.equal(result.ok, false);
  assert.match(result.reason, /CLAUDE_PLUGIN_ROOT not set/);
});

test('checkHookScripts: fail-open when hooks.json missing', () => {
  withTmpDir(tmpDir => {
    const result = bootstrap.checkHookScripts(tmpDir);
    assert.equal(result.ok, false);
    assert.match(result.reason, /cannot read\/parse/);
  });
});

test('checkStatusline: skipped when settings.json absent (CLAUDE_CONFIG_DIR isolation)', () => {
  withTmpDir(tmpDir => {
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    withEnvVar('CLAUDE_CONFIG_DIR', configDir, () => {
      const result = bootstrap.checkStatusline(tmpDir);
      assert.equal(result.skipped, true);
      assert.equal(result.settingsPath, path.join(configDir, 'settings.json'));
    });
  });
});

test('checkStatusline: resolves settings.json under CLAUDE_CONFIG_DIR, not ~/.claude', () => {
  withTmpDir(tmpDir => {
    const configDir = path.join(tmpDir, 'my-config');
    fs.mkdirSync(configDir, { recursive: true });
    withEnvVar('CLAUDE_CONFIG_DIR', configDir, () => {
      const result = bootstrap.checkStatusline(undefined);
      assert.equal(result.settingsPath, path.join(configDir, 'settings.json'));
      assert.notEqual(result.settingsPath, path.join(os.homedir(), '.claude', 'settings.json'));
    });
  });
});

test('checkStatusline: flags a missing plugin-owned statusLine script', () => {
  withTmpDir(tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    buildPluginRoot(pluginRoot);
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(
      path.join(configDir, 'settings.json'),
      JSON.stringify({ statusLine: { command: `node ${pluginRoot}/scripts/statusline.js` } })
    );
    withEnvVar('CLAUDE_CONFIG_DIR', configDir, () => {
      const result = bootstrap.checkStatusline(pluginRoot);
      assert.equal(result.ok, false);
      assert.ok(result.missing.some(p => p.endsWith('statusline.js')));
    });
  });
});

test('checkStatusline: ok when the referenced script exists', () => {
  withTmpDir(tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    buildPluginRoot(pluginRoot, { scripts: ['bootstrap.js', 'statusline.js'] });
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });
    fs.writeFileSync(
      path.join(configDir, 'settings.json'),
      JSON.stringify({ statusLine: { command: `node ${pluginRoot}/scripts/statusline.js` } })
    );
    withEnvVar('CLAUDE_CONFIG_DIR', configDir, () => {
      const result = bootstrap.checkStatusline(pluginRoot);
      assert.equal(result.ok, true);
    });
  });
});

// --- stale subagent-stop.js guard (OU-254) ----------------------------------

test('isStaleSubagentHook: false on the current clean subagent-stop.js', () => {
  const contents = fs.readFileSync(path.resolve(__dirname, '..', 'scripts', 'subagent-stop.js'), 'utf8');
  assert.equal(bootstrap.isStaleSubagentHook(contents), false);
});

test('isStaleSubagentHook: true on a stale pre-OU-222 fixture containing "tier-1 nudge fired"', () => {
  const stale = fs.readFileSync(path.join(__dirname, 'fixtures', 'stale-subagent-stop.js'), 'utf8');
  assert.equal(bootstrap.isStaleSubagentHook(stale), true);
});

test('isStaleSubagentHook: false on non-string input', () => {
  assert.equal(bootstrap.isStaleSubagentHook(undefined), false);
  assert.equal(bootstrap.isStaleSubagentHook(null), false);
});

test('checkSubagentHookStale: ok on the real plugin scripts/subagent-stop.js', () => {
  const pluginRoot = path.resolve(__dirname, '..');
  const result = bootstrap.checkSubagentHookStale(pluginRoot);
  assert.equal(result.ok, true);
  assert.equal(result.skipped, undefined);
});

test('checkSubagentHookStale: not ok when subagent-stop.js is the stale fixture', () => {
  withTmpDir(tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    fs.mkdirSync(path.join(pluginRoot, 'scripts'), { recursive: true });
    const staleContents = fs.readFileSync(path.join(__dirname, 'fixtures', 'stale-subagent-stop.js'), 'utf8');
    fs.writeFileSync(path.join(pluginRoot, 'scripts', 'subagent-stop.js'), staleContents);

    const result = bootstrap.checkSubagentHookStale(pluginRoot);
    assert.equal(result.ok, false);
  });
});

test('checkSubagentHookStale: skipped when pluginRoot unset or file unreadable', () => {
  assert.equal(bootstrap.checkSubagentHookStale(undefined).skipped, true);
  withTmpDir(tmpDir => {
    const result = bootstrap.checkSubagentHookStale(tmpDir); // no scripts/subagent-stop.js
    assert.equal(result.skipped, true);
  });
});

test('bootstrap: warns when subagent-stop.js is stale, still exits ok (fail-open)', async () => {
  await withTmpDir(async tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    const pluginData = path.join(tmpDir, 'plugin-data');
    buildPluginRoot(pluginRoot, { scripts: ['bootstrap.js'] });
    const staleContents = fs.readFileSync(path.join(__dirname, 'fixtures', 'stale-subagent-stop.js'), 'utf8');
    fs.writeFileSync(path.join(pluginRoot, 'scripts', 'subagent-stop.js'), staleContents);
    fs.mkdirSync(path.join(pluginData, 'bin'), { recursive: true });
    writeExecutable(path.join(pluginData, 'bin', 'ouroboros'));
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    const result = await bootstrap.bootstrap({ CLAUDE_PLUGIN_ROOT: pluginRoot, CLAUDE_PLUGIN_DATA: pluginData, CLAUDE_CONFIG_DIR: configDir });
    assert.equal(result.subagentHook.ok, false);
  });
});

test('bootstrap: no warning when subagent-stop.js is clean', async () => {
  await withTmpDir(async tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    const pluginData = path.join(tmpDir, 'plugin-data');
    buildPluginRoot(pluginRoot, { scripts: ['bootstrap.js'] });
    fs.writeFileSync(
      path.join(pluginRoot, 'scripts', 'subagent-stop.js'),
      fs.readFileSync(path.resolve(__dirname, '..', 'scripts', 'subagent-stop.js'), 'utf8')
    );
    fs.mkdirSync(path.join(pluginData, 'bin'), { recursive: true });
    writeExecutable(path.join(pluginData, 'bin', 'ouroboros'));
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    const result = await bootstrap.bootstrap({ CLAUDE_PLUGIN_ROOT: pluginRoot, CLAUDE_PLUGIN_DATA: pluginData, CLAUDE_CONFIG_DIR: configDir });
    assert.equal(result.subagentHook.ok, true);
  });
});

test('CLI: warns to stderr on stale subagent-stop.js fixture, still exits 0', () => {
  withTmpDir(tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    const pluginData = path.join(tmpDir, 'plugin-data');
    buildPluginRoot(pluginRoot, { scripts: ['bootstrap.js'] });
    const staleContents = fs.readFileSync(path.join(__dirname, 'fixtures', 'stale-subagent-stop.js'), 'utf8');
    fs.writeFileSync(path.join(pluginRoot, 'scripts', 'subagent-stop.js'), staleContents);
    fs.mkdirSync(path.join(pluginData, 'bin'), { recursive: true });
    writeExecutable(path.join(pluginData, 'bin', 'ouroboros'));
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    const result = spawnSync('node', [scriptPath], {
      env: { ...process.env, CLAUDE_PLUGIN_ROOT: pluginRoot, CLAUDE_PLUGIN_DATA: pluginData, CLAUDE_CONFIG_DIR: configDir },
      encoding: 'utf8',
    });
    assert.equal(result.status, 0);
    assert.match(result.stderr, /STALE subagent-stop\.js detected/);
  });
});

// --- bootstrap() orchestration (in-process) -----------------------------------

test('bootstrap: binary + scripts + statusline all present -> no repair triggered', async () => {
  await withTmpDir(async tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    const pluginData = path.join(tmpDir, 'plugin-data');
    buildPluginRoot(pluginRoot);
    fs.mkdirSync(path.join(pluginData, 'bin'), { recursive: true });
    writeExecutable(path.join(pluginData, 'bin', 'ouroboros'));
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    const result = await bootstrap.bootstrap({ CLAUDE_PLUGIN_ROOT: pluginRoot, CLAUDE_PLUGIN_DATA: pluginData, CLAUDE_CONFIG_DIR: configDir });
    assert.equal(result.repaired, false);
    assert.equal(result.binary.ok, true);
    assert.equal(result.scripts.ok, true);
    assert.equal(result.installResult, null);
  });
});

test('bootstrap: binary missing -> install() invoked and installs a dev binary', async () => {
  await withTmpDir(async tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    const pluginData = path.join(tmpDir, 'plugin-data');
    buildPluginRoot(pluginRoot);
    const devBinary = path.join(tmpDir, 'dev-ouroboros');
    writeExecutable(devBinary, 'dev-bytes');
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    const result = await bootstrap.bootstrap({
      CLAUDE_PLUGIN_ROOT: pluginRoot,
      CLAUDE_PLUGIN_DATA: pluginData,
      CLAUDE_CONFIG_DIR: configDir,
      OUROBOROS_DEV_BINARY: devBinary,
    });
    assert.equal(result.repaired, true);
    assert.equal(result.binary.ok, false); // pre-repair snapshot
    assert.equal(result.installResult.ok, true);
    assert.ok(fs.existsSync(path.join(pluginData, 'bin', 'ouroboros')));
  });
});

test('bootstrap: missing hook script logs actionable corrupt-install message, still no throw', async () => {
  await withTmpDir(async tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    const pluginData = path.join(tmpDir, 'plugin-data');
    buildPluginRoot(pluginRoot, { scripts: [] }); // bootstrap.js referenced but absent
    fs.mkdirSync(path.join(pluginData, 'bin'), { recursive: true });
    writeExecutable(path.join(pluginData, 'bin', 'ouroboros'));
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    const result = await bootstrap.bootstrap({ CLAUDE_PLUGIN_ROOT: pluginRoot, CLAUDE_PLUGIN_DATA: pluginData, CLAUDE_CONFIG_DIR: configDir });
    assert.equal(result.scripts.ok, false);
    assert.ok(result.scripts.missing.some(p => p.endsWith('bootstrap.js')));
  });
});

test('bootstrap: fail-open when everything is missing/unreadable (no throw)', async () => {
  await assert.doesNotReject(() => bootstrap.bootstrap({}));
});

// --- end-to-end CLI tests (spawned) -----------------------------------------

test('CLI: exits 0 with fully missing env (fail-open)', () => {
  const base = { ...process.env };
  delete base.CLAUDE_PLUGIN_ROOT;
  delete base.CLAUDE_PLUGIN_DATA;
  const result = spawnSync('node', [scriptPath], { env: base, encoding: 'utf8' });
  assert.equal(result.status, 0);
});

test('CLI: exits 0 and repairs binary via dev-binary install when broken', () => {
  withTmpDir(tmpDir => {
    const pluginRoot = path.join(tmpDir, 'plugin-root');
    const pluginData = path.join(tmpDir, 'plugin-data');
    buildPluginRoot(pluginRoot);
    const devBinary = path.join(tmpDir, 'dev-ouroboros');
    writeExecutable(devBinary, 'dev-bytes');
    const configDir = path.join(tmpDir, 'config');
    fs.mkdirSync(configDir, { recursive: true });

    const result = spawnSync('node', [scriptPath], {
      env: {
        ...process.env,
        CLAUDE_PLUGIN_ROOT: pluginRoot,
        CLAUDE_PLUGIN_DATA: pluginData,
        CLAUDE_CONFIG_DIR: configDir,
        OUROBOROS_DEV_BINARY: devBinary,
      },
      encoding: 'utf8',
    });
    assert.equal(result.status, 0);
    assert.ok(fs.existsSync(path.join(pluginData, 'bin', 'ouroboros')));
    assert.match(result.stderr, /installed dev binary/);
  });
});
