package server

import (
	"context"
	"fmt"
	"time"

	"github.com/toustifer/agentflow/pkg/engine"
)

func goalToMap(g *engine.Goal) map[string]any {
	if g == nil {
		return nil
	}
	tags := g.Tags
	if tags == nil {
		tags = []string{}
	}
	meta := g.Metadata
	if meta == nil {
		meta = map[string]string{}
	}
	return map[string]any{
		"id":           g.ID,
		"namespace_id": g.NamespaceID,
		"title":        g.Title,
		"description":  g.Description,
		"status":       string(g.Status),
		"priority":     g.Priority,
		"tags":         tags,
		"context":      g.Context,
		"dag_id":       g.DAGID,
		"metadata":     meta,
		"created_at":   g.CreatedAt.Format(time.RFC3339Nano),
		"updated_at":   g.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func (s *Server) handleGoalCreate(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	title, err := requiredString(input, "title")
	if err != nil {
		return nil, err
	}
	desc, _ := optionalString(input, "description")
	goalID, _ := optionalString(input, "goal_id")
	contextVal, _ := optionalString(input, "context")
	pri, _ := optionalInt(input, "priority")
	tags, _ := optionalStringSlice(input, "tags")
	metadata, _ := optionalStringMap(input, "metadata")

	goal, err := s.engine.CreateGoal(ctx, engine.CreateGoalRequest{
		NamespaceID: nsID,
		ID:          goalID,
		Title:       title,
		Description: desc,
		Priority:    pri,
		Tags:        tags,
		Context:     contextVal,
		Metadata:    metadata,
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"goal": goalToMap(goal),
	}, nil
}

func (s *Server) handleGoalGet(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	goalID, err := requiredString(input, "goal_id")
	if err != nil {
		return nil, err
	}

	goal, err := s.engine.GetGoal(ctx, nsID, goalID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"goal": goalToMap(goal),
	}, nil
}

func (s *Server) handleGoalUpdate(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	goalID, err := requiredString(input, "goal_id")
	if err != nil {
		return nil, err
	}

	req := engine.UpdateGoalRequest{
		NamespaceID: nsID,
		ID:          goalID,
	}
	if v, ok := input["title"].(string); ok {
		req.Title = &v
	}
	if v, ok := input["description"].(string); ok {
		req.Description = &v
	}
	if v, ok := input["status"].(string); ok {
		req.Status = engine.GoalStatus(v)
	}
	if _, ok := input["priority"]; ok {
		pri, err := optionalInt(input, "priority")
		if err != nil {
			return nil, err
		}
		req.Priority = &pri
	}
	if _, ok := input["tags"]; ok {
		tags, err := optionalStringSlice(input, "tags")
		if err != nil {
			return nil, err
		}
		req.Tags = tags
	}
	if v, ok := input["context"].(string); ok {
		req.Context = &v
	}
	if _, ok := input["metadata"]; ok {
		meta, err := optionalStringMap(input, "metadata")
		if err != nil {
			return nil, err
		}
		req.Metadata = meta
	}

	goal, err := s.engine.UpdateGoal(ctx, req)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"goal": goalToMap(goal),
	}, nil
}

func (s *Server) handleGoalList(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}

	filter := engine.GoalFilter{
		NamespaceID: nsID,
	}

	if _, ok := input["status"]; ok {
		statusStrs, err := optionalStringSlice(input, "status")
		if err != nil {
			return nil, err
		}
		for _, s := range statusStrs {
			filter.Statuses = append(filter.Statuses, engine.GoalStatus(s))
		}
	}
	if _, ok := input["tags"]; ok {
		tags, err := optionalStringSlice(input, "tags")
		if err != nil {
			return nil, err
		}
		filter.Tags = tags
	}
	if _, ok := input["priority_gte"]; ok {
		pri, err := optionalInt(input, "priority_gte")
		if err != nil {
			return nil, err
		}
		filter.PriorityGTE = &pri
	}

	goals, err := s.engine.ListGoals(ctx, filter)
	if err != nil {
		return nil, err
	}

	items := make([]any, 0, len(goals))
	for i := range goals {
		items = append(items, goalToMap(&goals[i]))
	}

	return map[string]any{
		"namespace_id": nsID,
		"total":        len(items),
		"goals":        items,
	}, nil
}

func (s *Server) handleGoalPromote(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	goalID, err := requiredString(input, "goal_id")
	if err != nil {
		return nil, err
	}
	dagID, _ := optionalString(input, "dag_id")
	dagTitle, _ := optionalString(input, "dag_title")
	execBranch, _ := optionalString(input, "execution_branch")
	baseBranch, _ := optionalString(input, "base_branch")

	res, err := s.engine.PromoteGoal(ctx, engine.PromoteGoalRequest{
		NamespaceID:     nsID,
		GoalID:          goalID,
		DAGID:           dagID,
		DAGTitle:        dagTitle,
		ExecutionBranch: execBranch,
		BaseBranch:      baseBranch,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"goal": goalToMap(&res.Goal),
		"dag":  dagToMap(res.DAG),
		"next_steps": []string{
			fmt.Sprintf("已为目标 %s 创建执行 DAG %q", res.Goal.ID, res.DAG.ID),
			"请调用 task_create / task_create_batch 为该 DAG 编排任务拓扑",
		},
		"actions": []string{"task_create", "task_create_batch"},
	}, nil
}
