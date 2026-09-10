#!/usr/bin/env node

const fs = require("fs");
const { projectDir, writeStatus } = require("./mode-lib");

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
    input?.project_dir ||
    input?.project?.workdir ||
    input?.workspace?.project_dir ||
    input?.cwd ||
    projectDir()
  );
}

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function text(value, fallback = "-") {
  if (value === undefined || value === null || value === "") return fallback;
  return String(value);
}

function taskLabel(task) {
  const t = asObject(task);
  return `${text(t.task_id)} ${text(t.title)}`;
}

function pushTaskBucket(lines, prefix, label, tasks) {
  const items = asArray(tasks);
  lines.push(`${prefix}${label}`);
  if (!items.length) {
    lines.push(`${prefix}  (empty)`);
    return;
  }
  for (const task of items) {
    lines.push(`${prefix}  - ${taskLabel(task)}`);
  }
}

function renderProject(snapshot) {
  const project = asObject(snapshot.project);
  const summary = asObject(snapshot.summary);
  const dags = asArray(snapshot.dags);
  const lines = [];
  lines.push(`Project ${text(project.namespace_id)} ${text(project.namespace_name)}`);
  lines.push(`phase: ${text(summary.phase_name || summary.phase)} · progress: ${text(summary.progress)}`);
  lines.push(
    `ready:${text(summary.ready_count, "0")} running:${text(summary.running_count, "0")} blocked:${text(summary.blocked_count, "0")} done:${text(summary.done_count, "0")} workers:${text(summary.worker_busy, "0")}/${text(summary.worker_total, "0")}`
  );
  for (const item of dags) {
    const dag = asObject(item.dag);
    const legacySuffix = dag.legacy ? " · legacy" : "";
    const prioritySuffix = dag.resume_priority && dag.resume_priority !== "default" ? ` · ${dag.resume_priority}` : "";
    lines.push(``);
    lines.push(`DAG ${text(dag.id)} ${text(dag.title)}${legacySuffix}${prioritySuffix}`);
    lines.push(
      `branch: ${text(dag.execution_branch || dag.branch)} · base: ${text(dag.base_branch)} · status: ${text(dag.status)} · completion: ${text(item.completion_pct, "0")}%`
    );
    pushTaskBucket(lines, "  ", "Ready", item.ready_tasks);
    pushTaskBucket(lines, "  ", "Running", item.active_tasks);
    pushTaskBucket(lines, "  ", "Blocked", item.blocked_tasks);
    pushTaskBucket(lines, "  ", "Done", item.done_task_items);
  }
  return lines.join("\n");
}

function renderDag(snapshot) {
  const focused = asObject(snapshot.focused_dag);
  const detail = asObject(snapshot.dag_detail);
  const dag = Object.keys(focused).length ? focused : asObject(detail.dag);
  const dags = asArray(snapshot.dags);
  const dagItem = dags.find((item) => text(asObject(item.dag).id, "") === text(dag.id, "")) || {};
  const lines = [];
  const legacySuffix = dag.legacy ? " · legacy" : "";
  const prioritySuffix = dag.resume_priority && dag.resume_priority !== "default" ? ` · ${dag.resume_priority}` : "";
  lines.push(`DAG ${text(dag.id)} ${text(dag.title)}${legacySuffix}${prioritySuffix}`);
  lines.push(
    `branch: ${text(dag.execution_branch || dag.branch)} · base: ${text(dag.base_branch)} · status: ${text(dag.status)} · completion: ${text(dagItem.completion_pct, "0")}%`
  );
  lines.push("");
  pushTaskBucket(lines, "", "Ready", dagItem.ready_tasks);
  pushTaskBucket(lines, "", "Running", dagItem.active_tasks);
  pushTaskBucket(lines, "", "Blocked", dagItem.blocked_tasks);
  pushTaskBucket(lines, "", "Done", dagItem.done_task_items);
  const nodes = asArray(detail.nodes);
  if (nodes.length) {
    lines.push("");
    lines.push("Depends on");
    for (const node of nodes) {
      const n = asObject(node);
      const deps = asArray(n.depends_on);
      lines.push(`- ${text(n.task_id)} ${text(n.title)} <= ${deps.length ? deps.join(", ") : "none"}`);
    }
  }
  return lines.join("\n");
}

function renderTask(snapshot) {
  const detail = asObject(snapshot.task_detail);
  const runtime = asObject(snapshot.task_runtime);
  const lines = [];
  lines.push(taskLabel(detail));
  lines.push(`state: ${text(detail.state)}`);
  lines.push(`assigned_worker: ${text(detail.assigned_worker)}`);
  lines.push(`blocked_by: ${asArray(detail.blocked_by).join(", ") || "none"}`);
  lines.push(`available_transitions: ${asArray(detail.available_transitions).join(", ") || "none"}`);
  lines.push(`worker_status: ${text(detail.worker_status)}`);
  lines.push(`branch: ${text(runtime.execution_branch || runtime.branch)}`);
  lines.push(`base_branch: ${text(runtime.base_branch)}`);
  lines.push(`worktree_path: ${text(runtime.worktree_path)}`);
  lines.push(`active_task_id: ${text(runtime.active_task_id)}`);
  lines.push(`lease_holder_task_id: ${text(runtime.lease_holder_task_id)}`);
  lines.push(`lease_holder_worker_id: ${text(runtime.lease_holder_worker_id)}`);
  lines.push(`lease_holder_agent_id: ${text(runtime.lease_holder_agent_id)}`);
  return lines.join("\n");
}

function renderBlockers(snapshot) {
  const blockers = asArray(snapshot.blockers);
  const lines = ["Blockers"];
  if (!blockers.length) {
    lines.push("(empty)");
    return lines.join("\n");
  }
  for (const blocker of blockers) {
    const b = asObject(blocker);
    lines.push(`- [${text(b.dag_id)}] ${text(b.task_id)} ${text(b.title)} · type=${text(b.type)} · blocked_by=${asArray(b.blocked_by).join(", ") || "none"}`);
  }
  return lines.join("\n");
}

function renderWorkers(snapshot) {
  const workers = asArray(snapshot.workers);
  const lines = ["Workers"];
  if (!workers.length) {
    lines.push("(empty)");
    return lines.join("\n");
  }
  for (const worker of workers) {
    const w = asObject(worker);
    const meta = asObject(w.metadata);
    lines.push(`- ${text(w.id)} ${text(w.name)} · status=${text(w.status)} · total_tasks=${text(meta.total_tasks, "-")} · done_tasks=${text(meta.done_tasks, "-")}`);
  }
  return lines.join("\n");
}

function renderNext(snapshot) {
  const tasks = asArray(snapshot.next_tasks);
  const lines = ["Ready queue"];
  if (!tasks.length) {
    lines.push("(empty)");
    return lines.join("\n");
  }
  for (const task of tasks) {
    const t = asObject(task);
    lines.push(`- [${text(t.dag_id)}] ${taskLabel(t)} · worker=${text(t.assigned_worker)} · reason=${text(t.reason)}`);
  }
  return lines.join("\n");
}

function renderSnapshot(snapshot) {
  const focus = text(snapshot.focus, "project");
  if (focus === "dag") return renderDag(snapshot);
  if (focus === "task") return renderTask(snapshot);
  if (focus === "blockers") return renderBlockers(snapshot);
  if (focus === "workers") return renderWorkers(snapshot);
  if (focus === "next") return renderNext(snapshot);
  return renderProject(snapshot);
}

function main() {
  const input = readStdinJson();
  if (!input || typeof input !== "object") {
    process.stderr.write("Expected JSON snapshot on stdin\n");
    process.exit(2);
  }
  const root = detectProjectDir(input);
  writeStatus(input, root);
  process.stdout.write(renderSnapshot(input) + "\n");
}

main();
