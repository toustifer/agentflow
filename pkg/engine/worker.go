package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Worker — global registry; Status is computed at runtime from tasks
// ---------------------------------------------------------------------------

type WorkerStatus string

const (
	WorkerIdle WorkerStatus = "idle"
	WorkerBusy WorkerStatus = "busy"
)

type Worker struct {
	ID               string            `json:"id"`
	NamespaceID      string            `json:"namespace_id"`
	Name             string            `json:"name"`
	Kind             string            `json:"kind,omitempty"`
	Scope            string            `json:"scope,omitempty"`
	Skills           []string          `json:"skills,omitempty"`
	TaskTags         []string          `json:"task_tags,omitempty"`
	PromptTemplate   string            `json:"prompt_template,omitempty"`
	RequiredReads    []string          `json:"required_reads,omitempty"`
	RecommendedMCP   []string          `json:"recommended_mcp,omitempty"`
	LaunchMode       string            `json:"launch_mode,omitempty"`
	HandoffTargets   []string          `json:"handoff_targets,omitempty"`
	RecoveryPolicy   []string          `json:"recovery_policy,omitempty"`
	FallbackMCP      []string          `json:"fallback_mcp,omitempty"`
	StuckPlaybook    string            `json:"stuck_playbook,omitempty"`
	EscalationMode   string            `json:"escalation_mode,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type RegisterWorkerRequest struct {
	NamespaceID      string
	ID               string
	Name             string
	Kind             string
	Scope            string
	Skills           []string
	TaskTags         []string
	PromptTemplate   string
	RequiredReads    []string
	RecommendedMCP   []string
	LaunchMode       string
	HandoffTargets   []string
	RecoveryPolicy   []string
	FallbackMCP      []string
	StuckPlaybook    string
	EscalationMode   string
	Metadata         map[string]string
}

type UpdateWorkerRequest struct {
	Name             string
	Kind             string
	Scope            string
	Skills           []string
	TaskTags         []string
	PromptTemplate   string
	RequiredReads    []string
	RecommendedMCP   []string
	LaunchMode       string
	HandoffTargets   []string
	RecoveryPolicy   []string
	FallbackMCP      []string
	StuckPlaybook    string
	EscalationMode   string
	Metadata         map[string]string
}

// ---------------------------------------------------------------------------
// Worker CRUD
// ---------------------------------------------------------------------------

func (e *Engine) RegisterWorker(ctx context.Context, req RegisterWorkerRequest) (*Worker, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[req.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	if req.ID == "" {
		return nil, errors.New("worker id is required")
	}
	if req.Name == "" {
		return nil, errors.New("worker name is required")
	}

	if e.workers[req.NamespaceID] == nil {
		e.workers[req.NamespaceID] = make(map[string]*Worker)
	}
	if _, ok := e.workers[req.NamespaceID][req.ID]; ok {
		return nil, errors.New("worker already exists")
	}

	now := time.Now().UTC()
	w := &Worker{
		ID:             req.ID,
		NamespaceID:    req.NamespaceID,
		Name:           req.Name,
		Kind:           req.Kind,
		Scope:          req.Scope,
		Skills:         cloneStrings(req.Skills),
		TaskTags:       cloneStrings(req.TaskTags),
		PromptTemplate: req.PromptTemplate,
		RequiredReads:  cloneStrings(req.RequiredReads),
		RecommendedMCP: cloneStrings(req.RecommendedMCP),
		LaunchMode:     req.LaunchMode,
		HandoffTargets: cloneStrings(req.HandoffTargets),
		RecoveryPolicy: cloneStrings(req.RecoveryPolicy),
		FallbackMCP:    cloneStrings(req.FallbackMCP),
		StuckPlaybook:  req.StuckPlaybook,
		EscalationMode: req.EscalationMode,
		Metadata:       cloneStringMap(req.Metadata),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	e.workers[req.NamespaceID][req.ID] = w

	if e.db != nil {
		if err := insertWorker(e.db, w); err != nil {
			delete(e.workers[req.NamespaceID], req.ID)
			return nil, fmt.Errorf("persist worker: %w", err)
		}
	}

	return cloneWorker(w), nil
}

func (e *Engine) GetWorker(ctx context.Context, nsID, workerID string) (*Worker, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	w, ok := e.workers[nsID][workerID]
	if !ok {
		return nil, errors.New("worker not found")
	}
	return cloneWorker(w), nil
}

func (e *Engine) ListWorkers(ctx context.Context, nsID string) ([]Worker, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	out := make([]Worker, 0, len(e.workers[nsID]))
	for _, w := range e.workers[nsID] {
		out = append(out, *cloneWorker(w))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (e *Engine) UpdateWorker(ctx context.Context, nsID, workerID string, req UpdateWorkerRequest) (*Worker, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	w, ok := e.workers[nsID][workerID]
	if !ok {
		return nil, errors.New("worker not found")
	}

	if req.Name != "" {
		w.Name = req.Name
	}
	if req.Kind != "" {
		w.Kind = req.Kind
	}
	if req.Scope != "" {
		w.Scope = req.Scope
	}
	if req.Skills != nil {
		w.Skills = cloneStrings(req.Skills)
	}
	if req.TaskTags != nil {
		w.TaskTags = cloneStrings(req.TaskTags)
	}
	if req.PromptTemplate != "" {
		w.PromptTemplate = req.PromptTemplate
	}
	if req.RequiredReads != nil {
		w.RequiredReads = cloneStrings(req.RequiredReads)
	}
	if req.RecommendedMCP != nil {
		w.RecommendedMCP = cloneStrings(req.RecommendedMCP)
	}
	if req.LaunchMode != "" {
		w.LaunchMode = req.LaunchMode
	}
	if req.HandoffTargets != nil {
		w.HandoffTargets = cloneStrings(req.HandoffTargets)
	}
	if req.RecoveryPolicy != nil {
		w.RecoveryPolicy = cloneStrings(req.RecoveryPolicy)
	}
	if req.FallbackMCP != nil {
		w.FallbackMCP = cloneStrings(req.FallbackMCP)
	}
	if req.StuckPlaybook != "" {
		w.StuckPlaybook = req.StuckPlaybook
	}
	if req.EscalationMode != "" {
		w.EscalationMode = req.EscalationMode
	}
	if req.Metadata != nil {
		w.Metadata = cloneStringMap(req.Metadata)
	}
	w.UpdatedAt = time.Now().UTC()

	if e.db != nil {
		if err := updateWorker(e.db, w); err != nil {
			return nil, fmt.Errorf("persist worker update: %w", err)
		}
	}

	return cloneWorker(w), nil
}

// WorkerStatus computes whether a worker is busy or idle by scanning
// all tasks across all DAGs in the namespace.
func (e *Engine) WorkerStatus(ctx context.Context, nsID, workerID string) WorkerStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return WorkerIdle
	}
	for _, task := range e.tasks[nsID] {
		if task.AssignedWorker == workerID {
			switch task.State {
			case TaskExecuting, TaskReviewPending, TaskReworkNeeded:
				return WorkerBusy
			}
		}
	}
	return WorkerIdle
}

func (e *Engine) WorkerPromptGet(ctx context.Context, nsID, workerID, taskID, taskTitle string, asReviewer bool) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	ns, ok := e.namespaces[nsID]
	if !ok {
		return "", ErrNamespaceNotFound
	}
	w, ok := e.workers[nsID][workerID]
	if !ok {
		return "", errors.New("worker not found")
	}
	if w.PromptTemplate == "" {
		return "", errors.New("worker has no prompt template configured")
	}

	title := taskTitle
	dagID := ""
	dagTitle := ""
	branch := ""
	workdir := ""
	repoPath := ""
	worktreePath := ""
	baseBranch := ""
	namespaceName := ns.Name
	reviewCommit := ""
	reviewDiff := ""
	assignedWorkerName := ""

	if ns.Metadata != nil {
		workdir = ns.Metadata["workdir"]
		repoPath = ns.Metadata["workdir"]
		baseBranch = ns.Metadata["git_main_branch"]
	}

	if taskID != "" {
		task, ok := e.tasks[nsID][taskID]
		if !ok {
			return "", errors.New("task not found")
		}
		if title == "" {
			title = task.Title
		}
		dagID = task.DAGID
		if task.Metadata != nil {
			if v := task.Metadata["git.branch"]; v != "" {
				branch = v
			}
			if v := task.Metadata["git.repo_path"]; v != "" {
				repoPath = v
			}
			if v := task.Metadata["git.worktree_path"]; v != "" {
				worktreePath = v
			}
			if v := task.Metadata["git.base_branch"]; v != "" {
				baseBranch = v
			}
			if asReviewer {
				if v := task.Metadata["review.commit"]; v != "" {
					reviewCommit = v
				}
				if v := task.Metadata["review.diff"]; v != "" {
					reviewDiff = v
				}
			}
			assignedWorkerName = task.AssignedWorker
		}
		if dagID != "" {
			if dag, ok := e.dags[nsID][dagID]; ok {
				dagTitle = dag.Title
				if branch == "" {
					branch = dag.Branch
				}
			}
		}
	}

	tpl := w.PromptTemplate
	tpl = strings.ReplaceAll(tpl, "{task_id}", taskID)
	tpl = strings.ReplaceAll(tpl, "{title}", title)
	tpl = strings.ReplaceAll(tpl, "{worker_id}", workerID)
	tpl = strings.ReplaceAll(tpl, "{worker_name}", w.Name)
	tpl = strings.ReplaceAll(tpl, "{scope}", w.Scope)
	tpl = strings.ReplaceAll(tpl, "{namespace_id}", nsID)
	tpl = strings.ReplaceAll(tpl, "{namespace_name}", namespaceName)
	tpl = strings.ReplaceAll(tpl, "{dag_id}", dagID)
	tpl = strings.ReplaceAll(tpl, "{dag_title}", dagTitle)
	tpl = strings.ReplaceAll(tpl, "{branch}", branch)
	tpl = strings.ReplaceAll(tpl, "{workdir}", workdir)
	tpl = strings.ReplaceAll(tpl, "{repo_path}", repoPath)
	tpl = strings.ReplaceAll(tpl, "{worktree_path}", worktreePath)
	tpl = strings.ReplaceAll(tpl, "{base_branch}", baseBranch)
	tpl = strings.ReplaceAll(tpl, "{review_commit}", reviewCommit)
	tpl = strings.ReplaceAll(tpl, "{review.diff}", reviewDiff)
	tpl = strings.ReplaceAll(tpl, "{assigned_worker}", assignedWorkerName)
	return tpl, nil
}

// ---------------------------------------------------------------------------
// cloning helpers
// ---------------------------------------------------------------------------

func cloneWorker(w *Worker) *Worker {
	if w == nil {
		return nil
	}
	cpy := *w
	cpy.Skills = cloneStrings(w.Skills)
	cpy.TaskTags = cloneStrings(w.TaskTags)
	cpy.PromptTemplate = w.PromptTemplate
	cpy.RequiredReads = cloneStrings(w.RequiredReads)
	cpy.RecommendedMCP = cloneStrings(w.RecommendedMCP)
	cpy.HandoffTargets = cloneStrings(w.HandoffTargets)
	cpy.RecoveryPolicy = cloneStrings(w.RecoveryPolicy)
	cpy.FallbackMCP = cloneStrings(w.FallbackMCP)
	cpy.Metadata = cloneStringMap(w.Metadata)
	return &cpy
}
