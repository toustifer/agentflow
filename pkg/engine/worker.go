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
	ID             string            `json:"id"`
	NamespaceID    string            `json:"namespace_id"`
	Name           string            `json:"name"`
	Scope          string            `json:"scope,omitempty"`
	Skills         []string          `json:"skills,omitempty"`
	PromptTemplate string            `json:"prompt_template,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type RegisterWorkerRequest struct {
	NamespaceID    string
	ID             string
	Name           string
	Scope          string
	Skills         []string
	PromptTemplate string
	Metadata       map[string]string
}

type UpdateWorkerRequest struct {
	Name           string
	Scope          string
	Skills         []string
	PromptTemplate string
	Metadata       map[string]string
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
		ID:          req.ID,
		NamespaceID: req.NamespaceID,
		Name:        req.Name,
		Scope:       req.Scope,
		Skills:         cloneStrings(req.Skills),
		PromptTemplate: req.PromptTemplate,
		Metadata:       cloneStringMap(req.Metadata),
		CreatedAt:   now,
		UpdatedAt:   now,
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
	if req.Scope != "" {
		w.Scope = req.Scope
	}
	if req.Skills != nil {
		w.Skills = cloneStrings(req.Skills)
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

func (e *Engine) WorkerPromptGet(ctx context.Context, nsID, workerID, taskID, taskTitle string) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return "", ErrNamespaceNotFound
	}
	w, ok := e.workers[nsID][workerID]
	if !ok {
		return "", errors.New("worker not found")
	}
	if w.PromptTemplate == "" {
		return "", errors.New("worker has no prompt template configured")
	}

	tpl := w.PromptTemplate
	tpl = strings.ReplaceAll(tpl, "{task_id}", taskID)
	tpl = strings.ReplaceAll(tpl, "{title}", taskTitle)
	tpl = strings.ReplaceAll(tpl, "{worker_id}", workerID)
	tpl = strings.ReplaceAll(tpl, "{worker_name}", w.Name)
	tpl = strings.ReplaceAll(tpl, "{scope}", w.Scope)
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
	cpy.PromptTemplate = w.PromptTemplate
	cpy.Metadata = cloneStringMap(w.Metadata)
	return &cpy
}
