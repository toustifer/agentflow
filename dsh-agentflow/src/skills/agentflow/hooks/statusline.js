#!/usr/bin/env node
// Optional statusline helper for agentflow sticky mode.
// settings.json:
// {
//   "statusLine": {
//     "type": "command",
//     "command": "node <path-to>/statusline.js",
//     "refreshInterval": 5
//   }
// }
// Prints nothing when mode is off (so it can be chained or used alone).

const fs = require("fs");
const {
  readMode,
  readStatus,
  projectDir,
  probeAgentflowMcpConfig,
} = require("./mode-lib");

function readStdinJson() {
  try {
    if (process.stdin.isTTY) return null;
    const raw = fs.readFileSync(0, "utf8");
    if (!raw) return null;
    return JSON.parse(raw);
  } catch (_) {
    return null;
  }
}

function detectProjectDir(input) {
  return (
    (input && input.workspace && input.workspace.project_dir) ||
    (input && input.cwd) ||
    projectDir()
  );
}

function color(code, text) {
  return `\x1b[${code}m${text}\x1b[0m`;
}

function mcpBadge(mcp) {
  if (!mcp || !mcp.configured) {
    return color(31, "MCP:missing");
  }
  if (!mcp.paths_ok) {
    return color(31, "MCP:broken");
  }
  // Config OK only — session tools may still be absent
  return color(33, "MCP:cfg");
}

function renderFallback(mode, mcp) {
  const parts = [color(32, "agentflow:on"), mcpBadge(mcp)];
  if (mode.namespace_id) parts.push("ns=" + mode.namespace_id);
  if (mode.dag_id) parts.push("dag=" + mode.dag_id);
  return parts.join(" · ");
}

function pickDagLabel(status, mode) {
  if (mode.dag_id) return mode.dag_id;
  const dags = Array.isArray(status && status.dags) ? status.dags : [];
  const active = dags.find((item) => {
    const dag = item && item.dag;
    return dag && dag.status && dag.status !== "done";
  });
  const chosen = active || dags[0];
  const dag = chosen && chosen.dag;
  if (!dag) return null;
  return dag.title || dag.id || null;
}

function pickBusyWorkers(status) {
  const workers = Array.isArray(status && status.workers) ? status.workers : [];
  const busy = workers
    .filter((worker) => worker && worker.status === "busy")
    .map((worker) => worker.name || worker.id)
    .filter(Boolean);
  if (!busy.length) return null;
  const head = busy.slice(0, 2).join(",");
  const rest = busy.length - 2;
  return rest > 0 ? `${head},+${rest}` : head;
}

function renderSummary(status, mode, mcp) {
  const summary = status && status.summary;
  if (!summary) return renderFallback(mode, mcp);
  const dagLabel = pickDagLabel(status, mode);
  const line1 = [color(32, "agentflow:on"), mcpBadge(mcp)];
  if (dagLabel) line1.push(`dag:${dagLabel}`);
  line1.push(color(33, `working:${summary.running_count ?? 0}`));
  line1.push(color(32, `ready:${summary.ready_count ?? 0}`));
  line1.push(color(31, `blocked:${summary.blocked_count ?? 0}`));

  const phaseText = `${summary.phase_name || summary.phase || "active"} ${summary.progress || ""}`.trim();
  const line2 = [];
  if (phaseText) line2.push(color(2, phaseText));
  line2.push(
    color(2, `workers:${summary.worker_busy ?? 0}/${summary.worker_total ?? 0}`)
  );
  const busyWorkers = pickBusyWorkers(status);
  if (busyWorkers) line2.push(color(2, `busy:${busyWorkers}`));
  if (mcp && (!mcp.configured || !mcp.paths_ok)) {
    line2.push(color(31, "fix MCP before work"));
  }
  return `${line1.join(" · ")}\n${line2.join(" · ")}`;
}

function main() {
  const input = readStdinJson();
  const root = detectProjectDir(input);
  const m = readMode(root);
  if (!m || !m.data || !m.data.enabled) {
    process.stdout.write("");
    process.exit(0);
  }

  const status = readStatus(root);
  const mcp = probeAgentflowMcpConfig();
  process.stdout.write(renderSummary(status, m.data, mcp));
  process.exit(0);
}

try {
  main();
} catch (e) {
  process.stdout.write("");
  process.exit(0);
}
