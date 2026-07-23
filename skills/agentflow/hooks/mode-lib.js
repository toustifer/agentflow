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
// Derive from this skill install (portable across machines; do not hardcode a username).
const HOOKS_DIR = __dirname;
const USER_PROMPT_HOOK_COMMAND = "node " + path.join(HOOKS_DIR, "mode-inject.js");
const STATUSLINE_COMMAND = "node " + path.join(HOOKS_DIR, "statusline.js");

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

function claudeJsonPath() {
  return path.join(os.homedir(), ".claude.json");
}

function expandUserPath(p) {
  if (p == null || p === "") return p;
  let s = String(p);
  if (s.startsWith("~/") || s === "~") {
    s = path.join(os.homedir(), s.slice(1));
  }
  // Windows: expand %USERPROFILE% etc. lightly
  s = s.replace(/%USERPROFILE%/gi, os.homedir());
  s = s.replace(/%HOME%/gi, os.homedir());
  return s;
}

function isLauncherCommand(cmd) {
  const base = path
    .basename(String(cmd || ""))
    .toLowerCase()
    .replace(/\.exe$/, "");
  return (
    base === "node" ||
    base === "cmd" ||
    base === "npx" ||
    base === "bash" ||
    base === "sh" ||
    base === "zsh" ||
    base === "powershell" ||
    base === "pwsh"
  );
}

function looksLikeAgentflowPath(arg) {
  if (typeof arg !== "string") return false;
  if (arg.startsWith("-")) return false;
  const lower = arg.toLowerCase().replace(/\\/g, "/");
  return (
    lower.includes("agentflow") ||
    lower.endsWith(".mjs") ||
    lower.endsWith("/agentflow") ||
    lower.endsWith("agentflow.exe")
  );
}

function collectAgentflowMcpEntries(cfg) {
  const found = [];
  if (!cfg || typeof cfg !== "object") return found;
  const top = cfg.mcpServers && cfg.mcpServers.agentflow;
  if (top) {
    found.push({ source: "user:~/.claude.json", entry: top });
  }
  const projects = cfg.projects;
  if (projects && typeof projects === "object") {
    for (const [proj, pdata] of Object.entries(projects)) {
      const e = pdata && pdata.mcpServers && pdata.mcpServers.agentflow;
      if (e) found.push({ source: "project:" + proj, entry: e });
    }
  }
  return found;
}

/**
 * Config-level probe only (hooks cannot see the model's live tool list).
 * Session-level truth still requires the agent to see mcp__agentflow__* tools.
 */
function probeAgentflowMcpConfig() {
  const cfgPath = claudeJsonPath();
  const cfg = readJsonFile(cfgPath);
  const found = collectAgentflowMcpEntries(cfg);
  if (!found.length) {
    return {
      configured: false,
      paths_ok: false,
      status: "missing",
      reason:
        "No mcpServers.agentflow in ~/.claude.json (user or project). /mcp will not list agentflow.",
      config_path: cfgPath,
      sources: [],
    };
  }

  const sources = [];
  const missing_paths = [];
  for (const item of found) {
    const entry = item.entry || {};
    const candidates = [];
    if (entry.command && !isLauncherCommand(entry.command)) {
      candidates.push(entry.command);
    }
    if (Array.isArray(entry.args)) {
      for (const a of entry.args) {
        if (looksLikeAgentflowPath(a)) candidates.push(a);
      }
    }
    const missing = [];
    for (const c of candidates) {
      const exp = expandUserPath(c);
      try {
        if (!fs.existsSync(exp)) missing.push(c);
      } catch (_) {
        missing.push(c);
      }
    }
    sources.push({
      source: item.source,
      command: entry.command,
      args: entry.args,
      type: entry.type,
      missing_paths: missing,
    });
    for (const m of missing) missing_paths.push(m);
  }

  const paths_ok = missing_paths.length === 0;
  return {
    configured: true,
    paths_ok,
    status: paths_ok ? "configured" : "broken_path",
    reason: paths_ok
      ? "agentflow MCP is configured in ~/.claude.json (still must appear as connected tools in THIS session)"
      : "agentflow MCP configured but path missing: " + missing_paths.join(", "),
    config_path: cfgPath,
    sources,
    missing_paths,
  };
}

function mcpGateBanner(mcp) {
  if (!mcp) return null;
  if (!mcp.configured) {
    return (
      "CONFIG ALERT: agentflow MCP is NOT configured (" +
      mcp.reason +
      "). Sticky mode is ON but execution is BLOCKED until MCP is fixed. " +
      "User action: install binary + add mcpServers.agentflow, then /mcp must show agentflow connected (not failed)."
    );
  }
  if (!mcp.paths_ok) {
    return (
      "CONFIG ALERT: agentflow MCP path broken (" +
      mcp.reason +
      "). Fix paths in ~/.claude.json and restart Claude Code before any project work."
    );
  }
  return (
    "CONFIG NOTE: agentflow appears in ~/.claude.json (" +
    mcp.status +
    "). This is NOT proof that THIS session has mcp__agentflow__* tools — verify the live tool list every turn."
  );
}

function buildAdditionalContext(modeData) {
  const mcp = probeAgentflowMcpConfig();
  const lines = [
    "AGENTFLOW MODE ON.",
    "You are operating under agentflow sticky mode for this session.",
    "Rules:",
    "0. MCP GATE (HARD — run every turn BEFORE any project/task/DAG work):",
    "   a. Inspect YOUR CURRENT tool list for names starting with `mcp__agentflow__`.",
    "   b. If NONE are present, or any `mcp__agentflow__*` call fails/is denied/is missing:",
    "      - STOP all product work and agentflow lifecycle immediately.",
    "      - Do NOT use Bash/shell to run `agentflow`, `agentflow stdio`, raw JSON-RPC tools/call,",
    "        direct sqlite against the agentflow DB, or any other substitute for MCP tools.",
    "      - Do NOT invent success (prepare/start/submit/pass) or claim agentflow compliance without real mcp__agentflow__* calls.",
    "      - Tell the user clearly, in the first response lines:",
    "        「agentflow MCP 在本会话不可用，请先修好 MCP，不要继续旁路推进。」",
    "      - Instruct the user to: open `/mcp` → ensure server `agentflow` is listed and NOT failed;",
    "        fix `~/.claude.json` / project mcp config; restart Claude Code; then re-check that",
    "        `mcp__agentflow__flow_ping` (or any mcp__agentflow__* tool) appears and succeeds.",
    "      - Follow `flows/setup.md` only for install/repair guidance. Do not proceed to goal/resume/execute.",
    "   c. `claude mcp list` showing Connected is NOT enough. Only tools actually available to you THIS turn count.",
    "   d. Prefer fixing MCP over any workaround. Workarounds are FORBIDDEN while mode is on.",
    "   e. `agentflow:on` in statusline only means mode.json is enabled — it does NOT mean MCP works.",
    "1. After MCP gate passes: use ONLY `mcp__agentflow__*` tools for project/task/DAG/worker operations.",
    "2. Always refer to tasks as `task_id + title` (never bare codes like RCPA3 alone).",
    "3. Starting a worker requires launch-ticket protocol:",
    "   task_prepare_start -> spawn real Agent subagent -> task_transition(start) with launch.ticket + real worker_agent_id -> task_worker_sync on completion.",
    "4. Do not mark a task executing without a real bound worker_agent_id from a spawned Agent subagent.",
    "5. LEADER MUST NOT IMPLEMENT PRODUCT CODE.",
    "   Leader may only: inspect/resume/plan, prepare_start, spawn Agent, transition/sync, diary/doc/shape files under .claude/.",
    "   Leader must NOT Write/Edit product source, run product commits on execution_branch, or hand-finish worker tasks in the main session.",
    "   If prepare/start/worktree fails: repair git ownership or escalate; never implement the task yourself as fallback.",
    "6. Branch ownership:",
    "   - Main session / primary workdir stays on base_branch (usually main).",
    "   - Never checkout DAG execution_branch in the primary workdir.",
    "   - Workers implement only inside dag shared worktree_path.",
    "7. DAG owns execution_branch + shared worktree; task only holds the current lease. One lease holder at a time on a shared branch.",
    "8. While mode is on, treat ordinary user requests as agentflow work by default unless the user explicitly asks to leave agentflow mode.",
    "9. Every new turn belongs to the same agentflow working session: route it through /agentflow resume, /agentflow inspect, /agentflow goal, or the currently active agentflow flow instead of ad-hoc handling.",
    "10. Do not answer project-work requests in a freeform way first; first recover or continue the agentflow state, then act within that workflow.",
    "11. If the user gives a new project objective in normal language, interpret it as the next agentflow goal/intake step rather than leaving the workflow.",
    "12. When unsure which flow to use, invoke /agentflow resume or read the agentflow skill flows.",
  ];
  const banner = mcpGateBanner(mcp);
  if (banner) lines.push(banner);
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
  probeAgentflowMcpConfig,
  buildAdditionalContext,
};
