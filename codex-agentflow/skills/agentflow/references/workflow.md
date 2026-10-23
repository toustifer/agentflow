# Native Codex workflow

Always inspect live MCP schemas; the names below are tool identities, not hardcoded host prefixes. The engine enforces the transition contract. Read errors rather than guessing missing parameters.

## Bind, inspect, plan

1. Call `flow_ping`; use `namespace_list` filtered to the current repository before creating anything. Existing binding → resume, not duplicate initialization.
2. For a new binding use `project_init` with the user's repository and base branch. Read the generated `.claude/agentflow-git.md`. Do not reset uncommitted work or invent a base branch. Ensure a first commit exists before worktree dispatch.
3. Inspect `project_next_steps` / `project_inspect`. For a new goal, clarify unresolved product boundaries, reuse or amend the existing `.claude/PROJECT_FINAL_SHAPE.md`, and record the agreed scope. Do not expand a minor request into a replacement product.
4. Register actual worker roles with `kind`, scope, `prompt_template`, required reads and `launch_mode`; use `worker_prompt_get` as preflight. Create a DAG with its explicit execution branch, then dependency-aware tasks through the exposed task APIs.
5. With multiple active DAGs, select an explicit `dag_id`; never dispatch arbitrarily across them.

## Prepare → real spawn → start

`leader_tick` / `project_next_steps` suggest next work. BT dispatch is preparation only; `lifecycle_tick` is diagnostic glue, not the production execution loop.

1. Read the task and call `task_prepare_start`. Capture the returned launch ticket, briefing, worktree path, execution branch and required reads. State remains assigned at this point.
2. Inspect any task `route.provider` / `route.model` and the prepared launch requirements before spawning. Honor the user's recorded route only if the host supports it; do not invent or silently substitute a model. If no route is specified, preserve the host's configured defaults. Spawn a real worker via the available Codex host tool (`spawn_agent` or the host's equivalent). Pass a self-contained task, exact prepared worktree path and required reads. Do not request another automatic worktree on top of the engine-prepared path. A host API without a cwd option requires the worker to explicitly use that path for every filesystem/command operation.
3. Tell the worker to acknowledge its identity and wait for a start acknowledgment before editing. The leader uses the actual identity returned by the spawn operation, never a synthetic `bt:*`, task ID or PID substituted as an agent identity.
4. Call `task_transition` with `action=start` and the metadata required by the live schema:

```text
launch.ticket     = exact issued ticket
worker_agent_id   = real spawned Codex worker identity
runtime.provider  = "codex"
runtime.status    = "started"
runtime.model     = actual selected model when route.model is specified
```

If the recorded provider is incompatible with the available Codex runtime, report it before launch; do not claim a different runtime provider. A required model that the host cannot select or verify is a launch blocker, not a reason to record the requested model as though it was used.

5. Only after successful transition, send the worker the start acknowledgment. If transition fails, keep it waiting or stop it through the available host API; resolve the engine error or report the blocker. Do not leave an untracked worker editing files.
6. Use the host's messages/wait tools to monitor actual execution; reflect status via `task_worker_sync` according to its schema. A timeout or host restart is not evidence of completion. Discover whether a worker still exists before relaunching it.

The main checkout remains on its base branch. Workers operate only inside the prepared task/DAG worktree. A shared DAG execution branch/worktree has one active lease; do not dispatch concurrent writers into it. Parallelize only independently leased worktrees.

## Worker delivery and review

The worker reads `task_get`, applicable project rules and relevant `doc_search` results, confirms worktree/branch, implements the assigned scope, and runs appropriate tests. It commits the task changes, records reusable documentation and a `worker_diary_write` entry, and submits through `task_transition` using the live schema. Clean worktree, diary, recorded `review.commit` and `review.diff` are delivery requirements.

The reviewer is a real separate agent. Pass it the recorded commit/diff, task acceptance criteria and worktree path. The reviewer returns pass/rework with reasons; the leader records the corresponding engine transition. Do not equate a worker's “done” message with reviewed engine completion. Review does not authorize pushing or merging unless the user's scope does.

On resume, inspect phase, task state, launch data and actual host workers. Reconcile using supported APIs; never edit SQLite or manufacture worker events. If a prior worker cannot be recovered, report it and follow the engine's retry/rework path after inspecting the worktree.

When blocked, describe the concrete missing prerequisite and retained state. When done, report reviewed deliverables, validation and remaining limitations. Keep existing project state and history for the next session.
