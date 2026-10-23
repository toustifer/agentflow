---
name: agentflow
description: Use agentflow MCP to organize project goals, task dependencies, worktrees, worker handoffs and reviews in Codex. Invoke explicitly for agentflow-managed work.
---

# Agentflow for Codex

Use `$agentflow` followed by `setup`, `init`, `goal`, `resume`, `inspect`, or the requested project goal. If the user types `/agentflow` as plain text, interpret the intent when this skill is loaded; this package does not register a custom slash-command handler.

Agentflow's Go MCP engine owns task state, launch tickets, worktrees and delivery records. Codex supplies actual worker/reviewer agents. Keep user scope, model choice and host permissions intact.

## Before business operations

Discover the currently available agentflow MCP tools and call `flow_ping`. Tool prefixes vary by host; use the names actually exposed. A config entry, `codex mcp list`, a standalone protocol probe, or an installer success message does not prove this session has working tools.

If tools are absent or fail, read [setup](references/setup.md) and report the setup problem. Do not advance task state by shell JSON-RPC, direct SQLite writes or fabricated agent IDs. Setup diagnostics may run protocol probes, but are never task execution evidence.

## Choose the operation

- **setup / update:** read [setup](references/setup.md). Do not run the Claude/DSH installer.
- **init:** bind an existing repository without resetting files, then establish a baseline; see [workflow](references/workflow.md).
- **goal / new request:** confirm unresolved scope, then plan and dispatch using that workflow. Preserve existing goals when a message is steering ongoing work.
- **resume:** inspect persisted tasks and real worker status before resuming. Do not spawn duplicates just because the leader session restarted.
- **inspect / status:** use read tools to report phase, dependencies, actual active workers, reviews and blockers. Do not change tasks merely to make the report look healthy.
- **on / off / mode:** this adapter has no sticky hook or statusline injection. Explain that explicit invocation selects the workflow; do not claim persistent per-turn interception or create Claude settings.

## Dispatch boundary

For execution, read [workflow](references/workflow.md). Prepare a launch ticket, spawn a real Codex subagent through the host's available delegation tool, record the actual returned identity, then start. A delegated worker must wait for the start acknowledgment before changing the worktree. The leader coordinates; workers implement; reviewers evaluate the recorded commit and diff.

If the host has no real delegation capability, inspection/planning can continue, but stop at the execution boundary with that limitation. Do not silently substitute a shell-launched model or invent a worker.

Existing `.claude/PROJECT_FINAL_SHAPE.md` and `.claude/agentflow-git.md` paths are engine-compatible project metadata, not a requirement to run Claude. Preserve them for existing namespaces. No DSH or Claude configuration is modified by this adapter.
