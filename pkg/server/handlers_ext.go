package server

import (
	"context"
	"fmt"
	"time"

	"github.com/toustifer/agentflow/pkg/engine"
)

// ---------------------------------------------------------------------------
// Optional input helpers (extend mcp.go's existing ones)
// ---------------------------------------------------------------------------

func optionalInt(input map[string]any, key string) (int, error) {
	value, ok := input[key]
	if !ok || value == nil {
		return 0, nil
	}
	switch v := value.(type) {
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("%w: %s must be an integer", ErrInvalidToolInput, key)
	}
}

func optionalFloat(input map[string]any, key string) (float64, error) {
	value, ok := input[key]
	if !ok || value == nil {
		return 0, nil
	}
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("%w: %s must be a number", ErrInvalidToolInput, key)
	}
}

// ---------------------------------------------------------------------------
// task_query
// ---------------------------------------------------------------------------

func (s *Server) handleTaskQuery(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	q := engine.TaskQuery{
		NamespaceID:    nsID,
		DAGID:          stringInputOrEmpty(input, "dag_id"),
		AssignedWorker: stringInputOrEmpty(input, "assigned_worker"),
		PriorityGTE:    intInputOrZero(input, "priority_gte"),
		ReadyOnly:      boolInputOrFalse(input, "ready_only"),
	}
	if states, err := optionalStringSlice(input, "states"); err != nil {
		return nil, err
	} else {
		for _, s := range states {
			q.States = append(q.States, engine.TaskState(s))
		}
	}
	if tags, err := optionalStringSlice(input, "tags"); err != nil {
		return nil, err
	} else {
		q.Tags = tags
	}

	tasks, err := s.engine.QueryTasks(ctx, q)
	if err != nil {
		return nil, err
	}

	items := make([]any, 0, len(tasks))
	for i := range tasks {
		items = append(items, taskToMap(&tasks[i]))
	}
	enriched := s.enrichTasksWithBlockedBy(ctx, nsID, tasks)
	for i := range enriched {
		if i < len(items) {
			items[i] = enriched[i]
		}
	}

	return map[string]any{"tasks": items}, nil
}

func (s *Server) enrichTasksWithBlockedBy(ctx context.Context, nsID string, tasks []engine.Task) []map[string]any {
	out := make([]map[string]any, 0, len(tasks))
	for i := range tasks {
		m := taskToMap(&tasks[i])
		var blockedBy []string
		for _, dep := range tasks[i].DependsOn {
			depTask, err := s.engine.GetTask(ctx, nsID, dep)
			if err != nil {
				blockedBy = append(blockedBy, dep)
				continue
			}
			if depTask.State != engine.TaskDone {
				blockedBy = append(blockedBy, dep)
			}
		}
		if len(blockedBy) > 0 {
			m["blocked_by"] = blockedBy
		} else {
			m["blocked_by"] = []string{}
		}
		if w := tasks[i].AssignedWorker; w != "" {
			m["worker_status"] = string(s.engine.WorkerStatus(ctx, nsID, w))
		}
		out = append(out, m)
	}
	return out
}

func stringInputOrEmpty(input map[string]any, key string) string {
	if v, ok := input[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func intInputOrZero(input map[string]any, key string) int {
	if v, ok := input[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int32:
			return int(n)
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

func boolInputOrFalse(input map[string]any, key string) bool {
	if v, ok := input[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// DAG handlers
// ---------------------------------------------------------------------------

func (s *Server) handleDAGCreate(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	dagID, err := requiredString(input, "dag_id")
	if err != nil {
		return nil, err
	}
	title, err := requiredString(input, "title")
	if err != nil {
		return nil, err
	}
	branch, _ := optionalString(input, "branch")

	dag, err := s.engine.CreateDAG(ctx, engine.CreateDAGRequest{
		NamespaceID: nsID,
		ID:          dagID,
		Title:       title,
		Branch:      branch,
	})
	if err != nil {
		return nil, err
	}
	return dagToMap(dag), nil
}

func (s *Server) handleDAGGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	dagID, err := requiredString(input, "dag_id")
	if err != nil {
		return nil, err
	}
	withGraph, _ := optionalString(input, "with")
	if withGraph == "graph" {
		graph, err := s.engine.GetDAGGraph(ctx, nsID, dagID)
		if err != nil {
			return nil, err
		}
		return dagGraphToMap(graph), nil
	}
	dag, err := s.engine.GetDAG(ctx, nsID, dagID)
	if err != nil {
		return nil, err
	}
	return dagToMap(dag), nil
}

func (s *Server) handleDAGList(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	dags, err := s.engine.ListDAGs(ctx, nsID)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(dags))
	for i := range dags {
		items = append(items, dagToMap(&dags[i]))
	}
	return map[string]any{"dags": items}, nil
}

func (s *Server) handleDAGUpdate(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	dagID, err := requiredString(input, "dag_id")
	if err != nil {
		return nil, err
	}
	title, _ := optionalString(input, "title")
	branch, _ := optionalString(input, "branch")

	dag, err := s.engine.UpdateDAG(ctx, nsID, dagID, engine.UpdateDAGRequest{
		Title:  title,
		Branch: branch,
	})
	if err != nil {
		return nil, err
	}
	return dagToMap(dag), nil
}

// ---------------------------------------------------------------------------
// DAGReport handler
// ---------------------------------------------------------------------------

func (s *Server) handleDAGReport(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	dagID, err := requiredString(input, "dag_id")
	if err != nil {
		return nil, err
	}

	report, err := s.engine.DAGReport(ctx, nsID, dagID)
	if err != nil {
		return nil, err
	}

	workers := make([]any, 0, len(report.Workers))
	for _, w := range report.Workers {
		workers = append(workers, map[string]any{
			"worker_id":   w.WorkerID,
			"total_tasks": w.TotalTasks,
			"done_tasks":  w.DoneTasks,
			"status":      w.Status,
		})
	}

	return map[string]any{
		"dag":             dagToMap(&report.DAG),
		"total_tasks":     report.TotalTasks,
		"done_tasks":      report.DoneTasks,
		"executing_tasks": report.ExecutingTasks,
		"pending_tasks":   report.PendingTasks,
		"completion_pct":  report.CompletionPct,
		"workers":         workers,
	}, nil
}

// ---------------------------------------------------------------------------
// Worker handlers
// ---------------------------------------------------------------------------

func (s *Server) handleWorkerRegister(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	workerID, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	name, err := requiredString(input, "name")
	if err != nil {
		return nil, err
	}
	scope, _ := optionalString(input, "scope")
	skills, _ := optionalStringSlice(input, "skills")
	metadata, _ := optionalStringMap(input, "metadata")

	w, err := s.engine.RegisterWorker(ctx, engine.RegisterWorkerRequest{
		NamespaceID: nsID,
		ID:          workerID,
		Name:        name,
		Scope:       scope,
		Skills:      skills,
		Metadata:    metadata,
	})
	if err != nil {
		return nil, err
	}
	return workerToMap(w, ""), nil
}

func (s *Server) handleWorkerGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	workerID, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	w, err := s.engine.GetWorker(ctx, nsID, workerID)
	if err != nil {
		return nil, err
	}
	status := s.engine.WorkerStatus(ctx, nsID, workerID)
	return workerToMap(w, string(status)), nil
}

func (s *Server) handleWorkerList(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	workers, err := s.engine.ListWorkers(ctx, nsID)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(workers))
	for i := range workers {
		status := s.engine.WorkerStatus(ctx, nsID, workers[i].ID)
		items = append(items, workerToMap(&workers[i], string(status)))
	}
	return map[string]any{"workers": items}, nil
}

func (s *Server) handleWorkerUpdate(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	workerID, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	name, _ := optionalString(input, "name")
	scope, _ := optionalString(input, "scope")
	var skills []string
	if s, err := optionalStringSlice(input, "skills"); err == nil {
		skills = s
	}
	var metadata map[string]string
	if m, err := optionalStringMap(input, "metadata"); err == nil {
		metadata = m
	}

	w, err := s.engine.UpdateWorker(ctx, nsID, workerID, engine.UpdateWorkerRequest{
		Name:     name,
		Scope:    scope,
		Skills:   skills,
		Metadata: metadata,
	})
	if err != nil {
		return nil, err
	}
	status := s.engine.WorkerStatus(ctx, nsID, workerID)
	return workerToMap(w, string(status)), nil
}

func (s *Server) handleWorkerStatus(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	workerID, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	status := s.engine.WorkerStatus(ctx, nsID, workerID)
	return map[string]any{
		"namespace_id": nsID,
		"worker_id":    workerID,
		"status":       string(status),
		"checked_at":   time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

// ---------------------------------------------------------------------------
// Project-level handlers
// ---------------------------------------------------------------------------

func (s *Server) handleProjectNextTasks(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	tasks, err := s.engine.ProjectNextTasks(ctx, nsID)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, map[string]any{
			"task_id":         t.TaskID,
			"title":           t.Title,
			"dag_id":          t.DAGID,
			"assigned_worker": t.AssignedWorker,
			"state":           t.State,
			"deps_satisfied":  t.DepsSatisfied,
			"worker_busy":     t.WorkerBusy,
			"ready":           t.Ready,
			"reason":          t.Reason,
		})
	}
	return map[string]any{"tasks": items}, nil
}

func (s *Server) handleProjectBlockers(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	blockers, err := s.engine.ProjectBlockers(ctx, nsID)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(blockers))
	for _, b := range blockers {
		items = append(items, map[string]any{
			"task_id":    b.TaskID,
			"title":      b.Title,
			"dag_id":     b.DAGID,
			"type":       b.Type,
			"blocked_by": b.BlockedBy,
		})
	}
	return map[string]any{"blockers": items}, nil
}

// ---------------------------------------------------------------------------
// Map helpers
// ---------------------------------------------------------------------------

func dagToMap(dag *engine.DAG) map[string]any {
	return map[string]any{
		"id":          dag.ID,
		"namespace_id": dag.NamespaceID,
		"title":       dag.Title,
		"branch":      dag.Branch,
		"status":      string(dag.Status),
		"created_at":  dag.CreatedAt,
		"updated_at":  dag.UpdatedAt,
	}
}

func dagGraphToMap(g *engine.DAGGraph) map[string]any {
	nodes := make([]any, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodes = append(nodes, map[string]any{
			"task_id":         n.TaskID,
			"title":           n.Title,
			"state":           string(n.State),
			"assigned_worker": n.AssignedWorker,
			"depends_on":      n.DependsOn,
		})
	}
	edges := make([]any, 0, len(g.Edges))
	for _, e := range g.Edges {
		edges = append(edges, map[string]any{
			"from": e.FromTaskID,
			"to":   e.ToTaskID,
		})
	}
	workers := make([]any, 0, len(g.Workers))
	for _, w := range g.Workers {
		workers = append(workers, map[string]any{
			"worker_id": w.WorkerID,
			"tasks":     w.Tasks,
		})
	}
	tasks := make([]any, 0, len(g.Tasks))
	for i := range g.Tasks {
		tasks = append(tasks, taskToMap(&g.Tasks[i]))
	}
	return map[string]any{
		"dag":     dagToMap(&g.DAG),
		"tasks":   tasks,
		"nodes":   nodes,
		"edges":   edges,
		"workers": workers,
	}
}

func workerToMap(w *engine.Worker, status string) map[string]any {
	m := map[string]any{
		"id":           w.ID,
		"namespace_id": w.NamespaceID,
		"name":         w.Name,
		"scope":        w.Scope,
		"skills":       w.Skills,
		"created_at":   w.CreatedAt,
		"updated_at":   w.UpdatedAt,
	}
	if len(w.Metadata) > 0 {
		m["metadata"] = w.Metadata
	}
	if status != "" {
		m["status"] = status
	}
	return m
}
