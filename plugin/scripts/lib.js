const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const SKIP_AGENT_TYPES = ['Explore', 'knowledge-explorer', 'backlog-manager'];

function readStdin() {
  return new Promise((resolve) => {
    let data = '';
    process.stdin.on('data', chunk => { data += chunk; });
    process.stdin.on('end', () => resolve(data));
    process.stdin.on('error', () => resolve(''));
  });
}

function getProject() {
  try {
    return execSync('git rev-parse --show-toplevel 2>/dev/null', {
      encoding: 'utf-8',
      timeout: 2000,
    }).trim().split('/').pop();
  } catch (e) {
    return null;
  }
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

function matchesAnyPattern(message, patterns) {
  if (!patterns || patterns.length === 0) {
    return false;
  }
  return patterns.some(p => p.test(message));
}

function formatContextLines(project, rows) {
  if (!rows || rows.length === 0) {
    return [];
  }

  const lines = [];
  lines.push(`[ouroboros] ${project} KB (${rows.length}):`);
  for (const row of rows) {
    lines.push(`  [${row.type}] ${row.title}`);
  }
  lines.push('');
  lines.push('persist any decisions/facts via a fenced kb block (project: ' + project + '):');
  lines.push('```kb');
  lines.push('[{"type":"decision|fact|note|plan|relation","title":"…","content":"≤500 chars","notes":"narrative","tags":["…"]}]');
  lines.push('```');

  return lines;
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

function resolveProject(hints = {}, workspaceRoot, skipGit = false) {
  // Priority 1: git rev-parse --show-toplevel (same as getProject)
  // Skip this if explicitly disabled (for testing or workspace-mode operation)
  if (!skipGit) {
    try {
      const gitRoot = execSync('git rev-parse --show-toplevel 2>/dev/null', {
        encoding: 'utf-8',
        timeout: 2000,
      }).trim();
      if (gitRoot) {
        return gitRoot.split('/').pop();
      }
    } catch (e) {
      // Fall through to next priority
    }
  }

  // Determine workspace root if not provided
  const root = workspaceRoot || findWorkspaceRoot();
  const projects = root ? listWorkspaceProjects(root) : [];

  // Priority 2: hints.filePath — extract project name from file path
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

  // Priority 3: hints.message — word-boundary regex match of project names
  if (hints.message && projects.length > 0) {
    for (const projectName of projects) {
      const escaped = projectName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      const regex = new RegExp(`\\b${escaped}\\b`, 'i');
      if (regex.test(hints.message)) {
        return projectName;
      }
    }
  }

  // Priority 4: hints.transcriptPath — scan JSONL backwards for tool_use entries
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

module.exports = { readStdin, getProject, getBinaryPath, isWithinCooldown, touchFile, extractKbBlock, matchesAnyPattern, formatContextLines, findWorkspaceRoot, listWorkspaceProjects, resolveProject, SKIP_AGENT_TYPES };
