const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const SKIP_AGENT_TYPES = ['Explore', 'knowledge-explorer', 'backlog-manager'];
const LOG_PATH = `${process.env.HOME}/.ouroboros/hooks.log`;
let logDirCreated = false;


function readStdin() {
  return new Promise((resolve) => {
    let data = '';
    process.stdin.on('data', chunk => { data += chunk; });
    process.stdin.on('end', () => resolve(data));
    process.stdin.on('error', () => resolve(''));
  });
}

function getBinaryPath() {
  if (process.env.CLAUDE_PLUGIN_DATA) {
    const pluginPath = `${process.env.CLAUDE_PLUGIN_DATA}/bin/ouroboros`;
    if (fs.existsSync(pluginPath)) return pluginPath;
  }
  try {
    return execSync('which ouroboros 2>/dev/null', {
      encoding: 'utf-8',
      timeout: 1000,
    }).trim();
  } catch (e) {
    return null;
  }
}

function isWithinCooldown(filePath, cooldownMs) {
  try {
    const stats = fs.statSync(filePath);
    return (Date.now() - stats.mtimeMs) < cooldownMs;
  } catch (e) {
    return false;
  }
}

function touchFile(filePath) {
  try { fs.writeFileSync(filePath, ''); } catch (e) {}
}

function extractKbBlock(message) {
  const regex = /```kb\s*\n([\s\S]*?)\n```/;
  const match = message.match(regex);
  if (match && match[1]) {
    return { matched: true, json: match[1] };
  }
  return { matched: false, json: null };
}

function extractAllKbBlocks(transcriptPath, opts = { maxLines: 2000 }) {
  const maxLines = opts.maxLines || 2000;
  const blocks = [];
  const turns = [];

  // Decision language patterns (simple, case-insensitive)
  const decisionPatterns = [
    /\bdecision\b/i,  // decision (exact word)
    /\bdecid/i,       // decide, decided, deciding
    /\bchose\b/i,     // chose
    /\bchosen\b/i,    // chosen
    /\bchoose\b/i,    // choose
    /\bgoing with\b/i,
    /\breject/i,      // reject, rejected, rejecting
    /\bpick/i,        // pick, picked, picking
    /\bselect/i,      // select, selected, selecting
    /\bopting\b/i,
  ];

  let raw;
  try {
    raw = fs.readFileSync(transcriptPath, 'utf-8');
  } catch (e) {
    return { blocks: [], turns: [] };
  }

  const lines = raw.split('\n').filter(line => line.trim());
  const startIdx = Math.max(0, lines.length - maxLines);

  for (let i = startIdx; i < lines.length; i++) {
    const line = lines[i];

    let obj;
    try {
      obj = JSON.parse(line);
    } catch (e) {
      continue;
    }

    // Only include main-context assistant entries (not sidechain)
    if (obj.type !== 'assistant') continue;
    if (obj.isSidechain === true) continue;

    // Extract text content blocks
    const content = (obj.message && obj.message.content) || [];
    const text = content
      .filter(c => c && c.type === 'text' && typeof c.text === 'string')
      .map(c => c.text)
      .join('\n');

    if (!text) continue;

    // Extract KB blocks from this turn's text
    const { matched, json } = extractKbBlock(text);
    const blockData = matched ? { text: json } : null;
    if (blockData) {
      blocks.push(blockData);
    }

    // Detect decision language in this turn
    const hasDecisionLanguage = decisionPatterns.some(pattern => pattern.test(text));

    turns.push({
      text,
      hasKbBlock: matched,
      hasDecisionLanguage,
    });
  }

  return { blocks, turns };
}

function matchesAnyPattern(message, patterns) {
  if (!patterns || patterns.length === 0) {
    return false;
  }
  return patterns.some(p => p.test(message));
}

function formatContextLines(project, rows, options = {}) {
  if (!rows || rows.length === 0) {
    return [];
  }

  const includeContract = options.includeContract !== false;
  const lines = [];
  lines.push(`[ouroboros] ${project} KB (${rows.length}):`);
  for (const row of rows) {
    lines.push(`  [${row.type}] ${row.title}`);
  }

  if (includeContract) {
    lines.push(`if a decision or fact is worth persisting, emit a fenced kb block (project: ${project}); otherwise say nothing`);
  }

  return lines;
}

function findGitRoot(startPath) {
  try {
    if (!startPath) return null;
    let current = path.resolve(startPath);
    const root = path.parse(current).root;

    // If startPath is a file, start from its directory
    if (fs.existsSync(current)) {
      const stats = fs.statSync(current);
      if (stats.isFile()) {
        current = path.dirname(current);
      }
    } else {
      // If startPath doesn't exist, still try to walk up from the given path
      current = path.dirname(current);
    }

    while (current !== root) {
      try {
        const gitPath = path.join(current, '.git');
        if (fs.existsSync(gitPath)) {
          return current;
        }
      } catch (e) {
        // Continue walking
      }
      current = path.dirname(current);
    }

    return null;
  } catch (e) {
    return null;
  }
}

function projectFromPath(startPath) {
  try {
    const gitRoot = findGitRoot(startPath);
    if (!gitRoot) {
      return null;
    }
    return path.basename(gitRoot);
  } catch (e) {
    return null;
  }
}

function findWorkspaceRoot() {
  let current = process.cwd();
  const root = path.parse(current).root;

  while (current !== root) {
    try {
      const claudeDir = path.join(current, '.claude');
      if (fs.existsSync(claudeDir)) {
        return current;
      }
    } catch (e) {
      // Continue walking
    }
    current = path.dirname(current);
  }

  return null;
}

function listWorkspaceProjects(root) {
  if (!root) {
    return [];
  }

  try {
    const entries = fs.readdirSync(root, { withFileTypes: true });
    return entries
      .filter(entry => entry.isDirectory() && !entry.name.startsWith('.'))
      .map(entry => entry.name);
  } catch (e) {
    return [];
  }
}

function resolveProject(hints = {}, workspaceRoot) {
  // Determine workspace root if not provided
  const root = workspaceRoot || findWorkspaceRoot();
  const projects = root ? listWorkspaceProjects(root) : [];

  // Priority 1: hints.filePath — extract project name from file path
  if (hints.filePath && root) {
    try {
      const relativePath = path.relative(root, hints.filePath);
      const firstSegment = relativePath.split(path.sep)[0];
      if (firstSegment && projects.includes(firstSegment)) {
        return firstSegment;
      }
    } catch (e) {
      // Fall through
    }
  }

  // Priority 2: hints.message — word-boundary regex match of project names
  if (hints.message && projects.length > 0) {
    for (const projectName of projects) {
      const escaped = projectName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      const regex = new RegExp(`\\b${escaped}\\b`, 'i');
      if (regex.test(hints.message)) {
        return projectName;
      }
    }
  }

  // Priority 3: hints.transcriptPath — scan JSONL backwards for tool_use entries
  if (hints.transcriptPath && root && projects.length > 0) {
    try {
      const content = fs.readFileSync(hints.transcriptPath, 'utf-8');
      const lines = content.split('\n');
      const scanLimit = 2000;
      const startIdx = Math.max(0, lines.length - scanLimit);

      for (let i = lines.length - 1; i >= startIdx; i--) {
        const line = lines[i].trim();
        if (!line) continue;

        try {
          const obj = JSON.parse(line);
          if (obj.message && Array.isArray(obj.message.content)) {
            for (const contentItem of obj.message.content) {
              if (contentItem.type === 'tool_use' && contentItem.input) {
                const filePath = contentItem.input.file_path || contentItem.input.path || contentItem.input.abs_path;
                if (filePath && typeof filePath === 'string') {
                  for (const projectName of projects) {
                    const projectPrefix = path.join(root, projectName);
                    if (filePath.startsWith(projectPrefix)) {
                      return projectName;
                    }
                  }
                }
              }
            }
          }
        } catch (e) {
          // Continue scanning
        }
      }
    } catch (e) {
      // Silently fail
    }
  }

  return null;
}

function getMaxLogSize() {
  const envVal = process.env.OUROBOROS_HOOK_LOG_MAX_SIZE;
  if (!envVal) return 5 * 1024 * 1024; // 5MB default
  const parsed = parseInt(envVal, 10);
  return isNaN(parsed) ? 5 * 1024 * 1024 : parsed;
}

function getMaxLogFiles() {
  const envVal = process.env.OUROBOROS_HOOK_LOG_MAX_FILES;
  if (!envVal) return 1; // 1 backup default
  const parsed = parseInt(envVal, 10);
  return isNaN(parsed) ? 1 : parsed;
}

function rotateLogFiles(logPath, maxFiles) {
  try {
    // If maxFiles is 0, don't rotate anything
    if (maxFiles <= 0) {
      try {
        fs.unlinkSync(logPath);
      } catch (e) {
        // Silent fail on unlink
      }
      return;
    }

    // Shift existing backups backwards: .log.N → .log.(N+1), ..., .log.1 → .log.2
    for (let i = maxFiles; i >= 1; i--) {
      const oldPath = `${logPath}.${i}`;
      const newPath = `${logPath}.${i + 1}`;
      try {
        if (fs.existsSync(oldPath)) {
          fs.renameSync(oldPath, newPath);
        }
      } catch (e) {
        // Silent fail on individual shift
      }
    }

    // Rotate current log → .log.1
    try {
      fs.renameSync(logPath, `${logPath}.1`);
    } catch (e) {
      // Silent fail on rename
    }

    // Delete any files beyond maxFiles (starting from .log.(maxFiles+1))
    let deleteIdx = maxFiles + 1;
    while (true) {
      const pathToDelete = `${logPath}.${deleteIdx}`;
      if (!fs.existsSync(pathToDelete)) break;
      try {
        fs.unlinkSync(pathToDelete);
      } catch (e) {
        // Silent fail on delete
      }
      deleteIdx++;
    }
  } catch (e) {
    // Silent fail on entire rotation
  }
}

function logHookEvent(fields) {
  const flag = process.env.OUROBOROS_HOOK_LOG;
  if (flag === '0' || flag === 'false' || flag === 'off') return;
  try {
    // Create directory on first call
    if (!logDirCreated) {
      try {
        const dir = path.dirname(LOG_PATH);
        fs.mkdirSync(dir, { recursive: true });
        logDirCreated = true;
      } catch (e) {
        // Silent fail on mkdir
        return;
      }
    }

    // Size check: if file exceeds configured max, rotate
    try {
      const maxSize = getMaxLogSize();
      const stats = fs.statSync(LOG_PATH);
      if (stats.size > maxSize) {
        const maxFiles = getMaxLogFiles();
        rotateLogFiles(LOG_PATH, maxFiles);
      }
    } catch (e) {
      // File doesn't exist yet; that's fine
    }

    // Write JSONL line
    const entry = {
      ts: new Date().toISOString(),
      ...fields,
    };
    fs.appendFileSync(LOG_PATH, JSON.stringify(entry) + '\n');
  } catch (e) {
    // Silent fail — logging must never break a hook
  }
}

function isSkippedAgentType(agentType) {
  if (!agentType) return false;
  if (SKIP_AGENT_TYPES.includes(agentType)) return true;
  const tail = agentType.includes(':') ? agentType.split(':').pop() : agentType;
  return SKIP_AGENT_TYPES.includes(tail);
}

module.exports = { readStdin, getBinaryPath, isWithinCooldown, touchFile, extractKbBlock, extractAllKbBlocks, matchesAnyPattern, formatContextLines, findGitRoot, projectFromPath, findWorkspaceRoot, listWorkspaceProjects, resolveProject, logHookEvent, getMaxLogSize, getMaxLogFiles, rotateLogFiles, SKIP_AGENT_TYPES, isSkippedAgentType };
