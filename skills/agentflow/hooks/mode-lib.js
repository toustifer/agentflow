// Shared helpers for agentflow sticky mode.
// Mode file: <project>/.claude/agentflow/mode.json

const fs = require("fs");
const path = require("path");
const crypto = require("crypto");
const os = require("os");

const MODE_REL = path.join(".claude", "agentflow", "mode.json");
const STATUS_REL = path.join(".claude", "agentflow", "status.json");
const AGENTFLOW_MODE_VERSION = 1;
const USER_SETTINGS_PATH = path.join(os.homedir(), ".claude", "settings.json");
const USER_PROMPT_HOOK_COMMAND = "node C:\\Users\\15775\\.claude\\skills\\agentflow\\hooks\\mode-inject.js";
const STATUSLINE_COMMAND = "node C:\\Users\\15775\\.claude\\skills\\agentflow\\hooks\\statusline.js";

function projectDir() {
  return process.env.CLAUDE_PROJECT_DIR || process.env.PWD || process.cwd();
}

function modePath(root) {
  return path.join(root || projectDir(), MODE_REL);
}

function statusPath(root) {
  return path.join(root || projectDir(), STATUS_REL);
}

function fallbackModePath(root) {
  const key = crypto
    .createHash("sha1")
    .update(path.resolve(root || projectDir()))
    .digest("hex")
    .slice(0, 16);
  return path.join(os.homedir(), ".claude", "agentflow-mode", key + ".json");
}

function readMode(root) {
  const primary = modePath(root);
  const fallback = fallbackModePath(root);
  for (const p of [primary, fallback]) {
    try {
      if (!fs.existsSync(p)) continue;
      const data = JSON.parse(fs.readFileSync(p, "utf8"));
      if (data && data.enabled) {
        return { path: p, data };
      }
      // present but disabled still counts as a known mode file
      if (data) return { path: p, data };
    } catch (_) {
      // ignore corrupt files
    }
  }
  return null;
}

function ensureDir(filePath) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
}

function writeMode(enabled, extra, root) {
  const p = modePath(root);
  ensureDir(p);
  const prev = readMode(root);
  const base =
    prev && prev.data && typeof prev.data === "object" ? prev.data : {};
  const next = {
    ...base,
    enabled: !!enabled,
    updated_at: new Date().toISOString(),
    project_dir: path.resolve(root || projectDir()),
    version: AGENTFLOW_MODE_VERSION,
  };
  if (enabled && !base.enabled_at) {
    next.enabled_at = next.updated_at;
  }
  if (!enabled) {
    delete next.enabled_at;
  }
  if (extra && typeof extra === "object") {
    for (const [k, v] of Object.entries(extra)) {
      if (v === undefined || v === null || v === "") delete next[k];
      else next[k] = v;
    }
  }
  fs.writeFileSync(p, JSON.stringify(next, null, 2) + "\n", "utf8");
  return { path: p, data: next };
}

function disableMode(root) {
  return writeMode(false, null, root);
}

function writeStatus(snapshot, root) {
  const p = statusPath(root);
  ensureDir(p);
  const next = {
    ...(snapshot && typeof snapshot === "object" ? snapshot : {}),
    updated_at: new Date().toISOString(),
    project_dir: path.resolve(root || projectDir()),
  };
  fs.writeFileSync(p, JSON.stringify(next, null, 2) + "\n", "utf8");
  return { path: p, data: next };
}

function readStatus(root) {
  return readJsonFile(statusPath(root));
}

function enableMode(extra, root) {
  return writeMode(true, extra, root);
}

function isEnabled(root) {
  const m = readMode(root);
  return !!(m && m.data && m.data.enabled);
}

function readJsonFile(filePath) {
  try {
    if (!fs.existsSync(filePath)) return null;
    return JSON.parse(fs.readFileSync(filePath, "utf8"));
  } catch (_) {
    return null;
  }
}

function readUserSettings() {
  return readJsonFile(USER_SETTINGS_PATH);
}

function normalizeCommand(command) {
  return String(command || "").replace(/\\/g, "/").trim().toLowerCase();
}

function hasAgentflowUserPromptHook(settings) {
  const hooks = settings && settings.hooks && settings.hooks.UserPromptSubmit;
  if (!Array.isArray(hooks)) return false;
  return hooks.some((group) => {
    if (!group || !Array.isArray(group.hooks)) return false;
    return group.hooks.some((hook) => {
      if (!hook || hook.type !== "command") return false;
      return normalizeCommand(hook.command).includes("/agentflow/hooks/mode-inject.js");
    });
  });
}

function hasAgentflowStatusLine(settings) {
  const statusLine = settings && settings.statusLine;
  if (!statusLine || statusLine.type !== "command") return false;
  return normalizeCommand(statusLine.command).includes("/agentflow/hooks/statusline.js");
}

function stickyModeReadiness() {
  const settings = readUserSettings();
  return {
    settings_path: USER_SETTINGS_PATH,
    user_prompt_hook_installed: hasAgentflowUserPromptHook(settings),
    statusline_installed: hasAgentflowStatusLine(settings),
    expected_user_prompt_hook_command: USER_PROMPT_HOOK_COMMAND,
    expected_statusline_command: STATUSLINE_COMMAND,
  };
}

function buildAdditionalContext(modeData) {
  const lines = [
    "AGENTFLOW MODE ON.",
    "You are operating under agentflow sticky mode for this session.",
    "Rules:",
    "1. Prefer mcp__agentflow__* tools for project/task/DAG/worker operations.",
    "2. Always refer to tasks as `task_id + title` (never bare codes like RCPA3 alone).",
    "3. Starting a worker requires launch-ticket protocol:",
    "   task_prepare_start -> spawn real Agent subagent -> task_transition(start) with launch.ticket + worker_agent_id -> task_worker_sync on completion.",
    "4. Do not mark a task executing without a real bound worker_agent_id.",
    "5. DAG owns execution_branch + shared worktree; task only holds the current lease.",
    "6. While mode is on, treat ordinary user requests as agentflow work by default unless the user explicitly asks to leave agentflow mode.",
    "7. Every new turn belongs to the same agentflow working session: route it through /agentflow resume, /agentflow inspect, /agentflow goal, or the currently active agentflow flow instead of ad-hoc handling.",
    "8. Do not answer project-work requests in a freeform way first; first recover or continue the agentflow state, then act within that workflow.",
    "9. If the user gives a new project objective in normal language, interpret it as the next agentflow goal/intake step rather than leaving the workflow.",
    "10. When unsure which flow to use, invoke /agentflow resume or read the agentflow skill flows.",
  ];
  if (modeData) {
    if (modeData.namespace_id) {
      lines.push("Active namespace_id: " + modeData.namespace_id);
    }
    if (modeData.dag_id) {
      lines.push("Focus dag_id: " + modeData.dag_id);
    }
    if (modeData.note) {
      lines.push("Note: " + modeData.note);
    }
    if (modeData.project_dir) {
      lines.push("Mode project_dir: " + modeData.project_dir);
    }
  }
  lines.push(
    "Mode file: .claude/agentflow/mode.json (toggle with /agentflow on|off)."
  );
  return lines.join("\n");
}

module.exports = {
  AGENTFLOW_MODE_VERSION,
  MODE_REL,
  STATUS_REL,
  STATUSLINE_COMMAND,
  USER_PROMPT_HOOK_COMMAND,
  USER_SETTINGS_PATH,
  projectDir,
  modePath,
  statusPath,
  fallbackModePath,
  readMode,
  writeMode,
  enableMode,
  disableMode,
  writeStatus,
  readStatus,
  isEnabled,
  readJsonFile,
  readUserSettings,
  hasAgentflowUserPromptHook,
  hasAgentflowStatusLine,
  stickyModeReadiness,
  buildAdditionalContext,
};
