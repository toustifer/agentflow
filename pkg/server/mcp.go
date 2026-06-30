package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/toustifer/agentflow/pkg/engine"
)

type ToolSpec struct {
	Name string `json:"name"`
}

func (s *Server) Tools() []ToolSpec {
	return []ToolSpec{
		{Name: "namespace_create"},
		{Name: "namespace_list"},
		{Name: "task_create"},
		{Name: "task_transition"},
		{Name: "task_get"},
		{Name: "task_list"},
		{Name: "task_history"},
		{Name: "flow_ping"},
	}
}

func (s *Server) Handle(ctx context.Context, tool string, input map[string]any) (map[string]any, error) {
	switch tool {
	case "namespace_create":
		result, err := s.handleNamespaceCreate(ctx, input)
		if err != nil {
			return nil, err
		}
		s.syncNamespace(ctx, result.namespace)
		return result.payload, nil
	case "namespace_list":
		return s.handleNamespaceList(ctx)
	case "task_create":
		result, err := s.handleTaskCreate(ctx, input)
		if err != nil {
			return nil, err
		}
		s.syncTask(ctx, result.task)
		return result.payload, nil
	case "task_transition":
		result, err := s.handleTaskTransition(ctx, input)
		if err != nil {
			return nil, err
		}
		s.syncTask(ctx, result.task)
		return result.payload, nil
	case "task_get":
		result, err := s.handleTaskGet(ctx, input)
		if err != nil {
			return nil, err
		}
		s.syncTask(ctx, result.task)
		return result.payload, nil
	case "task_list":
		result, err := s.handleTaskList(ctx, input)
		if err != nil {
			return nil, err
		}
		for i := range result.tasks {
			s.syncTask(ctx, &result.tasks[i])
		}
		return result.payload, nil
	case "task_history":
		result, err := s.handleTaskHistory(ctx, input)
		if err != nil {
			return nil, err
		}
		s.syncTask(ctx, result.task)
		return result.payload, nil
	case "flow_ping":
		result := map[string]any{"ok": true}
		s.syncPing(ctx)
		return result, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownTool, tool)
	}
}

type namespaceCreateResult struct {
	namespace *engine.Namespace
	payload   map[string]any
}

type taskResult struct {
	task    *engine.Task
	payload map[string]any
}

type taskListResult struct {
	tasks   []engine.Task
	payload map[string]any
}

type taskHistoryResult struct {
	task    *engine.Task
	payload map[string]any
}

func (s *Server) handleNamespaceCreate(ctx context.Context, input map[string]any) (namespaceCreateResult, error) {
	req, err := decodeCreateNamespaceRequest(input)
	if err != nil {
		return namespaceCreateResult{}, err
	}

	ns, err := s.engine.CreateNamespace(ctx, req)
	if err != nil {
		return namespaceCreateResult{}, err
	}

	return namespaceCreateResult{
		namespace: ns,
		payload:   namespaceToMap(ns),
	}, nil
}

func (s *Server) handleNamespaceList(ctx context.Context) (map[string]any, error) {
	namespaces, err := s.engine.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]any, 0, len(namespaces))
	for i := range namespaces {
		items = append(items, namespaceToMap(&namespaces[i]))
	}

	return map[string]any{"namespaces": items}, nil
}

func (s *Server) handleTaskCreate(ctx context.Context, input map[string]any) (taskResult, error) {
	req, err := decodeCreateTaskRequest(input)
	if err != nil {
		return taskResult{}, err
	}

	task, err := s.engine.CreateTask(ctx, req)
	if err != nil {
		return taskResult{}, err
	}

	return taskResult{
		task:    task,
		payload: taskToMap(task),
	}, nil
}

func (s *Server) handleTaskTransition(ctx context.Context, input map[string]any) (taskResult, error) {
	namespaceID, err := requiredString(input, "namespace_id")
	if err != nil {
		return taskResult{}, err
	}
	taskID, err := requiredString(input, "task_id")
	if err != nil {
		return taskResult{}, err
	}
	transitionValue, err := requiredString(input, "transition")
	if err != nil {
		return taskResult{}, err
	}
	actorRole, _ := optionalString(input, "actor_role")
	metadata, err := optionalStringMap(input, "metadata")
	if err != nil {
		return taskResult{}, err
	}
	if metadata == nil {
		metadata = make(map[string]string)
	}
	if actorRole != "" {
		metadata["actor_role"] = actorRole
	}

	task, err := s.engine.TransitionTask(ctx, namespaceID, taskID, engine.TaskTransition(transitionValue), metadata)
	if err != nil {
		return taskResult{}, err
	}

	return taskResult{
		task:    task,
		payload: taskToMap(task),
	}, nil
}

func (s *Server) handleTaskGet(ctx context.Context, input map[string]any) (taskResult, error) {
	namespaceID, err := requiredString(input, "namespace_id")
	if err != nil {
		return taskResult{}, err
	}
	taskID, err := requiredString(input, "task_id")
	if err != nil {
		return taskResult{}, err
	}

	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return taskResult{}, err
	}

	return taskResult{
		task:    task,
		payload: taskToMap(task),
	}, nil
}

func (s *Server) handleTaskList(ctx context.Context, input map[string]any) (taskListResult, error) {
	namespaceID, err := requiredString(input, "namespace_id")
	if err != nil {
		return taskListResult{}, err
	}
	states, err := optionalStringSlice(input, "states")
	if err != nil {
		return taskListResult{}, err
	}

	filter := engine.StateFilter{States: make(map[engine.TaskState]bool, len(states))}
	for _, state := range states {
		filter.States[engine.TaskState(state)] = true
	}
	if len(filter.States) == 0 {
		filter.States = nil
	}

	tasks, err := s.engine.ListTasks(ctx, namespaceID, filter)
	if err != nil {
		return taskListResult{}, err
	}

	items := make([]any, 0, len(tasks))
	for i := range tasks {
		items = append(items, taskToMap(&tasks[i]))
	}

	return taskListResult{
		tasks:   tasks,
		payload: map[string]any{"tasks": items},
	}, nil
}

func (s *Server) handleTaskHistory(ctx context.Context, input map[string]any) (taskHistoryResult, error) {
	namespaceID, err := requiredString(input, "namespace_id")
	if err != nil {
		return taskHistoryResult{}, err
	}
	taskID, err := requiredString(input, "task_id")
	if err != nil {
		return taskHistoryResult{}, err
	}

	history, err := s.engine.GetHistory(ctx, namespaceID, taskID)
	if err != nil {
		return taskHistoryResult{}, err
	}

	items := make([]any, 0, len(history))
	for i := range history {
		items = append(items, eventToMap(history[i]))
	}

	// best-effort: fetch task for hub sync (ignore error, syncTask handles nil)
	task, _ := s.engine.GetTask(ctx, namespaceID, taskID)

	return taskHistoryResult{
		task:    task,
		payload: map[string]any{"history": items},
	}, nil
}

func decodeCreateTaskRequest(input map[string]any) (engine.CreateTaskRequest, error) {
	var req engine.CreateTaskRequest

	namespaceID, err := requiredString(input, "namespace_id")
	if err != nil {
		return req, err
	}
	taskID, err := requiredString(input, "task_id")
	if err != nil {
		return req, err
	}
	title, err := requiredString(input, "title")
	if err != nil {
		return req, err
	}
	assignedWorker, err := optionalString(input, "assigned_worker")
	if err != nil {
		return req, err
	}
	description, err := optionalString(input, "description")
	if err != nil {
		return req, err
	}
	acceptanceCriteria, err := optionalStringSlice(input, "acceptance_criteria")
	if err != nil {
		return req, err
	}
	outputFiles, err := optionalStringSlice(input, "output_files")
	if err != nil {
		return req, err
	}
	metadata, err := optionalStringMap(input, "metadata")
	if err != nil {
		return req, err
	}

	return engine.CreateTaskRequest{
		NamespaceID:        namespaceID,
		ID:                 taskID,
		Title:              title,
		Description:        description,
		AssignedWorker:     assignedWorker,
		AcceptanceCriteria: acceptanceCriteria,
		OutputFiles:        outputFiles,
		Metadata:           metadata,
	}, nil
}

func decodeCreateNamespaceRequest(input map[string]any) (engine.CreateNamespaceRequest, error) {
	var req engine.CreateNamespaceRequest

	id, err := requiredString(input, "id")
	if err != nil {
		return req, err
	}
	name, err := requiredString(input, "name")
	if err != nil {
		return req, err
	}
	metadata, err := optionalStringMap(input, "metadata")
	if err != nil {
		return req, err
	}

	return engine.CreateNamespaceRequest{
		ID:       id,
		Name:     name,
		Metadata: metadata,
	}, nil
}

func requiredString(input map[string]any, key string) (string, error) {
	value, ok := input[key]
	if !ok {
		return "", fmt.Errorf("%w: missing %s", ErrInvalidToolInput, key)
	}

	stringValue, ok := value.(string)
	if !ok || strings.TrimSpace(stringValue) == "" {
		return "", fmt.Errorf("%w: %s must be a non-empty string", ErrInvalidToolInput, key)
	}

	return stringValue, nil
}

func optionalString(input map[string]any, key string) (string, error) {
	value, ok := input[key]
	if !ok || value == nil {
		return "", nil
	}

	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s must be a string", ErrInvalidToolInput, key)
	}

	return stringValue, nil
}

func optionalStringSlice(input map[string]any, key string) ([]string, error) {
	value, ok := input[key]
	if !ok || value == nil {
		return nil, nil
	}

	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			stringItem, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%w: %s must be an array of strings", ErrInvalidToolInput, key)
			}
			out = append(out, stringItem)
		}
		return out, nil
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out, nil
	default:
		return nil, fmt.Errorf("%w: %s must be an array of strings", ErrInvalidToolInput, key)
	}
}

func optionalStringMap(input map[string]any, key string) (map[string]string, error) {
	value, ok := input[key]
	if !ok || value == nil {
		return nil, nil
	}

	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]string, len(typed))
		for mapKey, mapValue := range typed {
			stringValue, ok := mapValue.(string)
			if !ok {
				return nil, fmt.Errorf("%w: %s must be an object of strings", ErrInvalidToolInput, key)
			}
			out[mapKey] = stringValue
		}
		return out, nil
	case map[string]string:
		out := make(map[string]string, len(typed))
		for mapKey, mapValue := range typed {
			out[mapKey] = mapValue
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%w: %s must be an object of strings", ErrInvalidToolInput, key)
	}
}

func namespaceToMap(ns *engine.Namespace) map[string]any {
	return map[string]any{
		"id":         ns.ID,
		"name":       ns.Name,
		"created_at": ns.CreatedAt,
		"updated_at": ns.UpdatedAt,
		"metadata":   cloneStringMapAny(ns.Metadata),
	}
}

func taskToMap(task *engine.Task) map[string]any {
	m := map[string]any{
		"id":                    task.ID,
		"namespace_id":          task.NamespaceID,
		"title":                 task.Title,
		"description":           task.Description,
		"state":                 string(task.State),
		"assigned_worker":       task.AssignedWorker,
		"acceptance_criteria":   cloneStringsAny(task.AcceptanceCriteria),
		"output_files":          cloneStringsAny(task.OutputFiles),
		"worker_agent_id":       task.WorkerAgentID,
		"review_cycle":          task.ReviewCycle,
		"created_at":            task.CreatedAt,
		"updated_at":            task.UpdatedAt,
		"metadata":              cloneStringMapAny(task.Metadata),
		"available_transitions": engine.AvailableTransitions(task),
	}
	return m
}

func eventToMap(event engine.Event) map[string]any {
	return map[string]any{
		"task_id":     event.TaskID,
		"transition":  event.Transition,
		"from_state":  string(event.FromState),
		"to_state":    string(event.ToState),
		"timestamp":   event.Timestamp,
		"actor":       event.Actor,
		"reason":      event.Reason,
		"metadata":    cloneStringMapAny(event.Metadata),
	}
}

func cloneStringsAny(values []string) []any {
	if values == nil {
		return nil
	}
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = value
	}
	return out
}

func cloneStringMapAny(values map[string]string) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
