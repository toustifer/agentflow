# Report Stuck Spec

## Goal

Make `report_stuck` a real leader action in the Python BT sidecar so the leader can surface a stable, machine-readable stuck snapshot when a namespace enters the stuck phase.

This action is read-only. It does not transition task state, mutate DAG state, or attempt recovery. It only exposes the current stuck situation in a form that later actions, tooling, or the user can consume.

## Why this action exists

After `dispatch_task` and `monitor_tasks`, the next missing leader action is the blocked-path observation branch.

Today the project can detect a stuck namespace through `project_next_steps`, but the BT branch still ends in a placeholder action. That means the leader tree can identify the phase, yet cannot emit a stable contract describing:

- which tasks are stuck
- why they are stuck
- what transitions are still available
- which follow-up actions the leader should consider

`report_stuck` closes that gap.

## Current inconsistency to fix first

`trees/leader-default.json` currently places `report_stuck` under the `phase_is_execute` branch:

```json
{
  "type": "Sequence",
  "children": [
    { "type": "Condition", "properties": { "fn": "phase_is_execute" } },
    {
      "type": "Fallback",
      "children": [
        ...,
        {
          "type": "Sequence",
          "children": [
            { "type": "Condition", "properties": { "fn": "has_stuck_tasks" } },
            { "type": "Action", "properties": { "fn": "report_stuck" } }
          ]
        }
      ]
    }
  ]
}
```

But `pkg/server/handlers_nextsteps.go` returns `phase = "stuck"` when there are no next tasks and no active tasks, and only then includes `stuck_tasks`.

So the `report_stuck` branch is not reachable in the real stuck case.

### Required tree fix

Move `report_stuck` to its own top-level phase branch:

```json
{
  "type": "Sequence",
  "children": [
    { "type": "Condition", "properties": { "fn": "phase_is_stuck" } },
    { "type": "Action", "properties": { "fn": "report_stuck" } }
  ]
}
```

That change is part of this implementation, not optional cleanup.

## Scope

This iteration includes:

1. a narrow Go provider that returns a normalized stuck snapshot for one namespace
2. a Python client for that provider
3. a real `report_stuck_action` in `bt_service/server/builtin.py`
4. leader tree wiring so the stuck branch is reachable
5. tests for Go provider behavior, Python action behavior, and end-to-end leader tick behavior

This iteration does not include:

- automatic recovery logic
- task transitions such as `resume`, `reassign`, or `cancel`
- writing diaries or docs
- user-facing remediation text generation beyond stable blackboard fields

## Behavior contract

### Preconditions

`report_stuck` only succeeds when all of the following are true:

- `namespace_id` is present
- `phase == "stuck"`
- `has_stuck_tasks == true`
- `stuck_tasks` is a non-empty list

If any precondition fails, the action returns `(False, ValueError(...))` and does not call the provider.

### Action semantics

The action selects the first item from `stuck_tasks` as the anchor task and asks Go for a namespace-level stuck snapshot.

Why first task: this matches the existing leader action style used by `dispatch_task` and `monitor_tasks`, where the action focuses on one canonical task while still allowing Go to return a broader summary.

The provider returns:

- a normalized copy of the anchor stuck task
- the task's available transitions
- namespace-level blockers from `project_blockers`
- a computed blocker summary
- a suggested leader follow-up action list

The Python action writes these values back to the blackboard using stable `last_stuck_*` keys.

## Provider contract

### Endpoint

Go sidecar provider:

- env: `AGENTFLOW_BT_STUCK_URL`
- env: `AGENTFLOW_BT_STUCK_TOKEN`
- path: `/report-stuck`
- method: `POST`
- auth header: `X-Agentflow-BT-Token`

### Request

```json
{
  "namespace_id": "ns-demo",
  "task_id": "T3"
}
```

`task_id` is the anchor task selected by Python from `stuck_tasks[0]`.

### Response

```json
{
  "task_id": "T3",
  "title": "Integrate payment webhook",
  "state": "rework_needed",
  "assigned_worker": "worker-a",
  "dag_id": "dag-1",
  "available_transitions": ["resume", "reassign", "cancel"],
  "blockers": [
    {
      "task_id": "T3",
      "title": "Integrate payment webhook",
      "dag_id": "dag-1",
      "type": "dependency",
      "blocked_by": "T2"
    }
  ],
  "blocker_summary": {
    "total": 1,
    "dependency": 1,
    "worker": 0
  },
  "suggested_actions": [
    "task_get",
    "project_blockers"
  ]
}
```

### Response rules

- `task_id`, `state`, and `suggested_actions` are always present on success.
- `available_transitions` is derived from `engine.AvailableTransitions(task)` and flattened to transition names.
- `blockers` comes from `engine.ProjectBlockers(ctx, namespaceID)`.
- `blocker_summary` is derived from `blockers`, not stored anywhere else.
- `suggested_actions` is conservative and read-only in this iteration.

### Suggested action policy

For this iteration:

- always include `task_get`
- include `project_blockers` when blocker list is non-empty
- do not emit mutation actions such as `task_transition resume` yet

So the first implementation should usually emit either:

```json
["task_get"]
```

or:

```json
["task_get", "project_blockers"]
```

## Blackboard contract

On success, Python writes:

- `last_stuck_task_id`
- `last_stuck_title`
- `last_stuck_state`
- `last_stuck_worker`
- `last_stuck_dag_id`
- `last_stuck_transitions`
- `last_stuck_blockers`
- `last_stuck_blocker_summary`
- `last_stuck_suggested_actions`

Example:

```json
{
  "last_stuck_task_id": "T3",
  "last_stuck_title": "Integrate payment webhook",
  "last_stuck_state": "rework_needed",
  "last_stuck_worker": "worker-a",
  "last_stuck_dag_id": "dag-1",
  "last_stuck_transitions": ["resume", "reassign", "cancel"],
  "last_stuck_blockers": [
    {
      "task_id": "T3",
      "title": "Integrate payment webhook",
      "dag_id": "dag-1",
      "type": "dependency",
      "blocked_by": "T2"
    }
  ],
  "last_stuck_blocker_summary": {
    "total": 1,
    "dependency": 1,
    "worker": 0
  },
  "last_stuck_suggested_actions": ["task_get", "project_blockers"]
}
```

The action does not overwrite `phase`, `stuck_tasks`, or any `last_monitored_*` / `last_dispatch_*` fields.

## Error behavior

### Python action failures

Return `(False, err)` when:

- `namespace_id` is missing
- `phase != "stuck"`
- `has_stuck_tasks != true`
- `stuck_tasks` is empty or malformed
- provider request fails
- provider returns a non-object

### Go provider HTTP status mapping

- `400` when `namespace_id` or `task_id` is missing
- `404` when the anchor task does not exist
- `409` when the anchor task is not in a stuck-compatible state for reporting
- `500` for unexpected internal errors

### Stuck-compatible states

For this iteration, the provider accepts anchor tasks in:

- `assigned`
- `rework_needed`
- `cancelled` is not accepted
- `done` is not accepted
- `executing` and `review_pending` are not accepted for `report_stuck`, because the project should not be in `phase=stuck` if active work exists

This protects the provider from being used out of phase.

## Go implementation design

### New file

`pkg/server/bt_stuck_provider.go`

### New types

```go
type reportStuckRequest struct {
    NamespaceID string `json:"namespace_id"`
    TaskID      string `json:"task_id"`
}

type reportStuckResponse struct {
    TaskID               string                   `json:"task_id"`
    Title                string                   `json:"title,omitempty"`
    State                string                   `json:"state"`
    AssignedWorker       string                   `json:"assigned_worker,omitempty"`
    DAGID                string                   `json:"dag_id,omitempty"`
    AvailableTransitions []string                 `json:"available_transitions,omitempty"`
    Blockers             []map[string]any         `json:"blockers,omitempty"`
    BlockerSummary       map[string]any           `json:"blocker_summary,omitempty"`
    SuggestedActions     []string                 `json:"suggested_actions,omitempty"`
}
```

A helper with the same style as the prior actions:

```go
func (s *Server) reportStuckOnce(ctx context.Context, namespaceID, taskID string) (reportStuckResponse, error)
```

### Implementation rules

1. Load the anchor task with `GetTask`.
2. Reject invalid states.
3. Read blockers with `ProjectBlockers`.
4. Compute `blocker_summary` counts.
5. Flatten `AvailableTransitions(task)` to strings.
6. Return the normalized response.
7. Do not transition or persist anything.

## Python implementation design

### New file

`bt_service/server/stuck_client.py`

### API

```python
class ReportStuckProviderError(RuntimeError):
    pass


def report_stuck(namespace_id: str, task_id: str) -> dict:
```

This client should mirror the existing dispatch/monitor clients.

### builtin wiring

In `bt_service/server/builtin.py`:

- import `report_stuck`, `ReportStuckProviderError`
- register `report_stuck` as a real action, not `_noop_action`
- implement `report_stuck_action(bb: Blackboard) -> tuple`

## Tree changes

### `trees/leader-default.json`

Replace the unreachable nested `has_stuck_tasks -> report_stuck` branch under execute with a top-level stuck-phase branch.

Final relevant leader shape:

```json
{
  "type": "Sequence",
  "children": [
    { "type": "Condition", "properties": { "fn": "phase_is_execute" } },
    {
      "type": "Fallback",
      "children": [
        {
          "type": "Sequence",
          "children": [
            { "type": "Condition", "properties": { "fn": "has_next_tasks" } },
            { "type": "Action", "properties": { "fn": "dispatch_task" } }
          ]
        },
        {
          "type": "Sequence",
          "children": [
            { "type": "Condition", "properties": { "fn": "has_active_tasks" } },
            { "type": "Action", "properties": { "fn": "monitor_tasks" } }
          ]
        }
      ]
    }
  ]
},
{
  "type": "Sequence",
  "children": [
    { "type": "Condition", "properties": { "fn": "phase_is_stuck" } },
    { "type": "Action", "properties": { "fn": "report_stuck" } }
  ]
}
```

### `pkg/server/bt_factory.go`

Update the embedded `leaderDefaultJSON` constant to match the file version.

## leader_tick response expectations

`handleLeaderTick` should continue to return the stable top-level phase fields from `outputs`, but once `report_stuck` is real and the tree reaches the stuck branch, the returned `blackboard` from raw BT tick calls should contain the new `last_stuck_*` keys.

For this iteration, `handleLeaderTick` itself does not need to promote `last_stuck_*` fields into the top-level MCP response. The BT payload already exposes them through `blackboard`, and the end-to-end integration test can assert that.

## Tests

### Python unit tests

New file: `bt_service/tests/test_report_stuck_action.py`

Add cases for:

1. `test_report_stuck_requires_namespace_id`
2. `test_report_stuck_requires_stuck_phase`
3. `test_report_stuck_requires_stuck_tasks`
4. `test_report_stuck_provider_success_writes_blackboard`
5. `test_report_stuck_provider_error_returns_failure`

### Go provider tests

New file: `pkg/server/bt_stuck_provider_test.go`

Add cases for:

1. `TestReportStuckOnceReturnsAssignedTaskSnapshot`
2. `TestReportStuckOnceReturnsReworkNeededTaskSnapshot`
3. `TestReportStuckOnceRejectsExecutingTask`
4. `TestReportStuckProviderHandlesValidationAndTaskNotFound`
5. `TestReportStuckOnceIncludesProjectBlockers`

### Sidecar integration tests

Extend `pkg/server/bt_sidecar_integration_test.go` with:

1. `TestLeaderTickReportStuckViaPython`

Suggested scenario:

- create namespace, worker, DAG
- create at least one task that leaves the project in `phase=stuck`
- easiest valid setup: one `rework_needed` task and no executing/review_pending tasks
- first `handleLeaderTick` should return `phase = stuck`
- direct bridge `tick(... return_blackboard=true)` should expose:
  - `last_stuck_task_id`
  - `last_stuck_state`
  - `last_stuck_transitions`
  - `last_stuck_suggested_actions`

## Acceptance criteria

Implementation is complete when all of the following are true:

1. `report_stuck` is no longer a placeholder in Python.
2. the leader tree can actually reach `report_stuck` during `phase=stuck`.
3. the Go provider returns a stable stuck snapshot.
4. the Python action writes stable `last_stuck_*` keys.
5. all new and existing BT tests pass.
6. `go test -count=1 ./pkg/...` stays green.
7. `python -m pytest bt_service/tests/ -q` stays green.

## Follow-up after this spec

Once this is done, the natural next action is `report_done`, because the leader mainline will then cover:

- dispatch path
- active-task monitoring path
- stuck-path reporting path
- done-path reporting path

That gives the leader tree a complete read/observe surface before moving on to mutation or recovery logic.
