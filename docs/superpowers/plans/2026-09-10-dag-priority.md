# DAG 优先级 (P0-P3) 实现计划 (Implementation Plan)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 DAG 引入原生的 P0 ~ P3 优先级模型，支持在 dag_create、dag_update 和 goal_promote 中指定或自动估量，并在持久化、排序以及候选推荐中全链路打通。

**Architecture:** 在 `pkg/engine/dag.go` 中定义 `DAGPriority`（P0, P1, P2, P3）与权重换算器；在 SQLite `dags` 表中增加 `priority` 字段并实现平滑迁移与持久化；在 `pkg/server` 的 MCP 工具（`dag_create`, `dag_update`, `goal_promote`）中暴露校验与映射；在 `orderResumeCandidates` 与 `project_inspect` 中按优先级降序调度。

**Tech Stack:** Go 1.22+, modernc.org/sqlite, JSON-RPC, testing.

---

### Task 1: DAG Priority Types, Normalization, and Weight Helpers

**Files:**
- Modify: `pkg/engine/dag.go`
- Create: `pkg/engine/dag_priority_test.go`

- [ ] **Step 1: Write failing test for DAGPriority types and normalization**

Add `pkg/engine/dag_priority_test.go`:
```go
package engine

import (
	"testing"
	"github.com/stretchr/testify/require"
)

func TestDAGPriorityNormalizationAndWeights(t *testing.T) {
	p, err := NormalizeDAGPriority("p0")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP0, p)
	require.Equal(t, 100, DAGPriorityWeight(p))

	p, err = NormalizeDAGPriority("P1")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP1, p)
	require.Equal(t, 50, DAGPriorityWeight(p))

	p, err = NormalizeDAGPriority("P2")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP2, p)
	require.Equal(t, 20, DAGPriorityWeight(p))

	p, err = NormalizeDAGPriority("p3")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP3, p)
	require.Equal(t, 0, DAGPriorityWeight(p))

	p, err = NormalizeDAGPriority("")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP2, p)

	_, err = NormalizeDAGPriority("invalid")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine -v -run TestDAGPriorityNormalizationAndWeights`
Expected: FAIL (compilation error, missing NormalizeDAGPriority).

- [ ] **Step 3: Implement DAGPriority and helper functions in `pkg/engine/dag.go`**

```go
type DAGPriority string

const (
	DAGPriorityP0 DAGPriority = "P0"
	DAGPriorityP1 DAGPriority = "P1"
	DAGPriorityP2 DAGPriority = "P2"
	DAGPriorityP3 DAGPriority = "P3"
)

func NormalizeDAGPriority(p string) (DAGPriority, error) {
	switch strings.ToUpper(strings.TrimSpace(p)) {
	case "", "P2":
		return DAGPriorityP2, nil
	case "P0":
		return DAGPriorityP0, nil
	case "P1":
		return DAGPriorityP1, nil
	case "P3":
		return DAGPriorityP3, nil
	default:
		return "", fmt.Errorf("invalid dag priority %q: must be one of P0, P1, P2, P3", p)
	}
}

func DAGPriorityWeight(p DAGPriority) int {
	switch p {
	case DAGPriorityP0:
		return 100
	case DAGPriorityP1:
		return 50
	case DAGPriorityP2:
		return 20
	case DAGPriorityP3:
		return 0
	default:
		return 20
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine -v -run TestDAGPriorityNormalizationAndWeights`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/dag.go pkg/engine/dag_priority_test.go
git commit -m "feat(engine): add DAGPriority types, normalization, and weight helpers"
```

---

### Task 2: SQLite Schema Migration & Persistence for DAG Priority

**Files:**
- Modify: `pkg/engine/store.go`
- Modify: `pkg/engine/store_dag_worker.go`
- Modify: `pkg/engine/dag.go`
- Test: `pkg/engine/dag_priority_test.go`

- [ ] **Step 1: Write failing test for DAG Priority SQLite persistence and reopen**

Add to `pkg/engine/dag_priority_test.go`:
```go
func TestDAGPriorityPersistenceAndReopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dag_priority.db")

	ctx := context.Background()
	eng1, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)

	_, err = eng1.CreateNamespace(ctx, CreateNamespaceRequest{ID: "ns-pri", Name: "Namespace Pri"})
	require.NoError(t, err)

	d1, err := eng1.CreateDAG(ctx, CreateDAGRequest{
		NamespaceID:     "ns-pri",
		ID:              "dag-p0",
		Title:           "Emergency Fix",
		Priority:        "P0",
		ExecutionBranch: "feat/p0",
	})
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP0, d1.Priority)

	require.NoError(t, eng1.Close())

	// Reopen
	eng2, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer eng2.Close()

	d1Reopened, err := eng2.GetDAG(ctx, "ns-pri", "dag-p0")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP0, d1Reopened.Priority)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine -v -run TestDAGPriorityPersistenceAndReopen`
Expected: FAIL.

- [ ] **Step 3: Implement SQLite migration and column queries in `pkg/engine/store.go` and `store_dag_worker.go`**

- Add `Priority DAGPriority` to `DAG` struct in `pkg/engine/dag.go`.
- Add `Priority string` to `CreateDAGRequest` and `Priority *string` to `UpdateDAGRequest`.
- Update `schemaSQL` in `pkg/engine/store.go` to include `priority TEXT NOT NULL DEFAULT 'P2'`.
- Update `migrateDAGsTable` to include `"priority TEXT NOT NULL DEFAULT 'P2'"`.
- Update `insertDAG`, `updateDAG`, and `loadDAGs` in `pkg/engine/store_dag_worker.go` to include the `priority` column.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine -v -run TestDAGPriorityPersistenceAndReopen`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/store.go pkg/engine/store_dag_worker.go pkg/engine/dag.go pkg/engine/dag_priority_test.go
git commit -m "feat(engine): add SQLite migration and persistence for DAG priority"
```

---

### Task 3: Engine DAG CRUD & ListDAGs Priority Ordering

**Files:**
- Modify: `pkg/engine/dag.go`
- Modify: `pkg/engine/goal.go`
- Test: `pkg/engine/dag_priority_test.go`

- [ ] **Step 1: Write failing test for DAG Update and ListDAGs priority ordering**

Add to `pkg/engine/dag_priority_test.go`:
```go
func TestListDAGsPriorityOrdering(t *testing.T) {
	eng, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	ctx := context.Background()

	_, err = eng.CreateNamespace(ctx, CreateNamespaceRequest{ID: "ns-order", Name: "Order NS"})
	require.NoError(t, err)

	_, err = eng.CreateDAG(ctx, CreateDAGRequest{
		NamespaceID:     "ns-order",
		ID:              "dag-p2",
		Title:           "P2 Task",
		Priority:        "P2",
		ExecutionBranch: "feat/p2",
	})
	require.NoError(t, err)

	_, err = eng.CreateDAG(ctx, CreateDAGRequest{
		NamespaceID:     "ns-order",
		ID:              "dag-p0",
		Title:           "P0 Task",
		Priority:        "P0",
		ExecutionBranch: "feat/p0",
	})
	require.NoError(t, err)

	_, err = eng.CreateDAG(ctx, CreateDAGRequest{
		NamespaceID:     "ns-order",
		ID:              "dag-p1",
		Title:           "P1 Task",
		Priority:        "P1",
		ExecutionBranch: "feat/p1",
	})
	require.NoError(t, err)

	dags, err := eng.ListDAGs(ctx, "ns-order")
	require.NoError(t, err)
	require.Len(t, dags, 3)
	require.Equal(t, "dag-p0", dags[0].ID)
	require.Equal(t, "dag-p1", dags[1].ID)
	require.Equal(t, "dag-p2", dags[2].ID)

	// Update dag-p2 to P0
	p0 := "P0"
	upd, err := eng.UpdateDAG(ctx, "ns-order", "dag-p2", UpdateDAGRequest{Priority: &p0})
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP0, upd.Priority)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine -v -run TestListDAGsPriorityOrdering`
Expected: FAIL.

- [ ] **Step 3: Update `CreateDAG`, `UpdateDAG`, `ListDAGs`, and `PromoteGoal`**

- In `CreateDAG`: normalize `req.Priority` (default `"P2"`).
- In `UpdateDAG`: if `req.Priority != nil`, normalize and update.
- In `ListDAGs`: sort by `DAGPriorityWeight(dags[i].Priority) > DAGPriorityWeight(dags[j].Priority)`.
- In `PromoteGoal`: support `req.Priority`, defaulting to mapping goal priority or `"P2"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine -v -run TestListDAGsPriorityOrdering`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/dag.go pkg/engine/goal.go pkg/engine/dag_priority_test.go
git commit -m "feat(engine): support DAG priority updates and ListDAGs priority ordering"
```

---

### Task 4: MCP Tools Integration (`dag_create`, `dag_update`, `goal_promote`)

**Files:**
- Modify: `pkg/server/mcp.go`
- Modify: `pkg/server/handlers_goal.go`
- Create: `pkg/server/dag_priority_mcp_test.go`

- [ ] **Step 1: Write failing test for MCP dag_create, dag_update, goal_promote with priority**

Create `pkg/server/dag_priority_mcp_test.go`:
```go
package server

import (
	"context"
	"testing"
	"github.com/stretchr/testify/require"
)

func TestDAGPriorityMCPIntegration(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	// 1. dag_create with priority
	createRes, err := srv.Handle(ctx, "dag_create", map[string]any{
		"namespace_id":     "ns-1",
		"dag_id":           "dag-crit",
		"title":            "Critical Bug",
		"priority":         "p0",
		"execution_branch": "feat/crit",
	})
	require.NoError(t, err)
	dagMap := createRes["dag"].(map[string]any)
	require.Equal(t, "P0", dagMap["priority"])

	// 2. dag_update priority
	updateRes, err := srv.Handle(ctx, "dag_update", map[string]any{
		"namespace_id": "ns-1",
		"dag_id":       "dag-crit",
		"priority":     "P1",
	})
	require.NoError(t, err)
	dagMap = updateRes["dag"].(map[string]any)
	require.Equal(t, "P1", dagMap["priority"])

	// 3. goal_promote with priority
	_, err = srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "ns-1",
		"goal_id":      "G-urgent",
		"title":        "Urgent Goal",
	})
	require.NoError(t, err)

	promRes, err := srv.Handle(ctx, "goal_promote", map[string]any{
		"namespace_id": "ns-1",
		"goal_id":      "G-urgent",
		"priority":     "P0",
	})
	require.NoError(t, err)
	promDAG := promRes["dag"].(map[string]any)
	require.Equal(t, "P0", promDAG["priority"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/server -v -run TestDAGPriorityMCPIntegration`
Expected: FAIL.

- [ ] **Step 3: Implement MCP parameter handling in `pkg/server/mcp.go` and `handlers_goal.go`**

- In `pkg/server/mcp.go`:
  - add `"priority"` to `dag_create` and `dag_update` tool schemas.
  - In `handleDAGCreate`: read `priority` from input.
  - In `handleDAGUpdate`: read `priority` from input.
  - In `dagToMap`: output `"priority": string(d.Priority)`.
- In `pkg/server/handlers_goal.go`:
  - In `handleGoalPromote`: pass `priority` to `engine.PromoteGoalRequest`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/server -v -run TestDAGPriorityMCPIntegration`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/server/mcp.go pkg/server/handlers_goal.go pkg/server/dag_priority_mcp_test.go
git commit -m "feat(server): expose priority in dag_create, dag_update, and goal_promote MCP tools"
```

---

### Task 5: Scheduling & Inspect Integration (`orderResumeCandidates`, `project_inspect`)

**Files:**
- Modify: `pkg/server/legacy_dag_policy.go`
- Modify: `pkg/server/handlers_ext.go`
- Test: `pkg/server/dag_priority_mcp_test.go`

- [ ] **Step 1: Write failing test for scheduling & inspect priority ordering**

Add to `pkg/server/dag_priority_mcp_test.go`:
```go
func TestDAGPrioritySchedulingAndInspect(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	_, err := srv.Handle(ctx, "dag_create", map[string]any{
		"namespace_id":     "ns-1",
		"dag_id":           "dag-low",
		"title":            "Low Priority DAG",
		"priority":         "P3",
		"execution_branch": "feat/low",
	})
	require.NoError(t, err)

	_, err = srv.Handle(ctx, "dag_create", map[string]any{
		"namespace_id":     "ns-1",
		"dag_id":           "dag-high",
		"title":            "High Priority DAG",
		"priority":         "P0",
		"execution_branch": "feat/high",
	})
	require.NoError(t, err)

	// project_inspect
	insp, err := srv.Handle(ctx, "project_inspect", map[string]any{"namespace_id": "ns-1"})
	require.NoError(t, err)
	dags := insp["dags"].([]any)
	require.GreaterOrEqual(t, len(dags), 2)
	firstDAG := dags[0].(map[string]any)["dag"].(map[string]any)
	require.Equal(t, "dag-high", firstDAG["id"])
	require.Equal(t, "P0", firstDAG["priority"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/server -v -run TestDAGPrioritySchedulingAndInspect`
Expected: FAIL.

- [ ] **Step 3: Update `orderResumeCandidates` and `project_inspect`**

- In `pkg/server/legacy_dag_policy.go`:
  - `orderResumeCandidates`: check `engine.DAGPriorityWeight(left.Priority) != engine.DAGPriorityWeight(right.Priority)` first.
  - `dagCandidateSummary`: include `"priority": string(d.Priority)`.
- In `pkg/server/handlers_ext.go`:
  - In `handleProjectInspect`: sort `dagItems` by priority weight descending first.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/server -v -run TestDAGPrioritySchedulingAndInspect`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/server/legacy_dag_policy.go pkg/server/handlers_ext.go pkg/server/dag_priority_mcp_test.go
git commit -m "feat(server): prioritize higher priority DAGs in inspect and candidate scheduling"
```

---

### Task 6: Full Suite Verification and Remote Push

- [ ] **Step 1: Run full engine and server test suites**
Run:
```bash
go test ./pkg/engine -v
go test ./pkg/server -v -run "TestGoal|TestDAG|TestProject"
```
- [ ] **Step 2: Git push origin deepseek/dsh-support**
