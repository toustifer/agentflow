package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	ErrNamespaceNotFound  = errors.New("namespace not found")
	ErrTaskNotFound       = errors.New("task not found")
	ErrInvalidTransition  = errors.New("invalid task transition")
	ErrDuplicateTask      = errors.New("task already exists")
	ErrDuplicateNamespace = errors.New("namespace already exists")
)

type Engine struct {
	mu               sync.RWMutex
	namespaces       map[string]*Namespace
	tasks            map[string]map[string]*Task
	history          map[string]map[string][]Event
	dags             map[string]map[string]*DAG
	workers          map[string]map[string]*Worker
	handbooks        map[string]map[string]*WorkerHandbook
	workerDiaries    map[string]map[string]*WorkerDiary
	leaderDiaries    map[string]map[string]*LeaderDiary
	projectDocs      map[string][]ProjectDoc
	nextProjectDocID map[string]int64
	db               *sql.DB // non-nil when SQLite backend is active
}

type NewEngineConfig struct {
	// DBPath controls persistence.
	//   "" (empty)   – pure in-memory maps (current default).
	//   ":memory:"   – in-memory SQLite (useful for testing).
	//   "/path/to.db" – persistent SQLite file.
	DBPath string
}

type CreateNamespaceRequest struct {
	ID       string
	Name     string
	Metadata map[string]string
}

type CreateTaskRequest struct {
	NamespaceID        string
	ID                 string
	Title              string
	Description        string
	AssignedWorker     string
	AcceptanceCriteria []string
	OutputFiles        []string
	DAGID              string
	DependsOn          []string
	Tags               []string
	Priority           int
	EstimatedHours     float64
	Metadata           map[string]string
}

type StateFilter struct {
	States map[TaskState]bool
}

type Namespace struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type Task struct {
	ID                 string            `json:"id"`
	NamespaceID        string            `json:"namespace_id"`
	Title              string            `json:"title"`
	Description        string            `json:"description"`
	State              TaskState         `json:"state"`
	AssignedWorker     string            `json:"assigned_worker"`
	AcceptanceCriteria []string          `json:"acceptance_criteria"`
	OutputFiles        []string          `json:"output_files"`
	DAGID              string            `json:"dag_id,omitempty"`
	DependsOn          []string          `json:"depends_on,omitempty"`
	Tags               []string          `json:"tags,omitempty"`
	Priority           int               `json:"priority,omitempty"`
	EstimatedHours     float64           `json:"estimated_hours,omitempty"`
	ActualHours        float64           `json:"actual_hours,omitempty"`
	WorkerAgentID      string            `json:"worker_agent_id,omitempty"`
	ReviewCycle        int               `json:"review_cycle"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

type Event struct {
	TaskID     string            `json:"task_id"`
	Transition string            `json:"transition"`
	FromState  TaskState         `json:"from_state"`
	ToState    TaskState         `json:"to_state"`
	Timestamp  time.Time         `json:"timestamp"`
	Actor      string            `json:"actor,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type TaskState string

const (
	TaskAssigned      TaskState = "assigned"
	TaskExecuting     TaskState = "executing"
	TaskReviewPending TaskState = "review_pending"
	TaskReworkNeeded  TaskState = "rework_needed"
	TaskDone          TaskState = "done"
	TaskCancelled     TaskState = "cancelled"
)

type TaskTransition string

const (
	TransStart    TaskTransition = "start"
	TransSubmit   TaskTransition = "submit"
	TransPass     TaskTransition = "pass"
	TransRework   TaskTransition = "rework"
	TransReassign TaskTransition = "reassign"
	TransResume   TaskTransition = "resume"
	TransCancel   TaskTransition = "cancel"
)

func NewEngine(cfg NewEngineConfig) (*Engine, error) {
	e := &Engine{
		namespaces:       make(map[string]*Namespace),
		tasks:            make(map[string]map[string]*Task),
		history:          make(map[string]map[string][]Event),
		dags:             make(map[string]map[string]*DAG),
		workers:          make(map[string]map[string]*Worker),
		handbooks:        make(map[string]map[string]*WorkerHandbook),
		workerDiaries:    make(map[string]map[string]*WorkerDiary),
		leaderDiaries:    make(map[string]map[string]*LeaderDiary),
		projectDocs:      make(map[string][]ProjectDoc),
		nextProjectDocID: make(map[string]int64),
	}

	if cfg.DBPath != "" {
		db, err := openSQLite(cfg.DBPath)
		if err != nil {
			return nil, fmt.Errorf("sqlite init: %w", err)
		}
		e.db = db

		// Load existing data so a reopened engine sees prior state.
		nsMap, err := loadNamespaces(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("load namespaces: %w", err)
		}
		e.namespaces = nsMap

		taskMap, err := loadTasks(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("load tasks: %w", err)
		}
		e.tasks = taskMap

		histMap, err := loadHistory(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("load history: %w", err)
		}
		e.history = histMap

		dagMap, err := loadDAGs(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("load dags: %w", err)
		}
		e.dags = dagMap

		workerMap, err := loadWorkers(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("load workers: %w", err)
		}
		e.workers = workerMap

		hbMap, err := loadHandbooks(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("load handbooks: %w", err)
		}
		e.handbooks = hbMap

		wdMap, err := loadWorkerDiaries(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("load worker diaries: %w", err)
		}
		e.workerDiaries = wdMap

		ldMap, err := loadLeaderDiaries(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("load leader diaries: %w", err)
		}
		e.leaderDiaries = ldMap

		docMap, err := loadProjectDocs(db)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("load project docs: %w", err)
		}
		e.projectDocs = docMap
		for nsID, docs := range docMap {
			e.nextProjectDocID[nsID] = nextProjectDocID(docs) - 1
		}
	}

	return e, nil
}

func (e *Engine) Close() error {
	if e.db != nil {
		return e.db.Close()
	}
	return nil
}

func (e *Engine) CreateNamespace(ctx context.Context, req CreateNamespaceRequest) (*Namespace, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if req.ID == "" {
		return nil, errors.New("namespace id is required")
	}
	if _, ok := e.namespaces[req.ID]; ok {
		return nil, ErrDuplicateNamespace
	}

	now := time.Now().UTC()
	ns := &Namespace{
		ID:        req.ID,
		Name:      req.Name,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  cloneStringMap(req.Metadata),
	}
	e.namespaces[req.ID] = ns
	e.tasks[req.ID] = make(map[string]*Task)
	e.history[req.ID] = make(map[string][]Event)

	if e.db != nil {
		if err := insertNamespace(e.db, ns); err != nil {
			// Roll back in-memory state on persistence failure.
			delete(e.namespaces, req.ID)
			delete(e.tasks, req.ID)
			delete(e.history, req.ID)
			return nil, fmt.Errorf("persist namespace: %w", err)
		}
	}

	return cloneNamespace(ns), nil
}

func (e *Engine) GetNamespace(ctx context.Context, nsID string) (*Namespace, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ns, ok := e.namespaces[nsID]
	if !ok {
		return nil, ErrNamespaceNotFound
	}
	return cloneNamespace(ns), nil
}

func (e *Engine) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]Namespace, 0, len(e.namespaces))
	for _, ns := range e.namespaces {
		out = append(out, *cloneNamespace(ns))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// DeleteNamespace deletes a namespace and all its cascade data.
// If confirm is false, it returns a descriptive error listing what would be deleted.
func (e *Engine) DeleteNamespace(ctx context.Context, nsID string, confirm bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	ns, ok := e.namespaces[nsID]
	if !ok {
		return ErrNamespaceNotFound
	}

	if !confirm {
		taskCount := len(e.tasks[nsID])
		workerCount := len(e.workers[nsID])
		dagCount := len(e.dags[nsID])
		eventMap := e.history[nsID]
		eventCount := 0
		for _, evs := range eventMap {
			eventCount += len(evs)
		}
		return fmt.Errorf(
			"⚠️ 删除确认：namespace %q (%s) 将级联删除 %d 个 DAG、%d 个 Task、%d 个 Worker、%d 条事件。\n"+
				"如需确认删除，请再次调用 namespace_delete 时传入 confirm=true",
			ns.ID, ns.Name, dagCount, taskCount, workerCount, eventCount,
		)
	}

	// Delete from SQLite
	if e.db != nil {
		if err := deleteAllForNamespace(e.db, nsID); err != nil {
			return fmt.Errorf("delete from db: %w", err)
		}
	}

	// Delete from in-memory maps
	delete(e.namespaces, nsID)
	delete(e.tasks, nsID)
	delete(e.workers, nsID)
	delete(e.dags, nsID)
	delete(e.history, nsID)

	return nil
}

func (e *Engine) CreateTask(ctx context.Context, req CreateTaskRequest) (*Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[req.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	if req.ID == "" {
		return nil, errors.New("task id is required")
	}
	if e.tasks[req.NamespaceID] == nil {
		e.tasks[req.NamespaceID] = make(map[string]*Task)
	}
	if _, ok := e.tasks[req.NamespaceID][req.ID]; ok {
		return nil, ErrDuplicateTask
	}

	now := time.Now().UTC()
	task := &Task{
		ID:                 req.ID,
		NamespaceID:        req.NamespaceID,
		Title:              req.Title,
		Description:        req.Description,
		State:              TaskAssigned,
		AssignedWorker:     req.AssignedWorker,
		AcceptanceCriteria: cloneStrings(req.AcceptanceCriteria),
		OutputFiles:        cloneStrings(req.OutputFiles),
		DAGID:              req.DAGID,
		DependsOn:          cloneStrings(req.DependsOn),
		Tags:               cloneStrings(req.Tags),
		Priority:           req.Priority,
		EstimatedHours:     req.EstimatedHours,
		ReviewCycle:        0,
		CreatedAt:          now,
		UpdatedAt:          now,
		Metadata:           cloneStringMap(req.Metadata),
	}
	if req.DAGID != "" && len(req.DependsOn) > 0 {
		if err := e.validateDAGDepsLocked(req.NamespaceID, req.DAGID, req.DependsOn, req.ID); err != nil {
			return nil, err
		}
	}
	e.tasks[req.NamespaceID][req.ID] = task
	ev := Event{
		TaskID:     req.ID,
		Transition: string(TransStart),
		FromState:  TaskAssigned,
		ToState:    TaskAssigned,
		Timestamp:  now,
		Reason:     "task created",
		Metadata:   cloneStringMap(req.Metadata),
	}
	e.appendEventLocked(req.NamespaceID, req.ID, ev)

	if e.db != nil {
		if err := insertTask(e.db, task); err != nil {
			return nil, fmt.Errorf("persist task: %w", err)
		}
		if err := insertEvent(e.db, req.NamespaceID, req.ID, ev); err != nil {
			return nil, fmt.Errorf("persist event: %w", err)
		}
	}

	// Update DAG status if task belongs to one
	e.recomputeDAGStatusForTaskLocked(req.NamespaceID, req.DAGID)

	return cloneTask(task), nil
}

type UpdateTaskRequest struct {
	State          TaskState
	ReviewMetadata map[string]string
	Metadata       map[string]string
	WorkerAgentID  string
}

func (e *Engine) UpdateTask(ctx context.Context, nsID, taskID string, req UpdateTaskRequest) (*Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	task, err := e.getTaskLocked(nsID, taskID)
	if err != nil {
		return nil, err
	}
	if req.State != "" {
		task.State = req.State
	}
	if req.WorkerAgentID != "" {
		task.WorkerAgentID = req.WorkerAgentID
	}
	if req.ReviewMetadata != nil || req.Metadata != nil {
		task.Metadata = ensureMap(task.Metadata)
		for k, v := range req.ReviewMetadata {
			task.Metadata[k] = v
		}
		for k, v := range req.Metadata {
			task.Metadata[k] = v
		}
	}
	task.UpdatedAt = time.Now().UTC()
	if e.db != nil {
		if err := updateTask(e.db, task); err != nil {
			return nil, fmt.Errorf("persist task update: %w", err)
		}
	}
	return cloneTask(task), nil
}

func (e *Engine) TransitionTask(ctx context.Context, nsID, taskID string, t TaskTransition, meta map[string]string) (*Task, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	task, err := e.getTaskLocked(nsID, taskID)
	if err != nil {
		return nil, err
	}

	// Validate actor_role if provided.
	if role := meta["actor_role"]; role != "" {
		if err := validateActorRole(t, role); err != nil {
			return nil, err
		}
	}
	if err := validateTransitionMetadata(task, t, meta); err != nil {
		return nil, err
	}

	from := task.State
	to, err := applyTransition(task, t)
	if err != nil {
		return nil, err
	}
	task.State = to
	task.UpdatedAt = time.Now().UTC()
	if t == TransRework {
		task.ReviewCycle++
	}
	if v := meta["worker_agent_id"]; v != "" {
		task.WorkerAgentID = v
	}
	if len(meta) > 0 {
		task.Metadata = ensureMap(task.Metadata)
		for k, v := range meta {
			if v == "" {
				continue
			}
			task.Metadata[k] = v
		}
	}
	if t == TransStart || t == TransResume {
		task.Metadata = ensureMap(task.Metadata)
		task.Metadata["launch.ticket_state"] = "consumed"
	}

	ev := Event{
		TaskID:     taskID,
		Transition: string(t),
		FromState:  from,
		ToState:    to,
		Timestamp:  time.Now().UTC(),
		Actor:      meta["actor"],
		Reason:     meta["reason"],
		Metadata:   cloneStringMap(meta),
	}
	e.appendEventLocked(nsID, taskID, ev)

	if e.db != nil {
		if err := updateTask(e.db, task); err != nil {
			return nil, fmt.Errorf("persist task update: %w", err)
		}
		if err := insertEvent(e.db, nsID, taskID, ev); err != nil {
			return nil, fmt.Errorf("persist event: %w", err)
		}
	}

	// Update DAG status if task belongs to one
	e.recomputeDAGStatusForTaskLocked(nsID, task.DAGID)

	return cloneTask(task), nil
}

func (e *Engine) GetTask(ctx context.Context, nsID, taskID string) (*Task, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	task, err := e.getTaskLocked(nsID, taskID)
	if err != nil {
		return nil, err
	}
	return cloneTask(task), nil
}

func (e *Engine) ListTasks(ctx context.Context, nsID string, filter StateFilter) ([]Task, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	out := make([]Task, 0, len(e.tasks[nsID]))
	for _, task := range e.tasks[nsID] {
		if len(filter.States) > 0 && !filter.States[task.State] {
			continue
		}
		out = append(out, *cloneTask(task))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (e *Engine) GetHistory(ctx context.Context, nsID, taskID string) ([]Event, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	h := e.history[nsID][taskID]
	out := make([]Event, len(h))
	copy(out, h)
	return out, nil
}

// ---------------------------------------------------------------------------
// TaskQuery — advanced filtering
// ---------------------------------------------------------------------------

type TaskQuery struct {
	NamespaceID    string
	DAGID          string
	AssignedWorker string
	States         []TaskState
	Tags           []string
	PriorityGTE    int
	ReadyOnly      bool
}

func (e *Engine) QueryTasks(ctx context.Context, q TaskQuery) ([]Task, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[q.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}

	stateSet := make(map[TaskState]bool, len(q.States))
	for _, s := range q.States {
		stateSet[s] = true
	}
	tagSet := make(map[string]bool, len(q.Tags))
	for _, t := range q.Tags {
		tagSet[t] = true
	}

	out := make([]Task, 0)
	for _, t := range e.tasks[q.NamespaceID] {
		if q.DAGID != "" && t.DAGID != q.DAGID {
			continue
		}
		if q.AssignedWorker != "" && t.AssignedWorker != q.AssignedWorker {
			continue
		}
		if len(stateSet) > 0 && !stateSet[t.State] {
			continue
		}
		if q.PriorityGTE > 0 && t.Priority < q.PriorityGTE {
			continue
		}
		if len(tagSet) > 0 {
			allMatched := true
			for _, tag := range t.Tags {
				if !tagSet[tag] {
					allMatched = false
					break
				}
			}
			if !allMatched {
				continue
			}
		}
		if q.ReadyOnly {
			// ReadyOnly returns tasks that can be dispatched next:
			// - state is assigned or rework_needed (not already executing/done)
			// - all depends_on are in done state
			if t.State != TaskAssigned && t.State != TaskReworkNeeded {
				continue
			}
			ready := true
			for _, dep := range t.DependsOn {
				depTask, ok := e.tasks[q.NamespaceID][dep]
				if !ok || depTask.State != TaskDone {
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
		}
		out = append(out, *cloneTask(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// validateDAGDepsLocked checks for circular deps and missing references.
// Caller must hold e.mu.
func (e *Engine) validateDAGDepsLocked(nsID, dagID string, deps []string, taskID string) error {
	if _, ok := e.dags[nsID][dagID]; !ok {
		return errors.New("dag not found: " + dagID)
	}
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

	adj := make(map[string][]string)
	for _, t := range e.tasks[nsID] {
		if t.DAGID == dagID && t.ID != taskID {
			adj[t.ID] = t.DependsOn
		}
	}
	if taskID != "" {
		adj[taskID] = deps
	}

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
	return nil
}

func (e *Engine) getTaskLocked(nsID, taskID string) (*Task, error) {
	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	task, ok := e.tasks[nsID][taskID]
	if !ok {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

func (e *Engine) appendEventLocked(nsID, taskID string, event Event) {
	if e.history[nsID] == nil {
		e.history[nsID] = make(map[string][]Event)
	}
	e.history[nsID][taskID] = append(e.history[nsID][taskID], event)
}

func validateTransitionMetadata(task *Task, t TaskTransition, meta map[string]string) error {
	switch t {
	case TransStart, TransResume:
		hasProtocolSignals := meta["launch.ticket"] != "" || meta["worker_agent_id"] != "" || meta["runtime.status"] != ""
		if !hasProtocolSignals {
			if task.Metadata == nil || task.Metadata["launch.ticket"] == "" {
				return nil
			}
		}
		launchTicket := meta["launch.ticket"]
		if launchTicket == "" {
			return fmt.Errorf("%w: %s requires launch.ticket", ErrInvalidTransition, t)
		}
		issued := ""
		issuedState := ""
		if task.Metadata != nil {
			issued = task.Metadata["launch.ticket"]
			issuedState = task.Metadata["launch.ticket_state"]
			if expiresAt := task.Metadata["launch.ticket_expires_at"]; expiresAt != "" {
				deadline, err := time.Parse(time.RFC3339, expiresAt)
				if err != nil {
					return fmt.Errorf("%w: invalid launch.ticket_expires_at", ErrInvalidTransition)
				}
				if time.Now().UTC().After(deadline) {
					return fmt.Errorf("%w: launch ticket expired", ErrInvalidTransition)
				}
			}
		}
		if issued == "" || issued != launchTicket {
			return fmt.Errorf("%w: launch ticket mismatch", ErrInvalidTransition)
		}
		if issuedState != "issued" {
			return fmt.Errorf("%w: launch ticket is not active", ErrInvalidTransition)
		}
		workerAgentID := meta["worker_agent_id"]
		if workerAgentID == "" {
			return fmt.Errorf("%w: %s requires worker_agent_id", ErrInvalidTransition, t)
		}
		if meta["runtime.provider"] == "" {
			return fmt.Errorf("%w: %s requires runtime.provider", ErrInvalidTransition, t)
		}
		if meta["runtime.status"] != "started" {
			return fmt.Errorf("%w: %s requires runtime.status=started", ErrInvalidTransition, t)
		}
		if task.WorkerAgentID != "" && task.WorkerAgentID != workerAgentID {
			return fmt.Errorf("%w: task already bound to another worker_agent_id", ErrInvalidTransition)
		}
	}
	return nil
}

func applyTransition(task *Task, t TaskTransition) (TaskState, error) {
	switch t {
	case TransStart:
		if task.State != TaskAssigned && task.State != TaskReworkNeeded {
			return "", ErrInvalidTransition
		}
		return TaskExecuting, nil
	case TransSubmit:
		if task.State != TaskExecuting {
			return "", ErrInvalidTransition
		}
		return TaskReviewPending, nil
	case TransPass:
		if task.State != TaskReviewPending {
			return "", ErrInvalidTransition
		}
		return TaskDone, nil
	case TransRework:
		if task.State != TaskReviewPending {
			return "", ErrInvalidTransition
		}
		return TaskReworkNeeded, nil
	case TransReassign:
		if task.State != TaskReworkNeeded &&
			task.State != TaskExecuting &&
			task.State != TaskReviewPending {
			return "", ErrInvalidTransition
		}
		return TaskAssigned, nil
	case TransResume:
		if task.State != TaskReworkNeeded {
			return "", ErrInvalidTransition
		}
		return TaskExecuting, nil
	case TransCancel:
		return TaskCancelled, nil
	default:
		return "", fmt.Errorf("unknown transition %q", t)
	}
}

// AvailableTransition describes a transition the caller is allowed to invoke
// from the current task state.
type AvailableTransition struct {
	Transition string `json:"transition"`
	Role       string `json:"role"`
	ToState    string `json:"to_state"`
	Hint       string `json:"hint,omitempty"`
}

// AvailableTransitions returns the list of transitions that are legal from the
// task's current state, annotated with the role permitted to invoke each one.
func AvailableTransitions(task *Task) []AvailableTransition {
	switch task.State {
	case TaskAssigned:
		return []AvailableTransition{
			{Transition: string(TransStart), Role: "leader", ToState: string(TaskExecuting),
				Hint: "Leader dispatches the task to the assigned Worker"},
		}
	case TaskExecuting:
		return []AvailableTransition{
			{Transition: string(TransSubmit), Role: "worker", ToState: string(TaskReviewPending),
				Hint: "Worker has finished and delivered the output files"},
			{Transition: string(TransReassign), Role: "leader", ToState: string(TaskAssigned),
				Hint: "Leader reassigns to a different Worker"},
			{Transition: string(TransCancel), Role: "leader", ToState: string(TaskCancelled),
				Hint: "Leader aborts the task"},
		}
	case TaskReviewPending:
		return []AvailableTransition{
			{Transition: string(TransPass), Role: "reviewer", ToState: string(TaskDone),
				Hint: "All acceptance criteria pass"},
			{Transition: string(TransRework), Role: "reviewer", ToState: string(TaskReworkNeeded),
				Hint: "At least one acceptance criterion failed — Worker needs to redo"},
			{Transition: string(TransReassign), Role: "leader", ToState: string(TaskAssigned),
				Hint: "Leader reassigns to a different Worker"},
		}
	case TaskReworkNeeded:
		return []AvailableTransition{
			{Transition: string(TransResume), Role: "leader", ToState: string(TaskExecuting),
				Hint: "Leader re-dispatches the task after rework feedback"},
			{Transition: string(TransReassign), Role: "leader", ToState: string(TaskAssigned),
				Hint: "Leader reassigns to a different Worker"},
			{Transition: string(TransCancel), Role: "leader", ToState: string(TaskCancelled),
				Hint: "Leader aborts the task"},
		}
	case TaskDone, TaskCancelled:
		return nil
	default:
		return nil
	}
}

// roleAllowedTransitions maps each role to the transitions that role may invoke.
var roleAllowedTransitions = map[string]map[TaskTransition]bool{
	"leader": {
		TransStart:    true,
		TransResume:   true,
		TransReassign: true,
		TransCancel:   true,
	},
	"worker": {
		TransSubmit: true,
	},
	"reviewer": {
		TransPass:   true,
		TransRework: true,
	},
}

func validateActorRole(t TaskTransition, role string) error {
	allowed, ok := roleAllowedTransitions[role]
	if !ok {
		return fmt.Errorf("unknown actor_role %q: valid roles are leader, worker, reviewer", role)
	}
	if !allowed[t] {
		// Build a helpful list of what this role CAN do.
		canDo := make([]string, 0, 3)
		for trans := range allowed {
			canDo = append(canDo, string(trans))
		}
		return fmt.Errorf("actor_role %q is not allowed to invoke transition %q; this role can only use: %v", role, string(t), canDo)
	}
	return nil
}

func cloneNamespace(ns *Namespace) *Namespace {
	if ns == nil {
		return nil
	}
	cpy := *ns
	cpy.Metadata = cloneStringMap(ns.Metadata)
	return &cpy
}

func cloneTask(task *Task) *Task {
	if task == nil {
		return nil
	}
	cpy := *task
	cpy.AcceptanceCriteria = cloneStrings(task.AcceptanceCriteria)
	cpy.OutputFiles = cloneStrings(task.OutputFiles)
	cpy.DependsOn = cloneStrings(task.DependsOn)
	cpy.Tags = cloneStrings(task.Tags)
	cpy.Metadata = cloneStringMap(task.Metadata)
	return &cpy
}

func cloneStrings(v []string) []string {
	if v == nil {
		return nil
	}
	cpy := make([]string, len(v))
	copy(cpy, v)
	return cpy
}

func cloneStringMap(v map[string]string) map[string]string {
	if v == nil {
		return nil
	}
	cpy := make(map[string]string, len(v))
	for k, val := range v {
		cpy[k] = val
	}
	return cpy
}

func ensureMap(v map[string]string) map[string]string {
	if v != nil {
		return v
	}
	return make(map[string]string)
}
