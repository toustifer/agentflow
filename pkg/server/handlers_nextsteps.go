package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/toustifer/agentflow/pkg/engine"
)

// handleProjectNextSteps - 检查项目阶段，返回当前进度和推荐下一步
func (s *Server) handleProjectNextSteps(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, _ := optionalString(input, "namespace_id")
	cwd, _ := optionalString(input, "workdir")

	// 如果没有传 namespace_id，用 workdir 反查
	if nsID == "" && cwd != "" {
		allNS, err := s.engine.ListNamespaces(ctx)
		if err == nil {
			for _, ns := range allNS {
				if ns.Metadata != nil {
					if wd, ok := ns.Metadata["workdir"]; ok && wd == cwd {
						nsID = ns.ID
						break
					}
				}
			}
		}
	}
	if nsID == "" && cwd == "" {
		cwd, _ = os.Getwd()
		allNS, err := s.engine.ListNamespaces(ctx)
		if err == nil {
			for _, ns := range allNS {
				if ns.Metadata != nil {
					if wd, ok := ns.Metadata["workdir"]; ok && wd == cwd {
						nsID = ns.ID
						break
					}
				}
			}
		}
	}

	// Phase 0: 无 namespace
	if nsID == "" {
		return map[string]any{
			"phase":        "setup",
			"phase_name":   "未初始化",
			"progress":     "0%",
			"completed":    []string{},
			"next_steps":   []string{"和用户确定项目目标", "创建 namespace（mcp__agentflow__namespace_create）"},
			"actions":      []string{"namespace_create"},
		}, nil
	}

	ns, err := s.engine.GetNamespace(ctx, nsID)
	if err != nil {
		return nil, err
	}

	completed := []string{}
	phase := "setup"
	phaseName := "未初始化"

	// 检查阶段进展
	shapePath := filepath.Join(cwd, ".claude", "PROJECT_FINAL_SHAPE.md")
	if cwd != "" {
		if _, err := os.Stat(shapePath); err == nil {
			completed = append(completed, "形态书已确认")
		}
	}

	// Worker 注册情况
	workers, _ := s.engine.ListWorkers(ctx, nsID)
	if len(workers) > 0 {
		completed = append(completed, fmt.Sprintf("已注册 %d 个 Worker", len(workers)))
	}

	// DAG 情况
	dags, _ := s.engine.ListDAGs(ctx, nsID)
	var activeDAG *engine.DAG
	for i := range dags {
		if dags[i].Status != "" {
			activeDAG = &dags[i]
			break
		}
	}

	// Task 情况
	tasks, _ := s.engine.ListTasks(ctx, nsID, engine.StateFilter{})

	// 判断阶段
	if len(workers) == 0 {
		phase = "shape"
		phaseName = "等待出形态书"
	} else if len(dags) == 0 {
		phase = "plan"
		phaseName = "等待拆解 DAG"
	} else {
		// 有 DAG，检查 task 完成情况
		totalTasks := len(tasks)
		doneTasks := 0
		activeTasks := 0
		for _, t := range tasks {
			if t.State == engine.TaskDone {
				doneTasks++
			}
			if t.State == engine.TaskExecuting || t.State == engine.TaskReviewPending {
				activeTasks++
			}
		}

		if totalTasks > 0 {
			pct := float64(doneTasks) * 100 / float64(totalTasks)
			if doneTasks == totalTasks {
				phase = "done"
				phaseName = fmt.Sprintf("已完成（%d/%d）", doneTasks, totalTasks)
				completed = append(completed, fmt.Sprintf("DAG %q 全部完成", activeDAG.Title))
				return map[string]any{
					"phase":      phase,
					"phase_name": phaseName,
					"progress":   fmt.Sprintf("%.0f%%", pct),
					"completed":  completed,
					"next_steps": []string{"项目已完成，可添加新功能（/agentflow goal + 目标）", "查看项目文档（doc_list）"},
					"actions":    []string{"goal", "doc_list"},
					"dag":        dagToSummaryMap(activeDAG),
				}, nil
			}

			phase = "execute"
			phaseName = fmt.Sprintf("执行中（%d/%d）", doneTasks, totalTasks)
			completed = append(completed, fmt.Sprintf("DAG %q 进度 %d/%d", activeDAG.Title, doneTasks, totalTasks))

			// 下一个可派发的 task
			var nextTasks []map[string]any
			next, _ := s.engine.ProjectNextTasks(ctx, nsID)
			for _, t := range next {
				nextTasks = append(nextTasks, map[string]any{
					"task_id":         t.TaskID,
					"title":           t.Title,
					"assigned_worker": t.AssignedWorker,
				})
			}

			// 活跃的 task（执行中）
			var activeList []map[string]any
			for _, t := range tasks {
				if t.State == engine.TaskExecuting || t.State == engine.TaskReviewPending {
					activeList = append(activeList, map[string]any{
						"task_id": t.ID,
						"title":   t.Title,
						"state":   string(t.State),
						"worker":  t.AssignedWorker,
					})
				}
			}

			result := map[string]any{
				"phase":       phase,
				"phase_name":  phaseName,
				"progress":    fmt.Sprintf("%.0f%%", pct),
				"completed":   completed,
				"next_steps":  []string{},
				"actions":     []string{},
				"dag":         dagToSummaryMap(activeDAG),
				"active_dags": len(dags),
			}
			if len(nextTasks) > 0 {
				result["next_steps"] = append(result["next_steps"].([]string), fmt.Sprintf("派发 task %s（%s）→ task_transition start", nextTasks[0]["task_id"], nextTasks[0]["title"]))
				result["actions"] = append(result["actions"].([]string), "task_transition start")
				result["next_tasks"] = nextTasks
			}
			if len(activeList) > 0 {
				result["next_steps"] = append(result["next_steps"].([]string), fmt.Sprintf("等待 Worker 完成 task %s（检查 task_get 状态）", activeList[0]["task_id"]))
				result["actions"] = append(result["actions"].([]string), "task_get")
				result["active_tasks"] = activeList
			}

			// reactive: 既无可派发也无活跃 → stuck
			if len(nextTasks) == 0 && len(activeList) == 0 {
				var stuckTasks []map[string]any
				for _, t := range tasks {
					if t.State != engine.TaskDone {
						stuckTasks = append(stuckTasks, map[string]any{
							"task_id": t.ID,
							"title":   t.Title,
							"state":   string(t.State),
							"worker":  t.AssignedWorker,
						})
					}
				}
				return map[string]any{
					"phase":       "stuck",
					"phase_name":  "阻塞",
					"progress":    fmt.Sprintf("%.0f%%", pct),
					"completed":   completed,
					"dag":         dagToSummaryMap(activeDAG),
					"stuck_tasks": stuckTasks,
					"next_steps":  []string{"检查 stuck_tasks 中的任务状态，手动修复阻塞后就恢复正常"},
					"actions":     []string{"task_get", "report_blockers"},
				}, nil
			}

			return result, nil
		}

			// 有 DAG 但无 task
			phase = "plan"
			phaseName = "等待拆解 Task"
	}

	// 非 execute 阶段的通用返回
	result := map[string]any{
		"phase":       phase,
		"phase_name":  phaseName,
		"progress":    "0%",
		"completed":   completed,
		"next_steps":  []string{},
		"actions":     []string{},
		"namespace":   ns.ID,
		"namespace_name": ns.Name,
		"workdir":     getWorkdir(ns),
		"active_dags": len(dags),
		"workers":     len(workers),
	}

	switch phase {
	case "shape":
		result["next_steps"] = []string{"和用户确认技术栈/功能清单/做和不做的边界", "写入 .claude/PROJECT_FINAL_SHAPE.md", "注册 Worker"}
		result["actions"] = []string{"brainstorm", "worker_register"}
	case "plan":
		if len(workers) > 0 && len(dags) == 0 {
			result["next_steps"] = []string{"拆解第一个 DAG", "用 dag_create + task_create_batch 创建"}
			result["actions"] = []string{"dag_create", "task_create_batch"}
		} else {
			result["next_steps"] = []string{"用 task_create_batch 补全 task"}
			result["actions"] = []string{"task_create_batch"}
		}
	}

	return result, nil
}

func getWorkdir(ns *engine.Namespace) string {
	if ns.Metadata != nil {
		if wd, ok := ns.Metadata["workdir"]; ok {
			return wd
		}
	}
	return ""
}

func dagToSummaryMap(dag *engine.DAG) map[string]any {
	if dag == nil {
		return nil
	}
	return map[string]any{
		"id":     dag.ID,
		"title":  dag.Title,
		"branch": dag.Branch,
		"status": string(dag.Status),
	}
}
