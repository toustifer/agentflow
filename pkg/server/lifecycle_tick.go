package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/toustifer/agentflow/pkg/engine"
)

func (s *Server) handleLifecycleTick(ctx context.Context, input map[string]any) (map[string]any, error) {
	namespaceID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	taskID, err := requiredString(input, "task_id")
	if err != nil {
		return nil, err
	}
	workerID, err := requiredString(input, "worker_id")
	if err != nil {
		return nil, err
	}
	reviewerID, err := requiredString(input, "reviewer_id")
	if err != nil {
		return nil, err
	}
	reviewDecisionInput, err := requiredString(input, "review_decision_input")
	if err != nil {
		return nil, err
	}
	docRecordContent, err := optionalString(input, "doc_record_content")
	if err != nil {
		return nil, err
	}
	docRecordTitle, err := optionalString(input, "doc_record_title")
	if err != nil {
		return nil, err
	}
	diaryEntryContent, err := optionalString(input, "diary_entry_content")
	if err != nil {
		return nil, err
	}

	leaderResult, err := s.handleLeaderTick(ctx, map[string]any{"namespace_id": namespaceID})
	if err != nil {
		return nil, err
	}

	initialTask, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return nil, err
	}
	if initialTask.AssignedWorker != workerID {
		return nil, fmt.Errorf("lifecycle_tick rejected: task %q is assigned to worker %q, not %q", taskID, initialTask.AssignedWorker, workerID)
	}
	if _, err := s.engine.GetWorker(ctx, namespaceID, workerID); err != nil {
		return nil, fmt.Errorf("lifecycle_tick rejected: worker %q not found: %w", workerID, err)
	}
	if _, err := s.engine.GetWorker(ctx, namespaceID, reviewerID); err != nil {
		return nil, fmt.Errorf("lifecycle_tick rejected: reviewer %q not found: %w", reviewerID, err)
	}
	if _, err := s.engine.WorkerPromptGet(ctx, namespaceID, workerID, taskID, initialTask.Title, false); err != nil {
		return nil, fmt.Errorf("lifecycle_tick rejected: worker %q prompt preflight failed: %w", workerID, err)
	}
	if _, err := s.engine.WorkerPromptGet(ctx, namespaceID, reviewerID, taskID, initialTask.Title, true); err != nil {
		return nil, fmt.Errorf("lifecycle_tick rejected: reviewer %q prompt preflight failed: %w", reviewerID, err)
	}
	if initialTask.State != engine.TaskExecuting && initialTask.State != engine.TaskReviewPending && initialTask.State != engine.TaskDone {
		return nil, fmt.Errorf("lifecycle_tick rejected: task %q was not dispatched by leader_tick and is still in state %q", taskID, initialTask.State)
	}

	bridge := btBridgeForRequest(s)
	if bridge == nil {
		return nil, fmt.Errorf("bt_service not available")
	}

	var workerPayload map[string]any
	workerBB := map[string]any{}
	if initialTask.State == engine.TaskExecuting {
		workerBlackboard := map[string]any{
			"namespace_id": namespaceID,
			"task_id":      taskID,
			"worker_id":    workerID,
		}
		if docRecordContent != "" {
			workerBlackboard["doc_record_content"] = docRecordContent
		}
		if docRecordTitle != "" {
			workerBlackboard["doc_record_title"] = docRecordTitle
		}
		if diaryEntryContent != "" {
			workerBlackboard["diary_entry_content"] = diaryEntryContent
		}

		workerPayload, workerBB, err = tickTreeWithBlackboard(bridge, "worker-default", workerBlackboard)
		if err != nil {
			return nil, err
		}
		if workerPayload["status"] != "success" {
			return nil, fmt.Errorf("worker-default tick failed with status %v and blackboard %v", workerPayload["status"], workerBB)
		}

		initialTask, err = s.engine.GetTask(ctx, namespaceID, taskID)
		if err != nil {
			return nil, err
		}
	}

	var reviewerPayload map[string]any
	reviewerBB := map[string]any{}
	if initialTask.State == engine.TaskReviewPending {
		reviewerPayload, reviewerBB, err = tickTreeWithBlackboard(bridge, "reviewer-default", map[string]any{
			"namespace_id":          namespaceID,
			"task_id":               taskID,
			"worker_id":             reviewerID,
			"review_decision_input": reviewDecisionInput,
		})
		if err != nil {
			return nil, err
		}
		if reviewerPayload["status"] != "success" {
			return nil, fmt.Errorf("reviewer-default tick failed with status %v", reviewerPayload["status"])
		}
	}

	finalLeaderResult, err := s.handleLeaderTick(ctx, map[string]any{"namespace_id": namespaceID})
	if err != nil {
		return nil, err
	}

	taskResult, err := s.handleTaskGet(ctx, map[string]any{"namespace_id": namespaceID, "task_id": taskID})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"leader": map[string]any{
			"initial": leaderResult,
			"final":   finalLeaderResult,
		},
		"worker": map[string]any{
			"status":     workerPayload["status"],
			"blackboard": workerBB,
		},
		"reviewer": map[string]any{
			"status":     reviewerPayload["status"],
			"blackboard": reviewerBB,
		},
		"task": taskResult.payload,
	}, nil
}

func tickTreeWithBlackboard(bridge *BTBridge, treeName string, blackboard map[string]any) (map[string]any, map[string]any, error) {
	payload, err := bridge.RPC("tick", map[string]any{
		"tree_name":  treeName,
		"blackboard": blackboard,
		"options":    map[string]any{"return_blackboard": true},
	})
	if err != nil {
		return nil, nil, err
	}
	bb, err := normalizeBTBlackboard(payload["blackboard"])
	if err != nil {
		return nil, nil, err
	}
	return payload, bb, nil
}

func normalizeBTBlackboard(raw any) (map[string]any, error) {
	if raw == nil {
		return map[string]any{}, nil
	}
	if bb, ok := raw.(map[string]any); ok {
		return bb, nil
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(bytes, &out); err != nil {
		return nil, err
	}
	return out, nil
}
