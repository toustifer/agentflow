# Backlog / Goal 储备池原生支持 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 引入原生的 Goal / Backlog 实体与储备池管理能力，支持挂起、检索、生命周期流转与一键晋升（Promote to DAG），并无缝联动 project_inspect 与 project_next_steps。

**Architecture:** 在 SQLite 中新增 `goals` 表并建立状态索引；在 `pkg/engine` 中实现 Goal 数据模型、线程安全缓存与 CRUD / ID 自动生成 / Promote 事务物化；在 `pkg/server` 中暴露 5 个 MCP 工具（`goal_create`, `goal_list`, `goal_get`, `goal_update`, `goal_promote`）；在 `project_inspect` 中增强 `backlog` 概览与计数；在 `project_next_steps` 中实现结案与空闲时的智能推荐流转。

**Tech Stack:** Go 1.22+, SQLite (modernc.org/sqlite), MCP JSON-RPC, testing.

---

### Task 1: SQLite Schema Migration & Goal Entity Types

**Files:**
- Create: `pkg/engine/goal.go`
- Modify: `pkg/engine/store.go`
- Modify: `pkg/engine/engine.go`

- [ ] **Step 1: Write the failing test for Goal types and basic initialization**

Add `pkg/engine/goal_test.go` with test verifying that Engine initializes with empty goals map and schema creates goals table:

```go
package engine

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEngineInitializesGoalsMapAndSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_goals_init.db")

	eng, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	if eng.goals == nil {
		t.Fatalf("eng.goals should be initialized")
	}

	// Verify goals table exists in sqlite
	var count int
	err = eng.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('goals') WHERE name = 'id'`).Scan(&count)
	if err != nil || count == 0 {
		t.Fatalf("goals table was not properly initialized: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine -v -run TestEngineInitializesGoalsMapAndSchema`
Expected: FAIL with compilation error (eng.goals undefined or missing schema).

- [ ] **Step 3: Implement minimal schema, types, and engine initialization**

1. Create `pkg/engine/goal.go`:
```go
package engine

import (
	"errors"
	"time"
)

var (
	ErrGoalNotFound        = errors.New("goal not found")
	ErrDuplicateGoal       = errors.New("goal already exists")
	ErrGoalAlreadyPromoted = errors.New("goal has already been promoted")
	ErrInvalidGoalStatus   = errors.New("invalid goal status")
)

type GoalStatus string

const (
	GoalPending  GoalStatus = "pending"
	GoalDeferred GoalStatus = "deferred"
	GoalPromoted GoalStatus = "promoted"
	GoalDropped  GoalStatus = "dropped"
)

type Goal struct {
	ID          string            `json:"id"`
	NamespaceID string            `json:"namespace_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      GoalStatus        `json:"status"`
	Priority    int               `json:"priority"`
	Tags        []string          `json:"tags"`
	Context     string            `json:"context"`
	DAGID       string            `json:"dag_id"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}
```

2. In `pkg/engine/store.go`, add `goals` table to `schemaSQL`:
```sql
CREATE TABLE IF NOT EXISTS goals (
	id           TEXT NOT NULL,
	namespace_id TEXT NOT NULL,
	title        TEXT NOT NULL,
	description  TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL DEFAULT 'pending',
	priority     INTEGER NOT NULL DEFAULT 0,
	tags         TEXT NOT NULL DEFAULT '[]',
	context      TEXT NOT NULL DEFAULT '',
	dag_id       TEXT NOT NULL DEFAULT '',
	metadata     TEXT NOT NULL DEFAULT '{}',
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL,
	PRIMARY KEY (namespace_id, id),
	FOREIGN KEY (namespace_id) REFERENCES namespaces(id)
);
CREATE INDEX IF NOT EXISTS idx_goals_ns_status ON goals(namespace_id, status);
```
And add `"goals"` to `deleteAllForNamespace` in `pkg/engine/store.go`.

3. In `pkg/engine/engine.go`:
Add `goals map[string]map[string]*Goal` to `struct Engine`.
Initialize `goals: make(map[string]map[string]*Goal)` in `NewEngine`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine -v -run TestEngineInitializesGoalsMapAndSchema`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/goal.go pkg/engine/store.go pkg/engine/engine.go pkg/engine/goal_test.go
git commit -m "feat(engine): add Goal entity types, schema, and engine initialization"
```

---

### Task 2: Goal Engine CRUD and Auto Short ID Generation

**Files:**
- Modify: `pkg/engine/goal.go`
- Test: `pkg/engine/goal_test.go`

- [ ] **Step 1: Write the failing tests for Goal CRUD & Auto ID**

Add to `pkg/engine/goal_test.go`:
```go
func TestGoalCRUDAndAutoID(t *testing.T) {
	eng, err := NewEngine(NewEngineConfig{})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	ctx := context.Background()
	_, err = eng.CreateNamespace(ctx, "ns-1", "Namespace 1")
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// 1. Auto-generate G-1
	g1, err := eng.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-1",
		Title:       "Auto Goal 1",
		Priority:    10,
		Tags:        []string{"feat"},
	})
	if err != nil {
		t.Fatalf("CreateGoal failed: %v", err)
	}
	if g1.ID != "G-1" {
		t.Fatalf("expected ID G-1, got %s", g1.ID)
	}
	if g1.Status != GoalPending {
		t.Fatalf("expected default status pending, got %s", g1.Status)
	}

	// 2. Auto-generate G-2
	g2, err := eng.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-1",
		Title:       "Auto Goal 2",
		Priority:    20,
	})
	if err != nil {
		t.Fatalf("CreateGoal 2 failed: %v", err)
	}
	if g2.ID != "G-2" {
		t.Fatalf("expected ID G-2, got %s", g2.ID)
	}

	// 3. Custom ID support
	gCustom, err := eng.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-1",
		ID:          "CUSTOM-99",
		Title:       "Custom Goal",
	})
	if err != nil {
		t.Fatalf("CreateGoal custom failed: %v", err)
	}
	if gCustom.ID != "CUSTOM-99" {
		t.Fatalf("expected ID CUSTOM-99, got %s", gCustom.ID)
	}

	// 4. Next auto-generate G-3 (skips custom)
	g3, err := eng.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-1",
		Title:       "Auto Goal 3",
	})
	if err != nil {
		t.Fatalf("CreateGoal 3 failed: %v", err)
	}
	if g3.ID != "G-3" {
		t.Fatalf("expected ID G-3, got %s", g3.ID)
	}

	// 5. GetGoal
	fetched, err := eng.GetGoal(ctx, "ns-1", "G-1")
	if err != nil {
		t.Fatalf("GetGoal failed: %v", err)
	}
	if fetched.Title != "Auto Goal 1" {
		t.Fatalf("unexpected fetched title: %s", fetched.Title)
	}

	// 6. UpdateGoal (defer it)
	updated, err := eng.UpdateGoal(ctx, UpdateGoalRequest{
		NamespaceID: "ns-1",
		ID:          "G-1",
		Status:      GoalDeferred,
		Priority:    ptrInt(15),
	})
	if err != nil {
		t.Fatalf("UpdateGoal failed: %v", err)
	}
	if updated.Status != GoalDeferred || updated.Priority != 15 {
		t.Fatalf("unexpected updated goal: %+v", updated)
	}

	// 7. ListGoals with default filtering (pending + deferred, sorted by priority desc)
	list, err := eng.ListGoals(ctx, GoalFilter{NamespaceID: "ns-1"})
	if err != nil {
		t.Fatalf("ListGoals failed: %v", err)
	}
	if len(list) != 4 { // G-2 (pri 20), G-1 (pri 15), G-3 (pri 0), CUSTOM-99 (pri 0)
		t.Fatalf("expected 4 active goals, got %d", len(list))
	}
	if list[0].ID != "G-2" || list[1].ID != "G-1" {
		t.Fatalf("expected priority ordering G-2, G-1, got %s, %s", list[0].ID, list[1].ID)
	}
}

func ptrInt(v int) *int { return &v }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine -v -run TestGoalCRUDAndAutoID`
Expected: FAIL (CreateGoal, GetGoal, UpdateGoal, ListGoals undefined).

- [ ] **Step 3: Implement Goal CRUD, Auto-ID generator, and filter in `pkg/engine/goal.go`**

Implement in `pkg/engine/goal.go`:
- `CreateGoalRequest`, `UpdateGoalRequest`, `GoalFilter`
- Regex-based helper to find next `G-<N>` in `eng.goals[nsID]`
- `CreateGoal(ctx context.Context, req CreateGoalRequest) (*Goal, error)`
- `GetGoal(ctx context.Context, nsID, id string) (*Goal, error)`
- `UpdateGoal(ctx context.Context, req UpdateGoalRequest) (*Goal, error)`
- `ListGoals(ctx context.Context, filter GoalFilter) ([]Goal, error)`
- Clone helper `cloneGoal(g *Goal) *Goal`

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine -v -run TestGoalCRUDAndAutoID`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/goal.go pkg/engine/goal_test.go
git commit -m "feat(engine): implement Goal CRUD and auto short ID generation"
```

---

### Task 3: Goal Promotion & DAG Materialization

**Files:**
- Modify: `pkg/engine/goal.go`
- Test: `pkg/engine/goal_test.go`

- [ ] **Step 1: Write the failing tests for PromoteGoal**

Add to `pkg/engine/goal_test.go`:
```go
func TestGoalPromotion(t *testing.T) {
	eng, err := NewEngine(NewEngineConfig{})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	ctx := context.Background()
	_, err = eng.CreateNamespace(ctx, "ns-promote", "Namespace Promote")
	if err != nil {
		t.Fatalf("create ns failed: %v", err)
	}

	g, err := eng.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-promote",
		Title:       "Feature Auth Token",
		Description: "Refactor auth tokens",
		Priority:    10,
	})
	if err != nil {
		t.Fatalf("create goal failed: %v", err)
	}

	// Promote goal
	res, err := eng.PromoteGoal(ctx, PromoteGoalRequest{
		NamespaceID: "ns-promote",
		GoalID:      g.ID,
	})
	if err != nil {
		t.Fatalf("PromoteGoal failed: %v", err)
	}

	if res.Goal.Status != GoalPromoted {
		t.Fatalf("expected goal status promoted, got %s", res.Goal.Status)
	}
	if res.DAG == nil {
		t.Fatalf("expected created DAG to be non-nil")
	}
	if res.DAG.ID != "dag-"+g.ID {
		t.Fatalf("expected DAG ID dag-%s, got %s", g.ID, res.DAG.ID)
	}
	if res.DAG.Title != g.Title {
		t.Fatalf("expected DAG title %s, got %s", g.Title, res.DAG.Title)
	}
	if res.DAG.Metadata["source_goal_id"] != g.ID {
		t.Fatalf("expected DAG metadata source_goal_id = %s, got %s", g.ID, res.DAG.Metadata["source_goal_id"])
	}

	// Verify repeat promotion fails
	_, err = eng.PromoteGoal(ctx, PromoteGoalRequest{
		NamespaceID: "ns-promote",
		GoalID:      g.ID,
	})
	if !errors.Is(err, ErrGoalAlreadyPromoted) {
		t.Fatalf("expected ErrGoalAlreadyPromoted, got %v", err)
	}

	// Verify updating status of promoted goal fails
	_, err = eng.UpdateGoal(ctx, UpdateGoalRequest{
		NamespaceID: "ns-promote",
		ID:          g.ID,
		Status:      GoalPending,
	})
	if err == nil {
		t.Fatalf("expected error updating status of promoted goal, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine -v -run TestGoalPromotion`
Expected: FAIL (`PromoteGoal` undefined).

- [ ] **Step 3: Implement PromoteGoal in `pkg/engine/goal.go`**

Implement `PromoteGoalRequest`, `PromoteGoalResult`, and `PromoteGoal`:
- Validate goal exists and is in `GoalPending` or `GoalDeferred` status.
- Derive `dagID = req.DAGID` (default: `dag-<goal.ID>`), `title = req.DAGTitle` (default: `goal.Title`), `executionBranch = req.ExecutionBranch` (default: `feature/<dagID>`).
- Check if DAG already exists; if `dagID` was auto-derived and collides, append sequence suffix.
- Invoke DAG creation logic, setting `Metadata["source_goal_id"] = goal.ID`.
- Update `goal.Status = GoalPromoted`, `goal.DAGID = dag.ID`, `goal.UpdatedAt = time.Now()`.
- Return `PromoteGoalResult{ Goal: *cloneGoal(goal), DAG: *cloneDAG(dag) }`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine -v -run TestGoalPromotion`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/goal.go pkg/engine/goal_test.go
git commit -m "feat(engine): implement PromoteGoal with DAG creation and source binding"
```

---

### Task 4: SQLite Persistence & Engine Recovery for Goals

**Files:**
- Create: `pkg/engine/store_goal.go`
- Modify: `pkg/engine/engine.go`
- Modify: `pkg/engine/goal.go`
- Test: `pkg/engine/goal_test.go`

- [ ] **Step 1: Write the failing test for SQLite persistence & reopen**

Add to `pkg/engine/goal_test.go`:
```go
func TestGoalPersistenceAndReopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "goals_persist.db")

	eng1, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed to create eng1: %v", err)
	}
	ctx := context.Background()
	_, err = eng1.CreateNamespace(ctx, "ns-p", "Namespace P")
	if err != nil {
		t.Fatalf("create ns failed: %v", err)
	}

	g1, err := eng1.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-p",
		Title:       "Persisted Goal 1",
		Priority:    5,
		Tags:        []string{"infra"},
		Context:     "Issue #123",
	})
	if err != nil {
		t.Fatalf("create goal failed: %v", err)
	}

	_, err = eng1.PromoteGoal(ctx, PromoteGoalRequest{
		NamespaceID: "ns-p",
		GoalID:      g1.ID,
	})
	if err != nil {
		t.Fatalf("promote goal failed: %v", err)
	}

	g2, err := eng1.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-p",
		Title:       "Persisted Goal 2",
		Priority:    10,
	})
	if err != nil {
		t.Fatalf("create goal 2 failed: %v", err)
	}

	if err := eng1.Close(); err != nil {
		t.Fatalf("failed to close eng1: %v", err)
	}

	// Reopen engine from dbPath
	eng2, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed to reopen eng2: %v", err)
	}
	defer eng2.Close()

	pG1, err := eng2.GetGoal(ctx, "ns-p", g1.ID)
	if err != nil {
		t.Fatalf("failed to get pG1: %v", err)
	}
	if pG1.Status != GoalPromoted || pG1.DAGID != "dag-"+g1.ID {
		t.Fatalf("unexpected pG1 after reopen: %+v", pG1)
	}

	pG2, err := eng2.GetGoal(ctx, "ns-p", g2.ID)
	if err != nil {
		t.Fatalf("failed to get pG2: %v", err)
	}
	if pG2.Status != GoalPending || pG2.Priority != 10 {
		t.Fatalf("unexpected pG2 after reopen: %+v", pG2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/engine -v -run TestGoalPersistenceAndReopen`
Expected: FAIL (store helper functions not implemented, goals not restored on reopen).

- [ ] **Step 3: Implement SQLite helpers in `pkg/engine/store_goal.go` and hook into `engine.go`**

1. Create `pkg/engine/store_goal.go`:
```go
package engine

import (
	"database/sql"
	"time"
)

func insertGoal(db *sql.DB, g *Goal) error {
	tags := mustMarshalJSON(g.Tags)
	meta := mustMarshalJSON(g.Metadata)
	_, err := db.Exec(
		`INSERT INTO goals (id, namespace_id, title, description, status, priority, tags, context, dag_id, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.NamespaceID, g.Title, g.Description, string(g.Status), g.Priority, tags, g.Context, g.DAGID, meta,
		g.CreatedAt.Format(time.RFC3339Nano), g.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func updateGoalRecord(db *sql.DB, g *Goal) error {
	tags := mustMarshalJSON(g.Tags)
	meta := mustMarshalJSON(g.Metadata)
	_, err := db.Exec(
		`UPDATE goals SET title=?, description=?, status=?, priority=?, tags=?, context=?, dag_id=?, metadata=?, updated_at=? WHERE namespace_id=? AND id=?`,
		g.Title, g.Description, string(g.Status), g.Priority, tags, g.Context, g.DAGID, meta,
		g.UpdatedAt.Format(time.RFC3339Nano), g.NamespaceID, g.ID,
	)
	return err
}

func loadGoals(db *sql.DB) (map[string]map[string]*Goal, error) {
	rows, err := db.Query(`SELECT id, namespace_id, title, description, status, priority, tags, context, dag_id, metadata, created_at, updated_at FROM goals`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*Goal)
	for rows.Next() {
		var (
			id, nsID, title, desc, statusStr, context, dagID, metaStr, tagsStr, createdAtStr, updatedAtStr string
			priority int
		)
		if err := rows.Scan(&id, &nsID, &title, &desc, &statusStr, &priority, &tagsStr, &context, &dagID, &metaStr, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		goal := &Goal{
			ID:          id,
			NamespaceID: nsID,
			Title:       title,
			Description: desc,
			Status:      GoalStatus(statusStr),
			Priority:    priority,
			Tags:        mustUnmarshalStringSlice(tagsStr),
			Context:     context,
			DAGID:       dagID,
			Metadata:    mustUnmarshalStringMap(metaStr),
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*Goal)
		}
		out[nsID][id] = goal
	}
	return out, rows.Err()
}
```

2. Call `insertGoal` in `CreateGoal`, `updateGoalRecord` in `UpdateGoal` and `PromoteGoal`.
3. In `pkg/engine/engine.go` (`NewEngine`), call `loadGoals(db)` and assign to `e.goals = goalMap`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/engine -v -run TestGoalPersistenceAndReopen`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/engine/store_goal.go pkg/engine/engine.go pkg/engine/goal.go pkg/engine/goal_test.go
git commit -m "feat(engine): add SQLite persistence and recovery for goals"
```

---

### Task 5: MCP Tool Endpoints (`goal_create`, `goal_list`, `goal_get`, `goal_update`, `goal_promote`)

**Files:**
- Create: `pkg/server/handlers_goal.go`
- Modify: `pkg/server/mcp.go`
- Create: `pkg/server/goal_mcp_test.go`

- [ ] **Step 1: Write the failing test for Goal MCP tools**

Create `pkg/server/goal_mcp_test.go`:
```go
package server

import (
	"context"
	"testing"

	"github.com/toustifer/agentflow/pkg/engine"
)

func TestGoalMCPHandlersLifecycle(t *testing.T) {
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	srv, err := NewServer(ServerConfig{Engine: eng})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ctx := context.Background()

	_, err = srv.Handle(ctx, "namespace_create", map[string]any{"id": "test-ns", "name": "Test NS"})
	if err != nil {
		t.Fatalf("namespace_create failed: %v", err)
	}

	// 1. goal_create
	createRes, err := srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "test-ns",
		"title":        "Optimize Query Engine",
		"description":  "Reduce SQL latency",
		"priority":     float64(10),
		"tags":         []any{"perf", "db"},
	})
	if err != nil {
		t.Fatalf("goal_create failed: %v", err)
	}
	goalMap := createRes["goal"].(map[string]any)
	goalID := goalMap["id"].(string)
	if goalID != "G-1" {
		t.Fatalf("expected G-1, got %s", goalID)
	}

	// 2. goal_get
	getRes, err := srv.Handle(ctx, "goal_get", map[string]any{
		"namespace_id": "test-ns",
		"goal_id":      goalID,
	})
	if err != nil {
		t.Fatalf("goal_get failed: %v", err)
	}
	if getRes["goal"].(map[string]any)["title"] != "Optimize Query Engine" {
		t.Fatalf("unexpected goal_get title")
	}

	// 3. goal_update
	updateRes, err := srv.Handle(ctx, "goal_update", map[string]any{
		"namespace_id": "test-ns",
		"goal_id":      goalID,
		"status":       "deferred",
		"priority":     float64(20),
	})
	if err != nil {
		t.Fatalf("goal_update failed: %v", err)
	}
	if updateRes["goal"].(map[string]any)["status"] != "deferred" {
		t.Fatalf("expected status deferred")
	}

	// 4. goal_list
	listRes, err := srv.Handle(ctx, "goal_list", map[string]any{
		"namespace_id": "test-ns",
	})
	if err != nil {
		t.Fatalf("goal_list failed: %v", err)
	}
	goals := listRes["goals"].([]any)
	if len(goals) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(goals))
	}

	// 5. goal_promote
	promoteRes, err := srv.Handle(ctx, "goal_promote", map[string]any{
		"namespace_id": "test-ns",
		"goal_id":      goalID,
	})
	if err != nil {
		t.Fatalf("goal_promote failed: %v", err)
	}
	promotedGoal := promoteRes["goal"].(map[string]any)
	if promotedGoal["status"] != "promoted" {
		t.Fatalf("expected status promoted, got %v", promotedGoal["status"])
	}
	createdDAG := promoteRes["dag"].(map[string]any)
	if createdDAG["id"] != "dag-G-1" {
		t.Fatalf("expected dag-G-1, got %v", createdDAG["id"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/server -v -run TestGoalMCPHandlersLifecycle`
Expected: FAIL ("unknown tool: goal_create").

- [ ] **Step 3: Implement MCP handlers and register tools**

1. Create `pkg/server/handlers_goal.go`:
Implement:
- `goalToMap(g *engine.Goal) map[string]any`
- `handleGoalCreate(ctx context.Context, input map[string]any) (map[string]any, error)`
- `handleGoalList(ctx context.Context, input map[string]any) (map[string]any, error)`
- `handleGoalGet(ctx context.Context, input map[string]any) (map[string]any, error)`
- `handleGoalUpdate(ctx context.Context, input map[string]any) (map[string]any, error)`
- `handleGoalPromote(ctx context.Context, input map[string]any) (map[string]any, error)`

2. In `pkg/server/mcp.go`:
- Add tools to `Tools()`: `"goal_create"`, `"goal_list"`, `"goal_get"`, `"goal_update"`, `"goal_promote"`.
- Add schema definitions in `toolInputSchema`.
- Route tools in `Handle(...)`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/server -v -run TestGoalMCPHandlersLifecycle`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/server/handlers_goal.go pkg/server/mcp.go pkg/server/goal_mcp_test.go
git commit -m "feat(server): expose goal_create, goal_list, goal_get, goal_update, goal_promote MCP tools"
```

---

### Task 6: Project Inspect Integration (Backlog Summary & Candidate Goals)

**Files:**
- Modify: `pkg/server/handlers_ext.go`
- Test: `pkg/server/goal_mcp_test.go`

- [ ] **Step 1: Write the failing test for project_inspect backlog visibility**

Add to `pkg/server/goal_mcp_test.go`:
```go
func TestProjectInspectIncludesBacklogSummary(t *testing.T) {
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	srv, err := NewServer(ServerConfig{Engine: eng})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ctx := context.Background()
	_, err = srv.Handle(ctx, "namespace_create", map[string]any{"id": "test-inspect", "name": "Test Inspect"})
	if err != nil {
		t.Fatalf("create ns failed: %v", err)
	}

	// Create 2 goals: 1 pending, 1 deferred
	_, _ = srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "test-inspect",
		"title":        "Pending Goal",
		"priority":     float64(5),
	})
	g2, _ := srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "test-inspect",
		"title":        "Deferred Goal",
		"priority":     float64(1),
	})
	_, _ = srv.Handle(ctx, "goal_update", map[string]any{
		"namespace_id": "test-inspect",
		"goal_id":      g2["goal"].(map[string]any)["id"],
		"status":       "deferred",
	})

	// Inspect
	res, err := srv.Handle(ctx, "project_inspect", map[string]any{"namespace_id": "test-inspect"})
	if err != nil {
		t.Fatalf("project_inspect failed: %v", err)
	}
	summary := res["summary"].(map[string]any)
	if summary["backlog_count"] != 2 {
		t.Fatalf("expected backlog_count 2, got %v", summary["backlog_count"])
	}
	backlog := res["backlog"].([]any)
	if len(backlog) != 2 {
		t.Fatalf("expected 2 items in backlog, got %d", len(backlog))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/server -v -run TestProjectInspectIncludesBacklogSummary`
Expected: FAIL (`backlog_count` not present or nil).

- [ ] **Step 3: Update `handleProjectInspect` in `pkg/server/handlers_ext.go`**

In `pkg/server/handlers_ext.go`:
- Retrieve active goals for namespace via `s.engine.ListGoals(ctx, engine.GoalFilter{NamespaceID: nsID})`.
- Calculate `backlogCount = len(goals)`.
- Put `summary["backlog_count"] = backlogCount`.
- Format `backlogItems` and attach `response["backlog"] = backlogItems`.
- If `focus == "backlog"`, attach full goal details to `response["backlog_detail"]`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/server -v -run TestProjectInspectIncludesBacklogSummary`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add pkg/server/handlers_ext.go pkg/server/goal_mcp_test.go
git commit -m "feat(server): inject backlog summary and candidate items into project_inspect"
```

---

### Task 7: Project Next Steps Integration (Candidate Goal Promotion Recommendations)

**Files:**
- Modify: `pkg/server/handlers_nextsteps.go`
- Test: `pkg/server/goal_mcp_test.go`

- [ ] **Step 1: Write the failing test for project_next_steps backlog recommendations**

Add to `pkg/server/goal_mcp_test.go`:
```go
func TestProjectNextStepsRecommendsBacklogPromotionWhenDone(t *testing.T) {
	eng, err := engine.NewEngine(engine.NewEngineConfig{})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	srv, err := NewServer(ServerConfig{Engine: eng})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	ctx := context.Background()
	_, _ = srv.Handle(ctx, "namespace_create", map[string]any{"id": "test-ns-done", "name": "Done NS"})
	_, _ = srv.Handle(ctx, "worker_register", map[string]any{
		"namespace_id":    "test-ns-done",
		"worker_id":       "w1",
		"name":            "Worker 1",
		"prompt_template": "Worker",
	})
	_, _ = srv.Handle(ctx, "dag_create", map[string]any{
		"namespace_id": "test-ns-done",
		"dag_id":       "dag-1",
		"title":        "DAG 1",
	})
	_, _ = srv.Handle(ctx, "task_create", map[string]any{
		"namespace_id":    "test-ns-done",
		"task_id":         "T1",
		"dag_id":          "dag-1",
		"title":           "Task 1",
		"assigned_worker": "w1",
	})
	// Complete task
	_, _ = srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "test-ns-done",
		"task_id":      "T1",
		"transition":   "start",
	})
	_, _ = srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "test-ns-done",
		"task_id":      "T1",
		"transition":   "submit",
	})
	_, _ = srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "test-ns-done",
		"task_id":      "T1",
		"transition":   "pass",
	})

	// Create pending goal in backlog
	_, _ = srv.Handle(ctx, "goal_create", map[string]any{
		"namespace_id": "test-ns-done",
		"title":        "Next Big Feature",
		"priority":     float64(50),
	})

	// Check project_next_steps
	res, err := srv.Handle(ctx, "project_next_steps", map[string]any{
		"namespace_id": "test-ns-done",
		"dag_id":       "dag-1",
	})
	if err != nil {
		t.Fatalf("project_next_steps failed: %v", err)
	}

	actions := res["actions"].([]string)
	hasGoalPromote := false
	for _, a := range actions {
		if a == "goal_promote" {
			hasGoalPromote = true
			break
		}
	}
	if !hasGoalPromote {
		t.Fatalf("expected goal_promote in actions, got %v", actions)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/server -v -run TestProjectNextStepsRecommendsBacklogPromotionWhenDone`
Expected: FAIL (goal_promote not in actions).

- [ ] **Step 3: Update `handleProjectNextSteps` in `pkg/server/handlers_nextsteps.go`**

In `pkg/server/handlers_nextsteps.go`:
- When in `doneTasks == totalTasks` (Phase `done`), check `s.engine.ListGoals(ctx, engine.GoalFilter{NamespaceID: nsID, Statuses: []engine.GoalStatus{engine.GoalPending, engine.GoalDeferred}})`.
- If backlog goals exist:
  - Add to `next_steps`: `fmt.Sprintf("当前 DAG 已结案，Backlog 储备池中有 %d 个待办目标（推荐优先晋升 %s: %q）", len(backlog), backlog[0].ID, backlog[0].Title)`
  - Add to `actions`: `"goal_promote"`, `"goal_list"`
- When in Phase `plan` with 0 DAGs:
  - If backlog goals exist:
    - Add to `next_steps`: `fmt.Sprintf("Backlog 储备池中有 %d 个待办目标，可使用 goal_promote 一键晋升为执行 DAG", len(backlog))`
    - Add to `actions`: `"goal_promote"`, `"goal_list"`

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/server -v -run TestProjectNextStepsRecommendsBacklogPromotionWhenDone`
Expected: PASS

- [ ] **Step 5: Run all package tests to ensure zero regressions**

Run:
```bash
go test ./pkg/engine -v
go test ./pkg/server -v
```
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add pkg/server/handlers_nextsteps.go pkg/server/goal_mcp_test.go
git commit -m "feat(server): recommend backlog goal promotion in project_next_steps upon completion or planning"
```
