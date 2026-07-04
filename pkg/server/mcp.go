package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/toustifer/agentflow/pkg/engine"
)

type ToolSpec struct {
	Name string `json:"name"`
}

func (s *Server) Tools() []ToolSpec {
	return []ToolSpec{
		{Name: "namespace_create"},
		{Name: "namespace_get"},
		{Name: "namespace_delete"},
		{Name: "namespace_list"},
		{Name: "task_create"},
		{Name: "task_transition"},
		{Name: "task_get"},
		{Name: "task_list"},
		{Name: "task_history"},
		{Name: "task_query"},
		{Name: "task_create_batch"},
		{Name: "dag_create"},
		{Name: "dag_get"},
		{Name: "dag_list"},
		{Name: "dag_update"},
		{Name: "dag_report"},
		{Name: "dag_flowchart"},
		{Name: "worker_register"},
		{Name: "worker_get"},
		{Name: "worker_list"},
		{Name: "worker_update"},
		{Name: "worker_status"},
		{Name: "worker_prompt_get"},
		{Name: "git_status"},
		{Name: "worktree_get"},
		{Name: "project_next_tasks"},
		{Name: "leader_tick"},
		{Name: "bt_list_trees"},
		{Name: "bt_show_tree"},
		{Name: "bt_validate_tree"},
		{Name: "bt_tick"},
		{Name: "project_init"},
		{Name: "project_next_steps"},
		{Name: "project_blockers"},
		{Name: "project_report"},
		{Name: "worker_handbook_write"},
		{Name: "worker_handbook_get"},
		{Name: "worker_handbook_list"},
		{Name: "find_knowledge"},
		{Name: "find_pitfalls"},
		{Name: "worker_diary_write"},
		{Name: "worker_diary_get"},
		{Name: "worker_diary_list"},
		{Name: "leader_diary_write"},
		{Name: "leader_diary_get"},
		{Name: "leader_diary_list"},
		{Name: "doc_write"},
		{Name: "doc_get"},
		{Name: "doc_list"},
		{Name: "doc_search"},
		{Name: "doc_delete"},
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
		return s.handleNamespaceList(ctx, input)
	case "namespace_get":
		return s.handleNamespaceGet(ctx, input)
	case "namespace_delete":
		return s.handleNamespaceDelete(ctx, input)
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
	case "task_query":
		return s.handleTaskQuery(ctx, input)
	case "task_create_batch":
		return s.handleTaskCreateBatch(ctx, input)
	case "dag_create":
		return s.handleDAGCreate(ctx, input)
	case "dag_get":
		return s.handleDAGGet(ctx, input)
	case "dag_list":
		return s.handleDAGList(ctx, input)
	case "dag_update":
		return s.handleDAGUpdate(ctx, input)
	case "worker_register":
		return s.handleWorkerRegister(ctx, input)
	case "worker_get":
		return s.handleWorkerGet(ctx, input)
	case "worker_list":
		return s.handleWorkerList(ctx, input)
	case "worker_update":
		return s.handleWorkerUpdate(ctx, input)
	case "worker_status":
		return s.handleWorkerStatus(ctx, input)
	case "worker_prompt_get":
		return s.handleWorkerPromptGet(ctx, input)
	case "git_status":
		return s.handleGitStatus(ctx, input)
	case "worktree_get":
		return s.handleWorktreeGet(ctx, input)
	case "dag_report":
		return s.handleDAGReport(ctx, input)
	case "dag_flowchart":
		return s.handleDAGFlowchart(ctx, input)
	case "project_next_tasks":
		return s.handleProjectNextTasks(ctx, input)
	case "leader_tick":
		return s.handleLeaderTick(ctx, input)
	case "bt_list_trees":
		return s.handleBTListTrees(ctx, input)
	case "bt_show_tree":
		return s.handleBTShowTree(ctx, input)
	case "bt_validate_tree":
		return s.handleBTValidateTree(ctx, input)
	case "bt_tick":
		return s.handleBTTick(ctx, input)
	case "project_init":
		result, err := s.handleProjectInit(ctx, input)
		if err != nil {
			return nil, err
		}
		s.syncNamespace(ctx, result.namespace)
		return result.payload, nil
	case "project_blockers":
		return s.handleProjectBlockers(ctx, input)
	case "project_report":
		return s.handleProjectReport(ctx, input)
	case "worker_handbook_write":
		return s.handleHandbookWrite(ctx, input)
	case "worker_handbook_get":
		return s.handleHandbookGet(ctx, input)
	case "worker_handbook_list":
		return s.handleHandbookList(ctx, input)
	case "find_knowledge":
		return s.handleFindKnowledge(ctx, input)
	case "find_pitfalls":
		return s.handleFindPitfalls(ctx, input)
	case "worker_diary_write":
		return s.handleWorkerDiaryWrite(ctx, input)
	case "worker_diary_get":
		return s.handleWorkerDiaryGet(ctx, input)
	case "worker_diary_list":
		return s.handleWorkerDiaryList(ctx, input)
	case "leader_diary_write":
		return s.handleLeaderDiaryWrite(ctx, input)
	case "leader_diary_get":
		return s.handleLeaderDiaryGet(ctx, input)
	case "leader_diary_list":
		return s.handleLeaderDiaryList(ctx, input)
	case "doc_write":
		return s.handleDocWrite(ctx, input)
	case "doc_get":
		return s.handleDocGet(ctx, input)
	case "doc_list":
		return s.handleDocList(ctx, input)
	case "doc_search":
		return s.handleDocSearch(ctx, input)
	case "doc_delete":
		return s.handleDocDelete(ctx, input)
	case "project_next_steps":
		return s.handleProjectNextSteps(ctx, input)
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

	// 检查 workdir 冲突（P1）：同一 workdir 不能绑到两个 namespace
	if req.Metadata != nil {
		if wd, ok := req.Metadata["workdir"]; ok && wd != "" {
			allNS, err := s.engine.ListNamespaces(ctx)
			if err == nil {
				for i := range allNS {
					ns := &allNS[i]
					if ns.Metadata != nil {
						if w, exists := ns.Metadata["workdir"]; exists && strings.EqualFold(w, wd) {
							return namespaceCreateResult{}, fmt.Errorf(
								"workdir %q 已被 namespace %q (%s) 绑定，不能重复创建。如需切换项目请使用已有 namespace，或清理后重试",
								wd, ns.ID, ns.Name,
							)
						}
					}
				}
			}
		}
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

func (s *Server) handleNamespaceList(ctx context.Context, input map[string]any) (map[string]any, error) {
	namespaces, err := s.engine.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	workdirContains, _ := optionalString(input, "workdir_contains")

	items := make([]any, 0, len(namespaces))
	for i := range namespaces {
		ns := &namespaces[i]
		if workdirContains != "" {
			wd := ""
			if ns.Metadata != nil {
				wd = ns.Metadata["workdir"]
			}
			if !strings.Contains(strings.ToLower(wd), strings.ToLower(workdirContains)) {
				continue
			}
		}
		items = append(items, namespaceToMap(ns))
	}

	return map[string]any{"namespaces": items}, nil
}

func (s *Server) handleNamespaceGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	ns, err := s.engine.GetNamespace(ctx, nsID)
	if err != nil {
		return nil, err
	}
	return namespaceToMap(ns), nil
}

func (s *Server) handleNamespaceDelete(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	confirm, _ := optionalString(input, "confirm")

	// 检查 namespace 是否存在
	ns, err := s.engine.GetNamespace(ctx, nsID)
	if err != nil {
		return nil, err
	}

	// 收集关联数据量
	taskCount := 0
	workerCount := 0
	dagCount := 0
	if tasks, err := s.engine.ListTasks(ctx, nsID, engine.StateFilter{}); err == nil {
		taskCount = len(tasks)
	}
	if workers, err := s.engine.ListWorkers(ctx, nsID); err == nil {
		workerCount = len(workers)
	}
	if dags, err := s.engine.ListDAGs(ctx, nsID); err == nil {
		dagCount = len(dags)
	}

	// 第一次调用：返回警告信息 + 弹窗
	if confirm != "true" {
		notifyPopup(
			"Agentflow 删除确认",
			fmt.Sprintf("命名空间 %q (%s) 即将被级联删除。\nDAG:%d Task:%d Worker:%d",
				ns.ID, ns.Name, dagCount, taskCount, workerCount),
		)
		return map[string]any{
			"warning":    "命名空间将被级联删除，此操作不可恢复",
			"namespace":  ns.ID,
			"name":       ns.Name,
			"dags":       dagCount,
			"tasks":      taskCount,
			"workers":    workerCount,
			"confirm":    "再次调用并传 confirm=true 以确认删除",
		}, nil
	}

	// 第二次调用：确认删除
	err = s.engine.DeleteNamespace(ctx, nsID, true)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"deleted":  nsID,
		"name":     ns.Name,
		"dags":     dagCount,
		"tasks":    taskCount,
		"workers":  workerCount,
	}, nil
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

	if transitionValue == "start" || transitionValue == "resume" {
		// 校验并准备 task worktree
		ns, err := s.engine.GetNamespace(ctx, namespaceID)
		if err != nil {
			return taskResult{}, err
		}
		task, err := s.engine.GetTask(ctx, namespaceID, taskID)
		if err != nil {
			return taskResult{}, err
		}
		if task.DAGID == "" {
			return taskResult{}, fmt.Errorf("%s 被拒绝：task %q 没有关联 DAG，无法绑定 git branch/worktree", transitionValue, taskID)
		}
		dag, err := s.engine.GetDAG(ctx, namespaceID, task.DAGID)
		if err != nil {
			return taskResult{}, err
		}
		runtime, err := s.prepareTaskGitRuntime(ctx, ns, dag, task)
		if err != nil {
			return taskResult{}, err
		}
		metadata["git.branch"] = runtime.Branch
		metadata["git.base_branch"] = runtime.BaseBranch
		metadata["git.repo_path"] = runtime.RepoPath
		metadata["git.worktree_path"] = runtime.WorktreePath
		metadata["git.head_sha"] = runtime.HeadSHA
		metadata["git.status"] = runtime.Status

		task, err = s.engine.TransitionTask(ctx, namespaceID, taskID, engine.TaskTransition(transitionValue), metadata)
		if err != nil {
			return taskResult{}, err
		}
		return taskResult{task: task, payload: taskToMap(task)}, nil
	}

	// 校验 2：submit 时必须已有 worker_diary
	if transitionValue == "submit" {
		task, err := s.engine.GetTask(ctx, namespaceID, taskID)
		if err != nil {
			return taskResult{}, err
		}
		today := time.Now().UTC().Format("2006-01-02")
		if _, err := s.engine.GetWorkerDiary(ctx, namespaceID, task.AssignedWorker, today); err != nil {
			// 回退 task 状态（忽略回退本身的错误，原始错误更重要）
			_, _ = s.engine.TransitionTask(ctx, namespaceID, taskID, engine.TransReassign, map[string]string{"actor_role": "leader"})
			return taskResult{}, fmt.Errorf(
				"submit 被拒绝：Worker %q 未写 %s 的工作日记。请先调 mcp__agentflow__worker_diary_write 再 submit",
				task.AssignedWorker, today,
			)
		}
		// 把当前的 HEAD 和 diff 写入 task.metadata，供 reviewer 读取
		if task.DAGID != "" && task.Metadata != nil && task.Metadata["git.worktree_path"] != "" {
			wtPath := task.Metadata["git.worktree_path"]
			dag, _ := s.engine.GetDAG(ctx, namespaceID, task.DAGID)
			base := task.Metadata["git.base_branch"]
			branch := task.Metadata["git.branch"]
			if dag != nil && base == "" { base = "main" }
			if branch != "" {
				if head, err := runGit(ctx, wtPath, "rev-parse", "HEAD"); err == nil {
					metadata["review.commit"] = head
				}
				if base != "" {
					if diff, err := runGit(ctx, wtPath, "diff", base+"..."+branch); err == nil {
						metadata["review.diff"] = diff
					}
				}
			}
		}
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
	dagID, err := optionalString(input, "dag_id")
	if err != nil {
		return req, err
	}
	dependsOn, err := optionalStringSlice(input, "depends_on")
	if err != nil {
		return req, err
	}
	tags, err := optionalStringSlice(input, "tags")
	if err != nil {
		return req, err
	}
	priority, err := optionalInt(input, "priority")
	if err != nil {
		return req, err
	}
	estimatedHours, err := optionalFloat(input, "estimated_hours")
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
		DAGID:              dagID,
		DependsOn:          dependsOn,
		Tags:               tags,
		Priority:           priority,
		EstimatedHours:     estimatedHours,
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


func requiredInt64(input map[string]any, key string) (int64, error) {
	v, ok := input[key]
	if !ok {
		return 0, fmt.Errorf("%w: missing %s", ErrInvalidToolInput, key)
	}
	switch n := v.(type) {
	case float64:
		return int64(n), nil
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	default:
		return 0, fmt.Errorf("%w: %s must be a number", ErrInvalidToolInput, key)
	}
}

func optionalInt64(input map[string]any, key string) (int64, error) {
	v, ok := input[key]
	if !ok || v == nil {
		return 0, nil
	}
	return requiredInt64(input, key)
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
		"dag_id":                task.DAGID,
		"depends_on":            cloneStringsAny(task.DependsOn),
		"tags":                  cloneStringsAny(task.Tags),
		"priority":              task.Priority,
		"estimated_hours":       task.EstimatedHours,
		"actual_hours":          task.ActualHours,
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
