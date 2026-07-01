package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ---------------------------------------------------------------------------
// DAG — lightweight container; graph structure is derived from Task.depends_on
// ---------------------------------------------------------------------------

type DAGStatus string

const (
	DAGPlanning   DAGStatus = "planning"
	DAGInProgress DAGStatus = "in_progress"
	DAGDone       DAGStatus = "done"
	DAGCancelled  DAGStatus = "cancelled"
)

type DAG struct {
	ID          string    `json:"id"`
	NamespaceID string    `json:"namespace_id"`
	Title       string    `json:"title"`
	Branch      string    `json:"branch"`
	Status      DAGStatus `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateDAGRequest struct {
	NamespaceID string
	ID          string
	Title       string
	Branch      string
}

type UpdateDAGRequest struct {
	Title  string
	Branch string
}

// DAGGraph is the derived view returned by GetDAG.
type DAGGraph struct {
	DAG     DAG         `json:"dag"`
	Tasks   []Task      `json:"tasks"`
	Nodes   []GraphNode `json:"nodes"`
	Edges   []GraphEdge `json:"edges"`
	Workers []DAGWorker `json:"workers"`
}

type GraphNode struct {
	TaskID         string    `json:"task_id"`
	Title          string    `json:"title"`
	State          TaskState `json:"state"`
	AssignedWorker string    `json:"assigned_worker"`
	DependsOn      []string  `json:"depends_on"`
}

type GraphEdge struct {
	FromTaskID string `json:"from"`
	ToTaskID   string `json:"to"`
}

type DAGWorker struct {
	WorkerID string   `json:"worker_id"`
	Tasks    []string `json:"tasks"`
}

// ---------------------------------------------------------------------------
// DAG CRUD
// ---------------------------------------------------------------------------

func (e *Engine) CreateDAG(ctx context.Context, req CreateDAGRequest) (*DAG, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[req.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	if req.ID == "" {
		return nil, errors.New("dag id is required")
	}

	if e.dags[req.NamespaceID] == nil {
		e.dags[req.NamespaceID] = make(map[string]*DAG)
	}
	if _, ok := e.dags[req.NamespaceID][req.ID]; ok {
		return nil, errors.New("dag already exists")
	}

	now := time.Now().UTC()
	dag := &DAG{
		ID:          req.ID,
		NamespaceID: req.NamespaceID,
		Title:       req.Title,
		Branch:      req.Branch,
		Status:      DAGPlanning,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	e.dags[req.NamespaceID][req.ID] = dag

	if e.db != nil {
		if err := insertDAG(e.db, dag); err != nil {
			delete(e.dags[req.NamespaceID], req.ID)
			return nil, fmt.Errorf("persist dag: %w", err)
		}
	}

	return cloneDAG(dag), nil
}

func (e *Engine) GetDAG(ctx context.Context, nsID, dagID string) (*DAG, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	dag, ok := e.dags[nsID][dagID]
	if !ok {
		return nil, errors.New("dag not found")
	}
	return cloneDAG(dag), nil
}

func (e *Engine) ListDAGs(ctx context.Context, nsID string) ([]DAG, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	out := make([]DAG, 0, len(e.dags[nsID]))
	for _, dag := range e.dags[nsID] {
		out = append(out, *cloneDAG(dag))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (e *Engine) UpdateDAG(ctx context.Context, nsID, dagID string, req UpdateDAGRequest) (*DAG, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	dag, ok := e.dags[nsID][dagID]
	if !ok {
		return nil, errors.New("dag not found")
	}

	if req.Title != "" {
		dag.Title = req.Title
	}
	if req.Branch != "" {
		dag.Branch = req.Branch
	}
	dag.UpdatedAt = time.Now().UTC()

	if e.db != nil {
		if err := updateDAG(e.db, dag); err != nil {
			return nil, fmt.Errorf("persist dag update: %w", err)
		}
	}

	return cloneDAG(dag), nil
}

// GetDAGGraph builds the full graph view of a DAG from its tasks.
func (e *Engine) GetDAGGraph(ctx context.Context, nsID, dagID string) (*DAGGraph, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	dag, ok := e.dags[nsID][dagID]
	if !ok {
		return nil, errors.New("dag not found")
	}

	// Collect all tasks in this DAG
	var tasks []Task
	for _, t := range e.tasks[nsID] {
		if t.DAGID == dagID {
			tasks = append(tasks, *cloneTask(t))
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	// Build nodes, edges, worker grouping
	nodes := make([]GraphNode, 0, len(tasks))
	edges := make([]GraphEdge, 0)
	workerTasks := make(map[string][]string)

	for _, t := range tasks {
		nodes = append(nodes, GraphNode{
			TaskID:         t.ID,
			Title:          t.Title,
			State:          t.State,
			AssignedWorker: t.AssignedWorker,
			DependsOn:      t.DependsOn,
		})
		for _, dep := range t.DependsOn {
			edges = append(edges, GraphEdge{FromTaskID: dep, ToTaskID: t.ID})
		}
		if t.AssignedWorker != "" {
			workerTasks[t.AssignedWorker] = append(workerTasks[t.AssignedWorker], t.ID)
		}
	}

	workers := make([]DAGWorker, 0, len(workerTasks))
	for wid, tids := range workerTasks {
		workers = append(workers, DAGWorker{WorkerID: wid, Tasks: tids})
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].WorkerID < workers[j].WorkerID })

	// Compute DAG status from tasks
	_ = computeDAGStatus(tasks) // status is currently derived on read; future: persist back to dag.Status

	return &DAGGraph{
		DAG:     *cloneDAG(dag),
		Tasks:   tasks,
		Nodes:   nodes,
		Edges:   edges,
		Workers: workers,
	}, nil
}

func computeDAGStatus(tasks []Task) DAGStatus {
	if len(tasks) == 0 {
		return DAGPlanning
	}
	anyActive := false
	anyDone := false
	allDone := true
	for _, t := range tasks {
		switch t.State {
		case TaskExecuting, TaskReviewPending, TaskReworkNeeded:
			anyActive = true
			allDone = false
		case TaskDone:
			anyDone = true
		case TaskCancelled:
			// ok
		default: // assigned
			allDone = false
		}
	}
	if allDone {
		return DAGDone
	}
	if anyActive || anyDone {
		return DAGInProgress
	}
	return DAGPlanning
}

// RecomputeAndPersistDAGStatus recalculates the DAG status from its tasks
// and updates the stored value (both in-memory and DB).
func (e *Engine) RecomputeAndPersistDAGStatus(ctx context.Context, nsID, dagID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return ErrNamespaceNotFound
	}
	e.recomputeDAGStatusForTaskLocked(nsID, dagID)
	return nil
}

// ---------------------------------------------------------------------------
// DAGReport — structured project report
// ---------------------------------------------------------------------------

type DAGReport struct {
	DAG            DAG                `json:"dag"`
	TotalTasks     int                `json:"total_tasks"`
	DoneTasks      int                `json:"done_tasks"`
	ExecutingTasks int                `json:"executing_tasks"`
	PendingTasks   int                `json:"pending_tasks"`
	CompletionPct  float64            `json:"completion_pct"`
	Workers        []DAGWorkerSummary `json:"workers"`
}

type DAGWorkerSummary struct {
	WorkerID   string `json:"worker_id"`
	TotalTasks int    `json:"total_tasks"`
	DoneTasks  int    `json:"done_tasks"`
	Status     string `json:"status"`
}

func (e *Engine) DAGReport(ctx context.Context, nsID, dagID string) (*DAGReport, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	dag, ok := e.dags[nsID][dagID]
	if !ok {
		return nil, errors.New("dag not found")
	}

	var tasks []Task
	workerTasks := make(map[string][]string)
	for _, t := range e.tasks[nsID] {
		if t.DAGID == dagID {
			tasks = append(tasks, *cloneTask(t))
			if t.AssignedWorker != "" {
				workerTasks[t.AssignedWorker] = append(workerTasks[t.AssignedWorker], t.ID)
			}
		}
	}

	doneCount := 0
	execCount := 0
	for _, t := range tasks {
		switch t.State {
		case TaskDone:
			doneCount++
		case TaskExecuting, TaskReviewPending, TaskReworkNeeded:
			execCount++
		}
	}
	total := len(tasks)
	pending := total - doneCount - execCount
	pct := 0.0
	if total > 0 {
		pct = float64(doneCount) / float64(total) * 100
	}

	workers := make([]DAGWorkerSummary, 0, len(workerTasks))
	for wid, tids := range workerTasks {
		wDone := 0
		for _, tid := range tids {
			if t, ok := e.tasks[nsID][tid]; ok && t.State == TaskDone {
				wDone++
			}
		}
		ws := e.workerStatusUnsafe(nsID, wid)
		workers = append(workers, DAGWorkerSummary{
			WorkerID:   wid,
			TotalTasks: len(tids),
			DoneTasks:  wDone,
			Status:     string(ws),
		})
	}

	return &DAGReport{
		DAG:            *cloneDAG(dag),
		TotalTasks:     total,
		DoneTasks:      doneCount,
		ExecutingTasks: execCount,
		PendingTasks:   pending,
		CompletionPct:  pct,
		Workers:        workers,
	}, nil
}

// workerStatusUnsafe is the RLock-free version for callers already holding the lock.
func (e *Engine) workerStatusUnsafe(nsID, workerID string) WorkerStatus {
	for _, t := range e.tasks[nsID] {
		if t.AssignedWorker == workerID {
			switch t.State {
			case TaskExecuting, TaskReviewPending, TaskReworkNeeded:
				return WorkerBusy
			}
		}
	}
	return WorkerIdle
}

// recomputeDAGStatusForTaskLocked finds which DAG the task belongs to,
// recalculates its status, and persists. Caller must hold e.mu.
func (e *Engine) recomputeDAGStatusForTaskLocked(nsID, dagID string) {
	if dagID == "" {
		return
	}
	dag, ok := e.dags[nsID][dagID]
	if !ok {
		return
	}
	var tasks []Task
	for _, t := range e.tasks[nsID] {
		if t.DAGID == dagID {
			tasks = append(tasks, *t)
		}
	}
	computed := computeDAGStatus(tasks)
	if dag.Status == computed {
		return
	}
	dag.Status = computed
	dag.UpdatedAt = time.Now().UTC()
	if e.db != nil {
		_ = updateDAG(e.db, dag)
	}
}

// ValidateDAGDeps checks that all depends_on references exist within the DAG
// and that there are no circular dependencies.
func (e *Engine) ValidateDAGDeps(ctx context.Context, nsID, dagID string, deps []string, taskID string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return ErrNamespaceNotFound
	}

	// Build set of task IDs in this DAG
	dagTasks := make(map[string]bool)
	for _, t := range e.tasks[nsID] {
		if t.DAGID == dagID {
			dagTasks[t.ID] = true
		}
	}

	for _, dep := range deps {
		if !dagTasks[dep] {
			return fmt.Errorf("dependency %q not found in DAG %s", dep, dagID)
		}
	}

	// Circular dependency check: add proposed edges and DFS
	if taskID != "" {
		adj := make(map[string][]string)
		for _, t := range e.tasks[nsID] {
			if t.DAGID == dagID && t.ID != taskID {
				adj[t.ID] = t.DependsOn
			}
		}
		adj[taskID] = deps

		visited := make(map[string]bool)
		inStack := make(map[string]bool)
		var dfs func(id string) bool
		dfs = func(id string) bool {
			visited[id] = true
			inStack[id] = true
			for _, dep := range adj[id] {
				if !visited[dep] {
					if dfs(dep) {
						return true
					}
				} else if inStack[dep] {
					return true
				}
			}
			inStack[id] = false
			return false
		}

		for id := range adj {
			if !visited[id] {
				if dfs(id) {
					return errors.New("circular dependency detected")
				}
			}
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// cloning helpers
// ---------------------------------------------------------------------------

func cloneDAG(dag *DAG) *DAG {
	if dag == nil {
		return nil
	}
	cpy := *dag
	return &cpy
}
