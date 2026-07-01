package engine

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// CreateTaskBatchRequest is a bulk version of CreateTaskRequest.
type CreateTaskBatchRequest struct {
	NamespaceID string
	DAGID       string
	Tasks       []BatchTaskItem
}

type BatchTaskItem struct {
	ID                 string
	Title              string
	Description        string
	AssignedWorker     string
	AcceptanceCriteria []string
	OutputFiles        []string
	DependsOn          []string
	Tags               []string
	Priority           int
	EstimatedHours     float64
	Metadata           map[string]string
}

type BatchResult struct {
	Created []*Task `json:"created"`
}

// CreateTaskBatch creates multiple tasks atomically within a single lock.
// Circular dependency detection is done across the whole batch.
func (e *Engine) CreateTaskBatch(ctx context.Context, req CreateTaskBatchRequest) (*BatchResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[req.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	if len(req.Tasks) == 0 {
		return nil, errors.New("no tasks in batch")
	}

	// Collect all IDs and check for duplicates in batch
	newIDs := make(map[string]bool)
	for _, t := range req.Tasks {
		if t.ID == "" {
			return nil, errors.New("each task must have an id")
		}
		if newIDs[t.ID] {
			return nil, fmt.Errorf("duplicate task id in batch: %s", t.ID)
		}
		newIDs[t.ID] = true
	}

	// Check for conflicts with existing tasks
	for _, t := range req.Tasks {
		if _, ok := e.tasks[req.NamespaceID][t.ID]; ok {
			return nil, fmt.Errorf("task %s already exists", t.ID)
		}
	}

	// Validate all depends_on references (within batch or existing)
	for _, t := range req.Tasks {
		for _, dep := range t.DependsOn {
			if !newIDs[dep] {
				if _, ok := e.tasks[req.NamespaceID][dep]; !ok {
					return nil, fmt.Errorf("task %s depends on %s which does not exist", t.ID, dep)
				}
			}
		}
	}

	// Detect cycles across the batch
	adj := make(map[string][]string)
	for _, t := range req.Tasks {
		adj[t.ID] = t.DependsOn
	}
	if err := detectBatchCycles(adj); err != nil {
		return nil, fmt.Errorf("circular dependency in batch: %w", err)
	}

	// Create all tasks
	now := time.Now().UTC()
	created := make([]*Task, 0, len(req.Tasks))
	for _, t := range req.Tasks {
		task := &Task{
			ID:                 t.ID,
			NamespaceID:        req.NamespaceID,
			Title:              t.Title,
			Description:        t.Description,
			State:              TaskAssigned,
			AssignedWorker:     t.AssignedWorker,
			AcceptanceCriteria: cloneStrings(t.AcceptanceCriteria),
			OutputFiles:        cloneStrings(t.OutputFiles),
			DAGID:              req.DAGID,
			DependsOn:          cloneStrings(t.DependsOn),
			Tags:               cloneStrings(t.Tags),
			Priority:           t.Priority,
			EstimatedHours:     t.EstimatedHours,
			ReviewCycle:        0,
			CreatedAt:          now,
			UpdatedAt:          now,
			Metadata:           cloneStringMap(t.Metadata),
		}

		if e.tasks[req.NamespaceID] == nil {
			e.tasks[req.NamespaceID] = make(map[string]*Task)
		}
		e.tasks[req.NamespaceID][t.ID] = task

		ev := Event{
			TaskID:    t.ID,
			Transition: string(TransStart),
			FromState:  TaskAssigned,
			ToState:    TaskAssigned,
			Timestamp:  now,
			Reason:     "batch created",
		}
		e.appendEventLocked(req.NamespaceID, t.ID, ev)

		if e.db != nil {
			if err := insertTask(e.db, task); err != nil {
				return nil, fmt.Errorf("persist task %s: %w", t.ID, err)
			}
			if err := insertEvent(e.db, req.NamespaceID, t.ID, ev); err != nil {
				return nil, fmt.Errorf("persist event for %s: %w", t.ID, err)
			}
		}

		created = append(created, cloneTask(task))
	}

	// Update DAG status once
	if req.DAGID != "" {
		e.recomputeDAGStatusForTaskLocked(req.NamespaceID, req.DAGID)
	}

	return &BatchResult{Created: created}, nil
}

// detectBatchCycles runs DFS on the batch adjacency map to find cycles.
func detectBatchCycles(adj map[string][]string) error {
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var dfs func(id string) error
	dfs = func(id string) error {
		visited[id] = true
		inStack[id] = true
		for _, dep := range adj[id] {
			if !visited[dep] {
				if err := dfs(dep); err != nil {
					return err
				}
			} else if inStack[dep] {
				return fmt.Errorf("cycle involving %s and %s", id, dep)
			}
		}
		inStack[id] = false
		return nil
	}

	for id := range adj {
		if !visited[id] {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}
	return nil
}
