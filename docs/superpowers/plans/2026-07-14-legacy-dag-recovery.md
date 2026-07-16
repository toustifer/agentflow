# Legacy DAG Recovery And Resume Targeting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop legacy DAGs from hijacking `/agentflow resume`, add an explicit `/agentflow resume dag <dag_id>` bypass, and make invalid legacy worktrees recoverable only when that DAG is intentionally resumed.

**Architecture:** Keep the Go backend as the source of truth. Fix the problem in two layers: first, select the right DAG at resume/leader time by de-prioritizing legacy DAGs that are superseded or cancelled; second, only for intentionally resumed DAGs, add a controlled worktree repair path instead of the current unconditional hard failure. The skill layer gets a targeted DAG resume syntax, but the permanent fix lives in backend state selection and recovery logic.

**Tech Stack:** Go, MCP server handlers, in-memory/sqlite engine store, Claude Code skill markdown flows, git worktree management.

---

## File Map

### Existing files to modify

- `pkg/engine/dag.go`
  - DAG status/state model. Extend lifecycle helpers so legacy DAGs can be formally de-prioritized without pretending they are still active.
- `pkg/server/handlers_nextsteps.go`
  - Current project phase and next-task selection. Today it picks the first non-empty-status DAG, which is too naive for legacy DAGs.
- `pkg/server/bt_leader.go`
  - `leader_tick` consumes `project_next_steps`; this file does not need new policy itself, but tests should confirm the new phase payload is correct here.
- `pkg/server/git_worktree.go`
  - Current `ensureTaskWorktree(...)` hard-fails when the path exists but is not a valid worktree.
- `pkg/server/mcp.go`
  - `handleTaskPrepareStart(...)` and any new MCP inputs for targeted resume / repair hints.
- `pkg/server/handlers_ext.go`
  - `project_inspect(...)` and related status payloads should surface whether a DAG is legacy/de-prioritized and why.
- `pkg/server/mcp_test.go`
  - Existing tests for worktree reuse and prepare-start behavior live here; extend them for legacy DAG de-prioritization and targeted repair behavior.
- `skills/agentflow/SKILL.md`
  - Add `/agentflow resume dag <dag_id>` syntax to routing docs.
- `skills/agentflow/flows/resume.md`
  - Define default resume vs targeted resume semantics and how legacy DAGs are treated.
- `C:/Users/15775/.claude/skills/agentflow/SKILL.md`
  - Mirror live skill copy.
- `C:/Users/15775/.claude/skills/agentflow/flows/resume.md`
  - Mirror live skill copy.

### New files to create

- `pkg/server/legacy_dag_policy.go`
  - Small focused file for legacy-DAG selection policy and helper functions. Keep this logic out of the larger handlers.
- `pkg/server/legacy_dag_policy_test.go`
  - Focused tests for DAG ordering / de-prioritization without mixing them into huge end-to-end MCP tests.
- `pkg/server/worktree_repair.go`
  - Focused helper for “path exists but is not a valid worktree” detection and opt-in repair.
- `pkg/server/worktree_repair_test.go`
  - Focused tests for invalid worktree detection and repair gating.

## Problem Definition

Current failure mode:

1. `handleProjectNextSteps(...)` in `pkg/server/handlers_nextsteps.go` chooses the first DAG whose `Status != ""`.
2. A legacy DAG like `provider-marketplace-integration-v1` remains `in_progress`, so it becomes the active DAG even when newer work has already superseded it.
3. `leader_tick` consumes that phase payload and recommends `pmi-*` tasks.
4. `task_prepare_start` reaches `prepareTaskGitRuntime(...)` in `pkg/server/git_worktree.go`.
5. `ensureTaskWorktree(...)` sees an existing path that is not a valid worktree and returns:
   `路径 %q 已存在但不是有效 worktree，请先手动清理`
6. The system is now blocked on a legacy execution line that should either be formally closed or intentionally resumed with repair.

The permanent bug is not “worktree bad” by itself. The permanent bug is:

- legacy DAGs are not formally closed or de-prioritized
- default resume has no DAG targeting escape hatch
- worktree recovery is all-or-nothing

## Recommended Solution

### Permanent fix

1. **Formalize legacy DAG de-prioritization in backend selection logic**
   - A DAG should not remain the default active DAG merely because it still says `in_progress`.
   - If it is superseded, cancelled, or explicitly marked legacy, `project_next_steps` must skip it by default.

2. **Keep explicit DAG targeting as an operator override**
   - `/agentflow resume dag <dag_id>` should intentionally focus a DAG even if default resume would skip it.

3. **Gate worktree repair behind explicit intent**
   - Default resume should not auto-repair a legacy broken worktree.
   - Targeted resume or explicit prepare-start repair should be allowed to repair/recreate the worktree.

### Temporary fix

- `/agentflow resume dag <dag_id>` is the temporary operator bypass.
- It solves “I know which DAG I want right now.”
- It is not the permanent state-machine fix because default `/agentflow resume` would still be wrong without legacy DAG de-prioritization.

## State Machine Changes

### DAG lifecycle policy

Keep `DAGCancelled` as the explicit retired state. Do **not** add a new `archived` status unless current requirements force it.

Instead, add metadata-driven policy for de-prioritization:

- `dag.metadata.legacy = true`
- `dag.metadata.superseded_by = <dag_id>`
- `dag.metadata.resume_priority = default|deprioritized|force`

Recommended runtime behavior:

- `done` and `cancelled` are never chosen as default active DAGs.
- `in_progress` DAGs with `resume_priority=deprioritized` or `legacy=true` are skipped unless explicitly targeted.
- `in_progress` DAGs with no ready tasks and only invalid worktree blockers are not chosen over a healthier newer DAG.

### Worktree recovery policy

- Default `task_prepare_start`:
  - if worktree is valid: reuse it
  - if missing: create it
  - if invalid existing path: fail with a structured repair-needed error
- Explicit targeted repair path:
  - if operator intentionally resumed that DAG, allow removing/recreating the invalid worktree path under a dedicated repair helper

## Task Plan

### Task 1: Introduce legacy DAG selection policy helpers

**Files:**
- Create: `pkg/server/legacy_dag_policy.go`
- Test: `pkg/server/legacy_dag_policy_test.go`

- [ ] **Step 1: Write the failing policy tests**

```go
package server

import (
    "testing"
    "time"

    "github.com/stretchr/testify/require"
    "github.com/toustifer/agentflow/pkg/engine"
)

func TestPickResumeDAGSkipsLegacyInProgressDAG(t *testing.T) {
    now := time.Now().UTC()
    dags := []engine.DAG{
        {
            ID: "legacy",
            Title: "Legacy Integration",
            Status: engine.DAGInProgress,
            UpdatedAt: now.Add(-2 * time.Hour),
        },
        {
            ID: "current",
            Title: "Current Release",
            Status: engine.DAGInProgress,
            UpdatedAt: now,
        },
    }
    hints := map[string]dagResumeHint{
        "legacy": {Legacy: true, ResumePriority: resumePriorityDeprioritized},
    }

    picked := pickResumeDAG(dags, hints, "")
    require.NotNil(t, picked)
    require.Equal(t, "current", picked.ID)
}

func TestPickResumeDAGHonorsExplicitTarget(t *testing.T) {
    now := time.Now().UTC()
    dags := []engine.DAG{
        {ID: "legacy", Title: "Legacy Integration", Status: engine.DAGInProgress, UpdatedAt: now.Add(-2 * time.Hour)},
        {ID: "current", Title: "Current Release", Status: engine.DAGInProgress, UpdatedAt: now},
    }
    hints := map[string]dagResumeHint{
        "legacy": {Legacy: true, ResumePriority: resumePriorityDeprioritized},
    }

    picked := pickResumeDAG(dags, hints, "legacy")
    require.NotNil(t, picked)
    require.Equal(t, "legacy", picked.ID)
}
```

- [ ] **Step 2: Run the new policy tests to verify they fail**

Run:
```bash
go test ./pkg/server -run 'TestPickResumeDAG' -v
```

Expected: FAIL with `undefined: dagResumeHint`, `undefined: pickResumeDAG`

- [ ] **Step 3: Add minimal legacy policy types and selection helper**

```go
package server

import (
    "sort"

    "github.com/toustifer/agentflow/pkg/engine"
)

type resumePriority string

const (
    resumePriorityDefault      resumePriority = "default"
    resumePriorityDeprioritized resumePriority = "deprioritized"
    resumePriorityForce        resumePriority = "force"
)

type dagResumeHint struct {
    Legacy         bool
    ResumePriority resumePriority
    SupersededBy   string
}

func pickResumeDAG(dags []engine.DAG, hints map[string]dagResumeHint, targetDAGID string) *engine.DAG {
    if targetDAGID != "" {
        for i := range dags {
            if dags[i].ID == targetDAGID {
                return &dags[i]
            }
        }
        return nil
    }

    candidates := make([]engine.DAG, 0, len(dags))
    for _, dag := range dags {
        if dag.Status == engine.DAGDone || dag.Status == engine.DAGCancelled {
            continue
        }
        hint := hints[dag.ID]
        if hint.Legacy || hint.ResumePriority == resumePriorityDeprioritized {
            continue
        }
        candidates = append(candidates, dag)
    }
    if len(candidates) == 0 {
        return nil
    }
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].UpdatedAt.After(candidates[j].UpdatedAt)
    })
    return &candidates[0]
}
```

- [ ] **Step 4: Run the policy tests to verify they pass**

Run:
```bash
go test ./pkg/server -run 'TestPickResumeDAG' -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/server/legacy_dag_policy.go pkg/server/legacy_dag_policy_test.go
git commit -m "feat: add legacy dag resume policy"
```

### Task 2: Feed DAG resume hints from namespace/project state

**Files:**
- Modify: `pkg/server/handlers_nextsteps.go`
- Modify: `pkg/engine/dag.go`
- Test: `pkg/server/legacy_dag_policy_test.go`

- [ ] **Step 1: Extend the failing tests to cover metadata-derived hints**

```go
func TestDagResumeHintFromMetadata(t *testing.T) {
    hint := dagResumeHintFromMetadata(map[string]string{
        "dag.legacy": "true",
        "dag.resume_priority": "deprioritized",
        "dag.superseded_by": "new-dag",
    })

    require.True(t, hint.Legacy)
    require.Equal(t, resumePriorityDeprioritized, hint.ResumePriority)
    require.Equal(t, "new-dag", hint.SupersededBy)
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run:
```bash
go test ./pkg/server -run 'Test(DagResumeHintFromMetadata|PickResumeDAG)' -v
```

Expected: FAIL with `undefined: dagResumeHintFromMetadata`

- [ ] **Step 3: Add metadata-aware resume hint parsing**

```go
func dagResumeHintFromMetadata(metadata map[string]string) dagResumeHint {
    if metadata == nil {
        return dagResumeHint{}
    }
    hint := dagResumeHint{}
    if metadata["dag.legacy"] == "true" {
        hint.Legacy = true
    }
    switch metadata["dag.resume_priority"] {
    case string(resumePriorityDeprioritized):
        hint.ResumePriority = resumePriorityDeprioritized
    case string(resumePriorityForce):
        hint.ResumePriority = resumePriorityForce
    default:
        hint.ResumePriority = resumePriorityDefault
    }
    hint.SupersededBy = metadata["dag.superseded_by"]
    return hint
}
```

- [ ] **Step 4: Add DAG metadata storage if absent in the current model**

```go
type DAG struct {
    ID                  string            `json:"id"`
    NamespaceID         string            `json:"namespace_id"`
    Title               string            `json:"title"`
    ExecutionBranch     string            `json:"execution_branch"`
    BaseBranch          string            `json:"base_branch,omitempty"`
    Metadata            map[string]string `json:"metadata,omitempty"`
    WorktreePath        string            `json:"worktree_path,omitempty"`
    // ...existing fields...
}
```

- [ ] **Step 5: Run the focused tests to verify they pass**

Run:
```bash
go test ./pkg/server -run 'Test(DagResumeHintFromMetadata|PickResumeDAG)' -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/server/legacy_dag_policy.go pkg/server/legacy_dag_policy_test.go pkg/server/handlers_nextsteps.go pkg/engine/dag.go
git commit -m "feat: parse legacy dag resume hints"
```

### Task 3: Change project_next_steps to choose the correct DAG by default

**Files:**
- Modify: `pkg/server/handlers_nextsteps.go`
- Test: `pkg/server/mcp_test.go`

- [ ] **Step 1: Add a failing regression test for legacy DAG hijacking**

```go
func TestProjectNextStepsSkipsLegacyDAGByDefault(t *testing.T) {
    t.Parallel()

    srv := newTestServer(t)
    // create namespace, workers, two DAGs, tasks
    // mark legacy DAG metadata as dag.legacy=true and dag.resume_priority=deprioritized
    // mark current DAG as in_progress with ready task

    result, err := srv.Handle(context.Background(), "project_next_steps", map[string]any{
        "namespace_id": "ns-1",
    })
    require.NoError(t, err)

    dag := result["dag"].(map[string]any)
    require.Equal(t, "current-dag", dag["id"])
}
```

- [ ] **Step 2: Run the regression test to verify it fails**

Run:
```bash
go test ./pkg/server -run TestProjectNextStepsSkipsLegacyDAGByDefault -v
```

Expected: FAIL because the old logic still picks the first `Status != ""` DAG

- [ ] **Step 3: Replace the naive active DAG selection**

```go
var targetDAGID string
if inputTarget, ok := input["dag_id"].(string); ok {
    targetDAGID = inputTarget
}

hints := make(map[string]dagResumeHint, len(dags))
for i := range dags {
    hints[dags[i].ID] = dagResumeHintFromMetadata(dags[i].Metadata)
}

activeDAG := pickResumeDAG(dags, hints, targetDAGID)
if activeDAG == nil && len(dags) > 0 {
    activeDAG = &dags[0]
}
```

- [ ] **Step 4: Add de-prioritization context to the response**

```go
if activeDAG != nil {
    result["dag"] = dagToSummaryMap(activeDAG)
    result["resume_targeted"] = targetDAGID != ""
    result["legacy_dags_skipped"] = summarizeSkippedLegacyDAGs(dags, hints, activeDAG.ID)
}
```

- [ ] **Step 5: Run the regression test and nearby phase tests**

Run:
```bash
go test ./pkg/server -run 'TestProjectNextStepsSkipsLegacyDAGByDefault|TestProjectInspectAggregatesBucketsAndTaskRuntime' -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/server/handlers_nextsteps.go pkg/server/mcp_test.go
git commit -m "fix: skip legacy dags in default resume"
```

### Task 4: Add `/agentflow resume dag <dag_id>` targeted resume support

**Files:**
- Modify: `skills/agentflow/SKILL.md`
- Modify: `skills/agentflow/flows/resume.md`
- Modify: `C:/Users/15775/.claude/skills/agentflow/SKILL.md`
- Modify: `C:/Users/15775/.claude/skills/agentflow/flows/resume.md`

- [ ] **Step 1: Add explicit targeted resume syntax to the skill docs**

```text
/agentflow resume              -> 恢复默认项目主链
/agentflow resume dag <dag_id> -> 恢复项目现场，但强制聚焦指定 DAG
```

- [ ] **Step 2: Add the parsing rule to `SKILL.md`**

```text
5. 如果 args 以 `resume` 开头
   - 读取 `flows/resume.md`
   - 如果参数形如 `resume dag <dag_id>`，则把该 `dag_id` 当作 targeted resume 输入
   - 先恢复项目 snapshot 和 DAG 列表
   - 再按 resume flow 决定继续哪条线
```

- [ ] **Step 3: Add targeted resume behavior to `flows/resume.md`**

```text
### targeted resume

当用户执行 `/agentflow resume dag <dag_id>`：
- 仍先恢复项目级 snapshot
- 但后续所有 `project_next_steps` / `project_inspect` / `dag_get` 应传入 `dag_id=<dag_id>`
- 不使用默认 recommended_dag_to_continue 覆盖该目标
- 若该 DAG 是 legacy DAG，必须明确提示：这是显式恢复历史线，而不是默认主线
```

- [ ] **Step 4: Mirror the same text into the live skill copy**

```text
skills/agentflow/SKILL.md
skills/agentflow/flows/resume.md
C:/Users/15775/.claude/skills/agentflow/SKILL.md
C:/Users/15775/.claude/skills/agentflow/flows/resume.md
```

- [ ] **Step 5: Commit**

```bash
git add skills/agentflow/SKILL.md skills/agentflow/flows/resume.md C:/Users/15775/.claude/skills/agentflow/SKILL.md C:/Users/15775/.claude/skills/agentflow/flows/resume.md
git commit -m "docs: add targeted dag resume flow"
```

### Task 5: Add structured invalid-worktree repair helpers

**Files:**
- Create: `pkg/server/worktree_repair.go`
- Test: `pkg/server/worktree_repair_test.go`
- Modify: `pkg/server/git_worktree.go`

- [ ] **Step 1: Write the failing repair tests**

```go
func TestClassifyInvalidExistingWorktreePath(t *testing.T) {
    dir := t.TempDir()
    err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("junk"), 0o644)
    require.NoError(t, err)

    state := classifyWorktreePath(dir)
    require.Equal(t, worktreePathInvalidExisting, state)
}

func TestRepairInvalidWorktreePathRequiresExplicitOptIn(t *testing.T) {
    dir := t.TempDir()
    require.NoError(t, os.WriteFile(filepath.Join(dir, "junk.txt"), []byte("x"), 0o644))

    err := repairInvalidWorktreePath(dir, false)
    require.Error(t, err)
    require.Contains(t, err.Error(), "repair not allowed")
}
```

- [ ] **Step 2: Run the repair tests to verify they fail**

Run:
```bash
go test ./pkg/server -run 'Test(ClassifyInvalidExistingWorktreePath|RepairInvalidWorktreePathRequiresExplicitOptIn)' -v
```

Expected: FAIL with undefined helper names

- [ ] **Step 3: Add minimal repair helpers**

```go
package server

import (
    "fmt"
    "os"
)

type worktreePathState string

const (
    worktreePathMissing         worktreePathState = "missing"
    worktreePathValid           worktreePathState = "valid"
    worktreePathInvalidExisting worktreePathState = "invalid_existing"
)

func classifyWorktreePath(path string) worktreePathState {
    if _, err := os.Stat(path); err != nil {
        return worktreePathMissing
    }
    return worktreePathInvalidExisting
}

func repairInvalidWorktreePath(path string, allowRepair bool) error {
    if !allowRepair {
        return fmt.Errorf("repair not allowed for invalid worktree path %q", path)
    }
    return os.RemoveAll(path)
}
```

- [ ] **Step 4: Run the repair tests to verify they pass**

Run:
```bash
go test ./pkg/server -run 'Test(ClassifyInvalidExistingWorktreePath|RepairInvalidWorktreePathRequiresExplicitOptIn)' -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/server/worktree_repair.go pkg/server/worktree_repair_test.go pkg/server/git_worktree.go
git commit -m "feat: add invalid worktree repair helpers"
```

### Task 6: Gate worktree repair behind targeted legacy resume intent

**Files:**
- Modify: `pkg/server/git_worktree.go`
- Modify: `pkg/server/mcp.go`
- Test: `pkg/server/mcp_test.go`

- [ ] **Step 1: Add a failing integration test for targeted repair**

```go
func TestTaskPrepareStartCanRepairInvalidLegacyWorktreeWhenTargeted(t *testing.T) {
    t.Parallel()

    srv := newTestServer(t)
    createDagTaskForStart(t, srv, "T-legacy-repair")

    // create junk directory at expected worktree path
    // mark DAG metadata dag.legacy=true

    _, err := srv.Handle(context.Background(), "task_prepare_start", map[string]any{
        "namespace_id":   "ns-1",
        "task_id":        "T-legacy-repair",
        "allow_repair":   true,
        "resume_targeted": true,
    })
    require.NoError(t, err)
}
```

- [ ] **Step 2: Run the repair integration test to verify it fails**

Run:
```bash
go test ./pkg/server -run TestTaskPrepareStartCanRepairInvalidLegacyWorktreeWhenTargeted -v
```

Expected: FAIL because the current path still hard-fails on invalid existing directory

- [ ] **Step 3: Extend `prepareTaskGitRuntime(...)` / `ensureTaskWorktree(...)` with explicit repair gating**

```go
func ensureTaskWorktree(ctx context.Context, repoPath, worktreePath, branch, baseBranch string, allowRepair bool) error {
    if runtime, err := inspectTaskGitRuntime(ctx, repoPath, worktreePath, branch, baseBranch); err == nil {
        if runtime.Branch == branch {
            return nil
        }
        return fmt.Errorf("现有 worktree %q 绑定到分支 %q，不是期望的 %q", worktreePath, runtime.Branch, branch)
    }
    if _, err := os.Stat(worktreePath); err == nil {
        if err := repairInvalidWorktreePath(worktreePath, allowRepair); err != nil {
            return fmt.Errorf("路径 %q 已存在但不是有效 worktree，且未允许 repair: %w", worktreePath, err)
        }
    }
    // existing add-worktree logic continues here
}
```

- [ ] **Step 4: Thread the explicit repair flag through `task_prepare_start`**

```go
allowRepair, _ := input["allow_repair"].(bool)
resumeTargeted, _ := input["resume_targeted"].(bool)
if allowRepair && !resumeTargeted {
    return taskResult{}, fmt.Errorf("allow_repair requires resume_targeted=true")
}
```

- [ ] **Step 5: Run the targeted repair tests and existing worktree regression tests**

Run:
```bash
go test ./pkg/server -run 'Test(TaskPrepareStartCanRepairInvalidLegacyWorktreeWhenTargeted|SameDAGTasksReuseSingleWorktreePath|SequentialTasksTransferDAGLeaseHolder)' -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/server/git_worktree.go pkg/server/mcp.go pkg/server/mcp_test.go pkg/server/worktree_repair.go pkg/server/worktree_repair_test.go
git commit -m "fix: allow targeted repair for legacy dag worktrees"
```

### Task 7: Surface legacy status in project_inspect and resume output

**Files:**
- Modify: `pkg/server/handlers_ext.go`
- Modify: `skills/agentflow/hooks/render-inspect.js`
- Test: `pkg/server/mcp_test.go`

- [ ] **Step 1: Add a failing inspect regression test**

```go
func TestProjectInspectShowsLegacyDagFlags(t *testing.T) {
    t.Parallel()

    srv := newTestServer(t)
    // create one legacy DAG and one current DAG

    result, err := srv.Handle(context.Background(), "project_inspect", map[string]any{
        "namespace_id": "ns-1",
        "focus": "project",
    })
    require.NoError(t, err)

    dags := result["dags"].([]any)
    first := dags[0].(map[string]any)
    require.Contains(t, first, "resume_priority")
    require.Contains(t, first, "legacy")
}
```

- [ ] **Step 2: Run the inspect regression test to verify it fails**

Run:
```bash
go test ./pkg/server -run TestProjectInspectShowsLegacyDagFlags -v
```

Expected: FAIL because the fields are absent

- [ ] **Step 3: Add legacy hint fields into the inspect payload**

```go
hint := dagResumeHintFromMetadata(d.DAG.Metadata)
item := map[string]any{
    "dag":             dagToMap(&d.DAG),
    "legacy":          hint.Legacy,
    "resume_priority": string(hint.ResumePriority),
    "superseded_by":   hint.SupersededBy,
    // existing counters...
}
```

- [ ] **Step 4: Show the fields in the render layer without exploding the UI**

```js
const legacySuffix = item.legacy ? " · legacy" : "";
const prioritySuffix = item.resume_priority && item.resume_priority !== "default"
  ? ` · ${item.resume_priority}`
  : "";
lines.push(`DAG ${text(dag.id)} ${text(dag.title)}${legacySuffix}${prioritySuffix}`);
```

- [ ] **Step 5: Run the inspect test and the render smoke tests**

Run:
```bash
go test ./pkg/server -run 'Test(ProjectInspectShowsLegacyDagFlags|ProjectInspectAggregatesBucketsAndTaskRuntime)' -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add pkg/server/handlers_ext.go pkg/server/mcp_test.go skills/agentflow/hooks/render-inspect.js
git commit -m "feat: surface legacy dag status in inspect"
```

### Task 8: End-to-end verification and documentation pass

**Files:**
- Modify: `README.md`
- Modify: `skills/agentflow/flows/resume.md`
- Modify: `C:/Users/15775/.claude/skills/agentflow/flows/resume.md`

- [ ] **Step 1: Add an operator note to README for the two-path model**

```md
### Legacy DAG recovery

- Default `/agentflow resume` skips DAGs marked legacy/deprioritized.
- `/agentflow resume dag <dag_id>` explicitly targets a DAG, including a legacy DAG.
- Invalid worktree repair is only allowed during explicit targeted resume/repair flows.
```

- [ ] **Step 2: Add an operator warning to the resume flow**

```text
如果 targeted DAG 被标记为 legacy：
- 必须显式告诉用户：这是恢复历史线
- 不要把它描述成当前默认主线
- 若需 repair worktree，必须走显式 repair 分支
```

- [ ] **Step 3: Run the focused server test suite for this feature cluster**

Run:
```bash
go test ./pkg/server -run 'Test(PickResumeDAG|DagResumeHintFromMetadata|ProjectNextStepsSkipsLegacyDAGByDefault|TaskPrepareStartCanRepairInvalidLegacyWorktreeWhenTargeted|ProjectInspectShowsLegacyDagFlags|SameDAGTasksReuseSingleWorktreePath|SequentialTasksTransferDAGLeaseHolder)' -v
```

Expected: PASS

- [ ] **Step 4: Run the broader package tests that cover engine + server integration**

Run:
```bash
go test ./pkg/engine ./pkg/server -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add README.md skills/agentflow/flows/resume.md C:/Users/15775/.claude/skills/agentflow/flows/resume.md
git commit -m "docs: document legacy dag resume and repair flow"
```

## Verification Matrix

1. Default resume skips a legacy `in_progress` DAG when a newer active DAG exists.
2. `/agentflow resume dag <dag_id>` targets that DAG even if it is legacy.
3. `leader_tick` reflects the corrected next-task selection because it consumes the fixed `project_next_steps` payload.
4. `task_prepare_start` still fails on invalid existing worktree paths by default.
5. Explicit targeted repair can recreate an invalid legacy worktree path.
6. Existing same-DAG shared-worktree behavior still passes.
7. Inspect output shows which DAGs are legacy/deprioritized so operators understand why default resume skipped them.

## Self-Review

- Spec coverage: covered temporary bypass (`resume dag <dag_id>`), permanent fix (legacy DAG de-prioritization/closure), worktree recovery gating, affected files, state logic, and verification.
- Placeholder scan: no `TODO`/`TBD` placeholders; each task includes file paths, code, commands, and expected outcomes.
- Type consistency: plan consistently uses `dagResumeHint`, `resumePriority`, `allow_repair`, and `resume_targeted`; `DAGCancelled` is reused instead of inventing a second permanent retired status.
