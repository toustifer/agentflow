package server

import (
	"context"
	"fmt"
	"sort"
	"strings"
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

func requiredList(input map[string]any, key string) ([]any, error) {
	v, ok := input[key]
	if !ok || v == nil {
		return nil, fmt.Errorf("%w: missing %s", ErrInvalidToolInput, key)
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %s must be an array", ErrInvalidToolInput, key)
	}
	return list, nil
}

func (s *Server) handleTaskCreateBatch(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	dagID, _ := optionalString(input, "dag_id")

	tasksRaw, err := requiredList(input, "tasks")
	if err != nil {
		return nil, err
	}

	var items []engine.BatchTaskItem
	for i, raw := range tasksRaw {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tasks[%d] is not an object", i)
		}
		taskID, err := requiredString(m, "task_id")
		if err != nil {
			return nil, fmt.Errorf("tasks[%d]: %w", i, err)
		}
		title, err := requiredString(m, "title")
		if err != nil {
			return nil, fmt.Errorf("tasks[%d]: %w", i, err)
		}
		worker, _ := optionalString(m, "assigned_worker")
		desc, _ := optionalString(m, "description")
		ac, _ := optionalStringSlice(m, "acceptance_criteria")
		of, _ := optionalStringSlice(m, "output_files")
		deps, _ := optionalStringSlice(m, "depends_on")
		tags, _ := optionalStringSlice(m, "tags")
		priority, _ := optionalInt(m, "priority")
		hours, _ := optionalFloat(m, "estimated_hours")
		meta, _ := optionalStringMap(m, "metadata")

		items = append(items, engine.BatchTaskItem{
			ID:                 taskID,
			Title:              title,
			Description:        desc,
			AssignedWorker:     worker,
			AcceptanceCriteria: ac,
			OutputFiles:        of,
			DependsOn:          deps,
			Tags:               tags,
			Priority:           priority,
			EstimatedHours:     hours,
			Metadata:           meta,
		})
	}

	result, err := s.engine.CreateTaskBatch(ctx, engine.CreateTaskBatchRequest{
		NamespaceID: nsID,
		DAGID:       dagID,
		Tasks:       items,
	})
	if err != nil {
		return nil, err
	}

	workdir := s.namespaceWorkdir(ctx, nsID)
	tasks := make([]any, 0, len(result.Created))
	syncNotes := make([]string, 0, len(result.Created))
	for _, t := range result.Created {
		m := taskToMap(t)
		note := softHubAfterTask(ctx, workdir, t, "", "")
		m["hub_task_sync"] = note
		syncNotes = append(syncNotes, note)
		tasks = append(tasks, m)
	}
	return map[string]any{
		"tasks":         tasks,
		"hub_task_sync": syncNotes,
	}, nil
}

// ---------------------------------------------------------------------------
// DAG handlers
// ---------------------------------------------------------------------------

func (s *Server) validateDAGBranches(ctx context.Context, nsID, executionBranch, baseBranch string) (string, error) {
	if strings.TrimSpace(executionBranch) == "" {
		return "", fmt.Errorf("execution_branch is required")
	}
	ns, err := s.engine.GetNamespace(ctx, nsID)
	if err != nil {
		return "", err
	}
	resolvedBase := baseBranch
	if resolvedBase == "" {
		if ns.Metadata != nil && ns.Metadata["git_main_branch"] != "" {
			resolvedBase = ns.Metadata["git_main_branch"]
		} else {
			resolvedBase = "main"
		}
	}
	if executionBranch == resolvedBase {
		return "", fmt.Errorf("execution_branch %q cannot equal base_branch %q", executionBranch, resolvedBase)
	}
	for _, reserved := range []string{"main", "master"} {
		if executionBranch == reserved {
			return "", fmt.Errorf("execution_branch %q is reserved; use a feature branch", executionBranch)
		}
	}
	return resolvedBase, nil
}

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
	executionBranch, _ := optionalString(input, "execution_branch")
	baseBranch, _ := optionalString(input, "base_branch")
	metadata, err := optionalStringMap(input, "metadata")
	if err != nil {
		return nil, err
	}
	if executionBranch == "" {
		executionBranch = branch
	}
	resolvedBase, err := s.validateDAGBranches(ctx, nsID, executionBranch, baseBranch)
	if err != nil {
		return nil, err
	}

	dag, err := s.engine.CreateDAG(ctx, engine.CreateDAGRequest{
		NamespaceID:     nsID,
		ID:              dagID,
		Title:           title,
		ExecutionBranch: executionBranch,
		BaseBranch:      resolvedBase,
		Metadata:        metadata,
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
	executionBranch, _ := optionalString(input, "execution_branch")
	baseBranch, _ := optionalString(input, "base_branch")
	metadata, err := optionalStringMap(input, "metadata")
	if err != nil {
		return nil, err
	}
	if executionBranch == "" {
		executionBranch = branch
	}
	if executionBranch == "" {
		existing, err := s.engine.GetDAG(ctx, nsID, dagID)
		if err != nil {
			return nil, err
		}
		executionBranch = existing.ExecutionBranch
		if baseBranch == "" {
			baseBranch = existing.BaseBranch
		}
	}
	resolvedBase, err := s.validateDAGBranches(ctx, nsID, executionBranch, baseBranch)
	if err != nil {
		return nil, err
	}

	dag, err := s.engine.UpdateDAG(ctx, nsID, dagID, engine.UpdateDAGRequest{
		Title:           title,
		ExecutionBranch: executionBranch,
		BaseBranch:      resolvedBase,
		Metadata:        metadata,
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

func (s *Server) handleDAGFlowchart(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	dagID, err := requiredString(input, "dag_id")
	if err != nil {
		return nil, err
	}
	chart, err := s.engine.DAGFlowchart(ctx, nsID, dagID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"flowchart": chart}, nil
}

func (s *Server) handleHandbookWrite(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	wid, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	scope, _ := optionalString(input, "scope")
	techStack, _ := optionalStringSlice(input, "tech_stack")

	var knowledge []engine.KnowledgeItem
	if raw, err := optionalList(input, "knowledge"); err == nil {
		for _, r := range raw {
			if m, ok := r.(map[string]any); ok {
				knowledge = append(knowledge, engine.KnowledgeItem{
					Topic:   stringField(m, "topic"),
					Content: stringField(m, "content"),
					Tags:    stringSliceField(m, "tags"),
					Source:  stringField(m, "source"),
				})
			}
		}
	}

	var pitfalls []engine.PitfallItem
	if raw, err := optionalList(input, "pitfalls"); err == nil {
		for _, r := range raw {
			if m, ok := r.(map[string]any); ok {
				pitfalls = append(pitfalls, engine.PitfallItem{
					Scenario: stringField(m, "scenario"),
					Problem:  stringField(m, "problem"),
					Solution: stringField(m, "solution"),
					Tags:     stringSliceField(m, "tags"),
					Source:   stringField(m, "source"),
				})
			}
		}
	}

	hb, err := s.engine.WriteHandbook(ctx, engine.WriteHandbookRequest{
		NamespaceID: nsID,
		WorkerID:    wid,
		Scope:       scope,
		TechStack:   techStack,
		Knowledge:   knowledge,
		Pitfalls:    pitfalls,
	})
	if err != nil {
		return nil, err
	}
	s.bestEffortMirrorDocs(ctx, nsID)
	return handbookToMap(hb), nil
}

func (s *Server) handleHandbookGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	wid, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	hb, err := s.engine.GetHandbook(ctx, nsID, wid)
	if err != nil {
		return nil, err
	}
	return handbookToMap(hb), nil
}

func (s *Server) handleHandbookList(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	hbs, err := s.engine.ListHandbooks(ctx, nsID)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(hbs))
	for _, hb := range hbs {
		items = append(items, handbookToMap(&hb))
	}
	return map[string]any{"handbooks": items}, nil
}

func (s *Server) handleFindKnowledge(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	query, err := requiredString(input, "query")
	if err != nil {
		return nil, err
	}
	results, err := s.engine.FindKnowledge(ctx, nsID, query)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"worker_id": r.WorkerID,
			"topic":     r.Item.Topic,
			"content":   r.Item.Content,
			"tags":      r.Item.Tags,
			"source":    r.Item.Source,
		})
	}
	return map[string]any{"results": items}, nil
}

func (s *Server) handleFindPitfalls(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	query, err := requiredString(input, "query")
	if err != nil {
		return nil, err
	}
	results, err := s.engine.FindPitfalls(ctx, nsID, query)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(results))
	for _, r := range results {
		items = append(items, map[string]any{
			"worker_id": r.WorkerID,
			"scenario":  r.Item.Scenario,
			"problem":   r.Item.Problem,
			"solution":  r.Item.Solution,
			"tags":      r.Item.Tags,
			"source":    r.Item.Source,
		})
	}
	return map[string]any{"results": items}, nil
}

func (s *Server) handleWorkerDiaryWrite(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	wid, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	date, err := requiredString(input, "date")
	if err != nil {
		return nil, err
	}
	content, err := requiredString(input, "content")
	if err != nil {
		return nil, err
	}
	taskID, _ := optionalString(input, "task_id")
	tags, _ := optionalStringSlice(input, "tags")

	d, err := s.engine.WriteWorkerDiary(ctx, engine.WriteDiaryRequest{
		NamespaceID: nsID,
		WorkerID:    wid,
		Date:        date,
		TaskID:      taskID,
		Content:     content,
		Tags:        tags,
	})
	if err != nil {
		return nil, err
	}
	s.bestEffortMirrorDocs(ctx, nsID)
	return workerDiaryToMap(d), nil
}

func (s *Server) handleWorkerDiaryGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	wid, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	date, err := requiredString(input, "date")
	if err != nil {
		return nil, err
	}
	d, err := s.engine.GetWorkerDiary(ctx, nsID, wid, date)
	if err != nil {
		return nil, err
	}
	return workerDiaryToMap(d), nil
}

func (s *Server) handleWorkerDiaryList(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	wid, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	diaries, err := s.engine.ListWorkerDiaries(ctx, nsID, wid)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(diaries))
	for _, d := range diaries {
		items = append(items, workerDiaryToMap(&d))
	}
	return map[string]any{"diaries": items}, nil
}

func (s *Server) handleLeaderDiaryWrite(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	date, err := requiredString(input, "date")
	if err != nil {
		return nil, err
	}
	entryType, _ := optionalString(input, "type")
	dagID, _ := optionalString(input, "dag_id")
	taskID, _ := optionalString(input, "task_id")
	title, _ := optionalString(input, "title")
	content, _ := optionalString(input, "content")
	tags, _ := optionalStringSlice(input, "tags")

	ld, err := s.engine.WriteLeaderDiary(ctx, engine.WriteLeaderDiaryRequest{
		NamespaceID: nsID,
		Date:        date,
		Entry: engine.DiaryEntry{
			Type:    entryType,
			DAGID:   dagID,
			TaskID:  taskID,
			Title:   title,
			Content: content,
			Tags:    tags,
		},
	})
	if err != nil {
		return nil, err
	}
	s.bestEffortMirrorDocs(ctx, nsID)
	return leaderDiaryToMap(ld), nil
}

func (s *Server) handleLeaderDiaryGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	date, err := requiredString(input, "date")
	if err != nil {
		return nil, err
	}
	ld, err := s.engine.GetLeaderDiary(ctx, nsID, date)
	if err != nil {
		return nil, err
	}
	return leaderDiaryToMap(ld), nil
}

func (s *Server) handleLeaderDiaryList(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	diaries, err := s.engine.ListLeaderDiaries(ctx, nsID)
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(diaries))
	for _, ld := range diaries {
		items = append(items, leaderDiaryToMap(&ld))
	}
	return map[string]any{"diaries": items}, nil
}

// ---------------------------------------------------------------------------
// Helper field readers
// ---------------------------------------------------------------------------

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func stringSliceField(m map[string]any, key string) []string {
	if raw, ok := m[key].([]any); ok {
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func optionalList(input map[string]any, key string) ([]any, error) {
	v, ok := input[key]
	if !ok || v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("field %s is not an array", key)
	}
	return list, nil
}

// ---------------------------------------------------------------------------
// Map helpers
// ---------------------------------------------------------------------------

func handbookToMap(hb *engine.WorkerHandbook) map[string]any {
	knowledge := make([]any, 0, len(hb.Knowledge))
	for _, k := range hb.Knowledge {
		knowledge = append(knowledge, map[string]any{
			"topic":   k.Topic,
			"content": k.Content,
			"tags":    k.Tags,
			"source":  k.Source,
		})
	}
	pitfalls := make([]any, 0, len(hb.Pitfalls))
	for _, p := range hb.Pitfalls {
		pitfalls = append(pitfalls, map[string]any{
			"scenario": p.Scenario,
			"problem":  p.Problem,
			"solution": p.Solution,
			"tags":     p.Tags,
			"source":   p.Source,
		})
	}
	m := map[string]any{
		"worker_id":    hb.WorkerID,
		"namespace_id": hb.NamespaceID,
		"scope":        hb.Scope,
		"tech_stack":   hb.TechStack,
		"knowledge":    knowledge,
		"pitfalls":     pitfalls,
		"created_at":   hb.CreatedAt,
		"updated_at":   hb.UpdatedAt,
	}
	return m
}

func workerDiaryToMap(d *engine.WorkerDiary) map[string]any {
	return map[string]any{
		"worker_id":    d.WorkerID,
		"namespace_id": d.NamespaceID,
		"date":         d.Date,
		"task_id":      d.TaskID,
		"content":      d.Content,
		"tags":         d.Tags,
		"created_at":   d.CreatedAt,
	}
}

func leaderDiaryToMap(ld *engine.LeaderDiary) map[string]any {
	entries := make([]any, 0, len(ld.Entries))
	for _, e := range ld.Entries {
		entries = append(entries, map[string]any{
			"type":      e.Type,
			"dag_id":    e.DAGID,
			"task_id":   e.TaskID,
			"title":     e.Title,
			"content":   e.Content,
			"tags":      e.Tags,
			"timestamp": e.Timestamp,
		})
	}
	return map[string]any{
		"namespace_id": ld.NamespaceID,
		"date":         ld.Date,
		"entries":      entries,
		"created_at":   ld.CreatedAt,
		"updated_at":   ld.UpdatedAt,
	}
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
	kind, _ := optionalString(input, "kind")
	skills, _ := optionalStringSlice(input, "skills")
	taskTags, _ := optionalStringSlice(input, "task_tags")
	requiredReads, _ := optionalStringSlice(input, "required_reads")
	recommendedMCP, _ := optionalStringSlice(input, "recommended_mcp")
	handoffTargets, _ := optionalStringSlice(input, "handoff_targets")
	recoveryPolicy, _ := optionalStringSlice(input, "recovery_policy")
	fallbackMCP, _ := optionalStringSlice(input, "fallback_mcp")
	stuckPlaybook, _ := optionalString(input, "stuck_playbook")
	escalationMode, _ := optionalString(input, "escalation_mode")
	launchMode, _ := optionalString(input, "launch_mode")
	metadata, _ := optionalStringMap(input, "metadata")
	promptTemplate, _ := optionalString(input, "prompt_template")
	if strings.TrimSpace(promptTemplate) == "" {
		return nil, fmt.Errorf("%w: prompt_template is required", ErrInvalidToolInput)
	}

	w, err := s.engine.RegisterWorker(ctx, engine.RegisterWorkerRequest{
		NamespaceID:    nsID,
		ID:             workerID,
		Name:           name,
		Kind:           kind,
		Scope:          scope,
		Skills:         skills,
		TaskTags:       taskTags,
		PromptTemplate: promptTemplate,
		RequiredReads:  requiredReads,
		RecommendedMCP: recommendedMCP,
		LaunchMode:     launchMode,
		HandoffTargets: handoffTargets,
		RecoveryPolicy: recoveryPolicy,
		FallbackMCP:    fallbackMCP,
		StuckPlaybook:  stuckPlaybook,
		EscalationMode: escalationMode,
		Metadata:       metadata,
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
	kind, _ := optionalString(input, "kind")
	scope, _ := optionalString(input, "scope")
	var skills []string
	if s, err := optionalStringSlice(input, "skills"); err == nil {
		skills = s
	}
	var taskTags []string
	if s, err := optionalStringSlice(input, "task_tags"); err == nil {
		taskTags = s
	}
	var requiredReads []string
	if s, err := optionalStringSlice(input, "required_reads"); err == nil {
		requiredReads = s
	}
	var recommendedMCP []string
	if s, err := optionalStringSlice(input, "recommended_mcp"); err == nil {
		recommendedMCP = s
	}
	var handoffTargets []string
	if s, err := optionalStringSlice(input, "handoff_targets"); err == nil {
		handoffTargets = s
	}
	var recoveryPolicy []string
	if s, err := optionalStringSlice(input, "recovery_policy"); err == nil {
		recoveryPolicy = s
	}
	var fallbackMCP []string
	if s, err := optionalStringSlice(input, "fallback_mcp"); err == nil {
		fallbackMCP = s
	}
	stuckPlaybook, _ := optionalString(input, "stuck_playbook")
	escalationMode, _ := optionalString(input, "escalation_mode")
	var metadata map[string]string
	if m, err := optionalStringMap(input, "metadata"); err == nil {
		metadata = m
	}
	promptTemplate, _ := optionalString(input, "prompt_template")
	launchMode, _ := optionalString(input, "launch_mode")

	w, err := s.engine.UpdateWorker(ctx, nsID, workerID, engine.UpdateWorkerRequest{
		Name:           name,
		Kind:           kind,
		Scope:          scope,
		Skills:         skills,
		TaskTags:       taskTags,
		PromptTemplate: promptTemplate,
		RequiredReads:  requiredReads,
		RecommendedMCP: recommendedMCP,
		LaunchMode:     launchMode,
		HandoffTargets: handoffTargets,
		RecoveryPolicy: recoveryPolicy,
		FallbackMCP:    fallbackMCP,
		StuckPlaybook:  stuckPlaybook,
		EscalationMode: escalationMode,
		Metadata:       metadata,
	})
	if err != nil {
		return nil, err
	}
	status := s.engine.WorkerStatus(ctx, nsID, workerID)
	return workerToMap(w, string(status)), nil
}

func (s *Server) handleWorkerPromptGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	wid, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	taskID, _ := optionalString(input, "task_id")
	title, _ := optionalString(input, "title")
	asReviewer := false
	if raw, ok := input["as_reviewer"]; ok {
		if v, ok := raw.(bool); ok {
			asReviewer = v
		}
	}

	prompt, err := s.engine.WorkerPromptGet(ctx, nsID, wid, taskID, title, asReviewer)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"worker_id": wid,
		"prompt":    prompt,
	}, nil
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

func (s *Server) handleGitStatus(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	ns, err := s.engine.GetNamespace(ctx, nsID)
	if err != nil {
		return nil, err
	}
	repoPath, err := validateGitRepo(ctx, ns.Metadata["workdir"])
	if err != nil {
		return nil, err
	}
	branch, err := runGit(ctx, repoPath, "branch", "--show-current")
	if err != nil {
		return nil, err
	}
	headSHA, err := runGit(ctx, repoPath, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	statusText, err := runGit(ctx, repoPath, "status", "--short")
	if err != nil {
		return nil, err
	}
	status := "clean"
	if strings.TrimSpace(statusText) != "" {
		status = "dirty"
	}
	result := map[string]any{
		"namespace_id": nsID,
		"repo_path":    repoPath,
		"branch":       branch,
		"head_sha":     headSHA,
		"status":       status,
		"workdir":      ns.Metadata["workdir"],
	}
	if taskID, _ := optionalString(input, "task_id"); taskID != "" {
		task, err := s.engine.GetTask(ctx, nsID, taskID)
		if err != nil {
			return nil, err
		}
		result["task_id"] = taskID
		result["task_metadata"] = cloneStringMapAny(task.Metadata)
	}
	return result, nil
}

func (s *Server) handleWorktreeGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	taskID, err := requiredString(input, "task_id")
	if err != nil {
		return nil, err
	}
	ns, err := s.engine.GetNamespace(ctx, nsID)
	if err != nil {
		return nil, err
	}
	task, err := s.engine.GetTask(ctx, nsID, taskID)
	if err != nil {
		return nil, err
	}
	var dag *engine.DAG
	if task.DAGID != "" {
		dag, err = s.engine.GetDAG(ctx, nsID, task.DAGID)
		if err != nil {
			return nil, err
		}
	}
	runtime, err := s.getTaskGitRuntime(ctx, ns, dag, task)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"namespace_id":           nsID,
		"task_id":                taskID,
		"dag_id":                 task.DAGID,
		"repo_path":              runtime.RepoPath,
		"worktree_path":          runtime.WorktreePath,
		"branch":                 runtime.Branch,
		"base_branch":            runtime.BaseBranch,
		"head_sha":               runtime.HeadSHA,
		"status":                 runtime.Status,
		"worktree_owner_scope":   "dag",
		"active_task_id":         dag.ActiveTaskID,
		"lease_holder_task_id":   dag.LeaseHolderTaskID,
		"lease_holder_worker_id": dag.LeaseHolderWorkerID,
		"lease_holder_agent_id":  dag.LeaseHolderAgentID,
		"metadata":               cloneStringMapAny(task.Metadata),
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
	hint := dagResumeHintFromMetadata(dag.Metadata)
	return map[string]any{
		"id":               dag.ID,
		"namespace_id":     dag.NamespaceID,
		"title":            dag.Title,
		"branch":           dag.ExecutionBranch,
		"execution_branch": dag.ExecutionBranch,
		"base_branch":      dag.BaseBranch,
		"status":           string(dag.Status),
		"legacy":           hint.Legacy,
		"resume_priority":  string(hint.ResumePriority),
		"superseded_by":    hint.SupersededBy,
		"created_at":       dag.CreatedAt,
		"updated_at":       dag.UpdatedAt,
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
		"id":              w.ID,
		"namespace_id":    w.NamespaceID,
		"name":            w.Name,
		"kind":            w.Kind,
		"scope":           w.Scope,
		"skills":          w.Skills,
		"task_tags":       w.TaskTags,
		"prompt_template": w.PromptTemplate,
		"required_reads":  w.RequiredReads,
		"recommended_mcp": w.RecommendedMCP,
		"launch_mode":     w.LaunchMode,
		"handoff_targets": w.HandoffTargets,
		"recovery_policy": w.RecoveryPolicy,
		"fallback_mcp":    w.FallbackMCP,
		"stuck_playbook":  w.StuckPlaybook,
		"escalation_mode": w.EscalationMode,
		"created_at":      w.CreatedAt,
		"updated_at":      w.UpdatedAt,
	}
	if len(w.Metadata) > 0 {
		m["metadata"] = w.Metadata
	}
	if status != "" {
		m["status"] = status
	}
	return m
}

func (s *Server) handleProjectReport(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	report, err := s.engine.ProjectReport(ctx, nsID)
	if err != nil {
		return nil, err
	}

	dags := make([]any, 0, len(report.DAGs))
	for _, d := range report.DAGs {
		workers := make([]any, 0, len(d.Workers))
		for _, w := range d.Workers {
			workers = append(workers, map[string]any{
				"worker_id":   w.WorkerID,
				"total_tasks": w.TotalTasks,
				"done_tasks":  w.DoneTasks,
				"status":      w.Status,
			})
		}
		dags = append(dags, map[string]any{
			"dag":             dagToMap(&d.DAG),
			"total_tasks":     d.TotalTasks,
			"done_tasks":      d.DoneTasks,
			"executing_tasks": d.ExecutingTasks,
			"pending_tasks":   d.PendingTasks,
			"completion_pct":  d.CompletionPct,
			"workers":         workers,
		})
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
		"total_dags":      report.TotalDAGs,
		"total_tasks":     report.TotalTasks,
		"done_tasks":      report.DoneTasks,
		"executing_tasks": report.ExecutingTasks,
		"pending_tasks":   report.PendingTasks,
		"completion_pct":  report.CompletionPct,
		"dags":            dags,
		"workers":         workers,
	}, nil
}

func (s *Server) handleProjectInspect(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	focus, _ := optionalString(input, "focus")
	dagID, _ := optionalString(input, "dag_id")
	taskID, _ := optionalString(input, "task_id")

	ns, err := s.engine.GetNamespace(ctx, nsID)
	if err != nil {
		return nil, err
	}
	nextStepsInput := map[string]any{"namespace_id": nsID}
	if dagID != "" {
		nextStepsInput["dag_id"] = dagID
	}
	steps, err := s.handleProjectNextSteps(ctx, nextStepsInput)
	if err != nil {
		return nil, err
	}
	report, err := s.engine.ProjectReport(ctx, nsID)
	if err != nil {
		return nil, err
	}
	next, err := s.engine.ProjectNextTasks(ctx, nsID)
	if err != nil {
		return nil, err
	}
	blockers, err := s.engine.ProjectBlockers(ctx, nsID)
	if err != nil {
		return nil, err
	}
	workers, err := s.engine.ListWorkers(ctx, nsID)
	if err != nil {
		return nil, err
	}
	dags, err := s.engine.ListDAGs(ctx, nsID)
	if err != nil {
		return nil, err
	}
	tasks, err := s.engine.ListTasks(ctx, nsID, engine.StateFilter{})
	if err != nil {
		return nil, err
	}

	project := map[string]any{
		"namespace_id":   nsID,
		"namespace_name": ns.Name,
		"workdir":        getWorkdir(ns),
		"phase":          steps["phase"],
		"phase_name":     steps["phase_name"],
		"progress":       steps["progress"],
		"total_dags":     report.TotalDAGs,
		"total_tasks":    report.TotalTasks,
		"done_tasks":     report.DoneTasks,
		"running_tasks":  report.ExecutingTasks,
		"pending_tasks":  report.PendingTasks,
		"completion_pct": report.CompletionPct,
	}

	workerItems := make([]any, 0, len(workers))
	busyWorkers := 0
	for i := range workers {
		status := string(s.engine.WorkerStatus(ctx, nsID, workers[i].ID))
		if status == string(engine.WorkerBusy) {
			busyWorkers++
		}
		workerItems = append(workerItems, workerToMap(&workers[i], status))
	}
	sort.Slice(workerItems, func(i, j int) bool {
		left := workerItems[i].(map[string]any)
		right := workerItems[j].(map[string]any)
		return fmt.Sprint(left["id"]) < fmt.Sprint(right["id"])
	})

	readyByDAG := make(map[string][]map[string]any)
	blockedByDAG := make(map[string][]map[string]any)
	for _, item := range next {
		entry := map[string]any{
			"task_id":         item.TaskID,
			"title":           item.Title,
			"dag_id":          item.DAGID,
			"assigned_worker": item.AssignedWorker,
			"state":           item.State,
			"worker_busy":     item.WorkerBusy,
			"deps_satisfied":  item.DepsSatisfied,
			"reason":          item.Reason,
		}
		if item.Ready {
			readyByDAG[item.DAGID] = append(readyByDAG[item.DAGID], entry)
		} else {
			blockedByDAG[item.DAGID] = append(blockedByDAG[item.DAGID], entry)
		}
	}

	runningByDAG := make(map[string][]map[string]any)
	doneByDAG := make(map[string][]map[string]any)
	allByID := make(map[string]engine.Task, len(tasks))
	for i := range tasks {
		t := tasks[i]
		allByID[t.ID] = t
		entry := map[string]any{
			"task_id":         t.ID,
			"title":           t.Title,
			"dag_id":          t.DAGID,
			"assigned_worker": t.AssignedWorker,
			"state":           string(t.State),
		}
		switch t.State {
		case engine.TaskExecuting, engine.TaskReviewPending, engine.TaskReworkNeeded:
			runningByDAG[t.DAGID] = append(runningByDAG[t.DAGID], entry)
		case engine.TaskDone:
			doneByDAG[t.DAGID] = append(doneByDAG[t.DAGID], entry)
		}
	}

	blockerItems := make([]any, 0, len(blockers))
	for _, b := range blockers {
		blockerItems = append(blockerItems, map[string]any{
			"task_id":    b.TaskID,
			"title":      b.Title,
			"dag_id":     b.DAGID,
			"type":       b.Type,
			"blocked_by": b.BlockedBy,
		})
	}

	dagItems := make([]any, 0, len(report.DAGs))
	for _, d := range report.DAGs {
		item := map[string]any{
			"dag":             dagToMap(&d.DAG),
			"total_tasks":     d.TotalTasks,
			"done_tasks":      d.DoneTasks,
			"running_tasks":   d.ExecutingTasks,
			"pending_tasks":   d.PendingTasks,
			"completion_pct":  d.CompletionPct,
			"ready_tasks":     readyByDAG[d.DAG.ID],
			"blocked_tasks":   blockedByDAG[d.DAG.ID],
			"active_tasks":    runningByDAG[d.DAG.ID],
			"done_task_items": doneByDAG[d.DAG.ID],
		}
		dagItems = append(dagItems, item)
	}
	sort.Slice(dagItems, func(i, j int) bool {
		left := dagItems[i].(map[string]any)["dag"].(map[string]any)
		right := dagItems[j].(map[string]any)["dag"].(map[string]any)
		return fmt.Sprint(left["id"]) < fmt.Sprint(right["id"])
	})

	readyCount := len(flattenTaskBuckets(readyByDAG))
	runningCount := len(flattenTaskBuckets(runningByDAG))
	blockedCount := len(flattenTaskBuckets(blockedByDAG))
	doneCount := report.DoneTasks
	nextTaskItems := make([]any, 0)
	if dagID != "" {
		// Targeted inspect: counts and next queue follow the focused DAG.
		readyCount = len(readyByDAG[dagID])
		runningCount = len(runningByDAG[dagID])
		blockedCount = len(blockedByDAG[dagID])
		doneCount = len(doneByDAG[dagID])
		for _, item := range readyByDAG[dagID] {
			nextTaskItems = append(nextTaskItems, item)
		}
	} else {
		for _, bucket := range readyByDAG {
			for _, item := range bucket {
				nextTaskItems = append(nextTaskItems, item)
			}
		}
	}

	response := map[string]any{
		"focus":   focus,
		"project": project,
		"summary": map[string]any{
			"phase":         steps["phase"],
			"phase_name":    steps["phase_name"],
			"progress":      steps["progress"],
			"ready_count":   readyCount,
			"running_count": runningCount,
			"blocked_count": blockedCount,
			"done_count":    doneCount,
			"worker_busy":   busyWorkers,
			"worker_total":  len(workers),
		},
		"dags":       dagItems,
		"blockers":   blockerItems,
		"workers":    workerItems,
		"next_tasks": nextTaskItems,
	}

	if dagID != "" {
		graph, err := s.engine.GetDAGGraph(ctx, nsID, dagID)
		if err != nil {
			return nil, err
		}
		response["dag_detail"] = dagGraphToMap(graph)
	}
	if taskID != "" {
		task, err := s.engine.GetTask(ctx, nsID, taskID)
		if err != nil {
			return nil, err
		}
		taskDetail := taskToMap(task)
		var blockedBy []string
		for _, dep := range task.DependsOn {
			depTask, ok := allByID[dep]
			if !ok || depTask.State != engine.TaskDone {
				blockedBy = append(blockedBy, dep)
			}
		}
		taskDetail["blocked_by"] = blockedBy
		taskDetail["worker_status"] = string(s.engine.WorkerStatus(ctx, nsID, task.AssignedWorker))
		response["task_detail"] = taskDetail
		if task.DAGID != "" {
			worktree, err := s.handleWorktreeGet(ctx, map[string]any{"namespace_id": nsID, "task_id": taskID})
			if err == nil {
				response["task_runtime"] = worktree
			}
		}
	}

	for i := range dags {
		if dagID != "" && dags[i].ID == dagID {
			response["focused_dag"] = dagToMap(&dags[i])
			break
		}
	}
	return response, nil
}

func flattenTaskBuckets(buckets map[string][]map[string]any) []map[string]any {
	out := make([]map[string]any, 0)
	for _, items := range buckets {
		out = append(out, items...)
	}
	return out
}
