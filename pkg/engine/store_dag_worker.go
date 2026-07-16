package engine

import (
	"database/sql"
	"time"
)

// ---------------------------------------------------------------------------
// DAG persistence
// ---------------------------------------------------------------------------

func insertDAG(db *sql.DB, dag *DAG) error {
	meta := mustMarshalJSON(dag.Metadata)
	_, err := db.Exec(
		`INSERT INTO dags (id, namespace_id, title, branch, execution_branch, base_branch, metadata, worktree_path, worktree_status, head_sha, active_task_id, lease_holder_task_id, lease_holder_worker_id, lease_holder_agent_id, lease_acquired_at, runtime_updated_at, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		dag.ID, dag.NamespaceID, dag.Title, dag.ExecutionBranch, dag.ExecutionBranch, dag.BaseBranch, meta, dag.WorktreePath, dag.WorktreeStatus, dag.HeadSHA, dag.ActiveTaskID, dag.LeaseHolderTaskID, dag.LeaseHolderWorkerID, dag.LeaseHolderAgentID, dag.LeaseAcquiredAt, dag.RuntimeUpdatedAt, string(dag.Status),
		dag.CreatedAt.Format(time.RFC3339Nano), dag.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func updateDAG(db *sql.DB, dag *DAG) error {
	meta := mustMarshalJSON(dag.Metadata)
	_, err := db.Exec(
		`UPDATE dags SET title=?, branch=?, execution_branch=?, base_branch=?, metadata=?, worktree_path=?, worktree_status=?, head_sha=?, active_task_id=?, lease_holder_task_id=?, lease_holder_worker_id=?, lease_holder_agent_id=?, lease_acquired_at=?, runtime_updated_at=?, status=?, updated_at=? WHERE namespace_id=? AND id=?`,
		dag.Title, dag.ExecutionBranch, dag.ExecutionBranch, dag.BaseBranch, meta, dag.WorktreePath, dag.WorktreeStatus, dag.HeadSHA, dag.ActiveTaskID, dag.LeaseHolderTaskID, dag.LeaseHolderWorkerID, dag.LeaseHolderAgentID, dag.LeaseAcquiredAt, dag.RuntimeUpdatedAt, string(dag.Status),
		dag.UpdatedAt.Format(time.RFC3339Nano),
		dag.NamespaceID, dag.ID,
	)
	return err
}

func loadDAGs(db *sql.DB) (map[string]map[string]*DAG, error) {
	rows, err := db.Query(`SELECT id, namespace_id, title, branch, execution_branch, base_branch, metadata, worktree_path, worktree_status, head_sha, active_task_id, lease_holder_task_id, lease_holder_worker_id, lease_holder_agent_id, lease_acquired_at, runtime_updated_at, status, created_at, updated_at FROM dags`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*DAG)
	for rows.Next() {
		var (
			id, nsID, title, branch, executionBranch, baseBranch, metaStr, worktreePath, worktreeStatus, headSHA, activeTaskID, leaseHolderTaskID, leaseHolderWorkerID, leaseHolderAgentID, leaseAcquiredAt, runtimeUpdatedAt, statusStr, createdAtStr, updatedAtStr string
		)
		if err := rows.Scan(&id, &nsID, &title, &branch, &executionBranch, &baseBranch, &metaStr, &worktreePath, &worktreeStatus, &headSHA, &activeTaskID, &leaseHolderTaskID, &leaseHolderWorkerID, &leaseHolderAgentID, &leaseAcquiredAt, &runtimeUpdatedAt, &statusStr, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		if executionBranch == "" {
			executionBranch = branch
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		dag := &DAG{
			ID:                  id,
			NamespaceID:         nsID,
			Title:               title,
			ExecutionBranch:     executionBranch,
			BaseBranch:          baseBranch,
			Metadata:            mustUnmarshalStringMap(metaStr),
			WorktreePath:        worktreePath,
			WorktreeStatus:      worktreeStatus,
			HeadSHA:             headSHA,
			ActiveTaskID:        activeTaskID,
			LeaseHolderTaskID:   leaseHolderTaskID,
			LeaseHolderWorkerID: leaseHolderWorkerID,
			LeaseHolderAgentID:  leaseHolderAgentID,
			LeaseAcquiredAt:     leaseAcquiredAt,
			RuntimeUpdatedAt:    runtimeUpdatedAt,
			Status:              DAGStatus(statusStr),
			CreatedAt:           createdAt,
			UpdatedAt:           updatedAt,
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*DAG)
		}
		out[nsID][id] = dag
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Worker persistence
// ---------------------------------------------------------------------------

func insertWorker(db *sql.DB, w *Worker) error {
	skills := mustMarshalJSON(w.Skills)
	taskTags := mustMarshalJSON(w.TaskTags)
	requiredReads := mustMarshalJSON(w.RequiredReads)
	recommendedMCP := mustMarshalJSON(w.RecommendedMCP)
	handoffTargets := mustMarshalJSON(w.HandoffTargets)
	recoveryPolicy := mustMarshalJSON(w.RecoveryPolicy)
	fallbackMCP := mustMarshalJSON(w.FallbackMCP)
	meta := mustMarshalJSON(w.Metadata)
	_, err := db.Exec(
		`INSERT INTO workers (id, namespace_id, name, kind, scope, skills, task_tags, prompt_template, required_reads, recommended_mcp, launch_mode, handoff_targets, recovery_policy, fallback_mcp, stuck_playbook, escalation_mode, metadata, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.NamespaceID, w.Name, w.Kind, w.Scope, skills, taskTags, w.PromptTemplate, requiredReads, recommendedMCP, w.LaunchMode, handoffTargets, recoveryPolicy, fallbackMCP, w.StuckPlaybook, w.EscalationMode, meta,
		w.CreatedAt.Format(time.RFC3339Nano), w.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func updateWorker(db *sql.DB, w *Worker) error {
	skills := mustMarshalJSON(w.Skills)
	taskTags := mustMarshalJSON(w.TaskTags)
	requiredReads := mustMarshalJSON(w.RequiredReads)
	recommendedMCP := mustMarshalJSON(w.RecommendedMCP)
	handoffTargets := mustMarshalJSON(w.HandoffTargets)
	recoveryPolicy := mustMarshalJSON(w.RecoveryPolicy)
	fallbackMCP := mustMarshalJSON(w.FallbackMCP)
	meta := mustMarshalJSON(w.Metadata)
	_, err := db.Exec(
		`UPDATE workers SET name=?, kind=?, scope=?, skills=?, task_tags=?, prompt_template=?, required_reads=?, recommended_mcp=?, launch_mode=?, handoff_targets=?, recovery_policy=?, fallback_mcp=?, stuck_playbook=?, escalation_mode=?, metadata=?, updated_at=? WHERE namespace_id=? AND id=?`,
		w.Name, w.Kind, w.Scope, skills, taskTags, w.PromptTemplate, requiredReads, recommendedMCP, w.LaunchMode, handoffTargets, recoveryPolicy, fallbackMCP, w.StuckPlaybook, w.EscalationMode, meta,
		w.UpdatedAt.Format(time.RFC3339Nano),
		w.NamespaceID, w.ID,
	)
	return err
}

func loadWorkers(db *sql.DB) (map[string]map[string]*Worker, error) {
	rows, err := db.Query(`SELECT id, namespace_id, name, kind, scope, skills, task_tags, prompt_template, required_reads, recommended_mcp, launch_mode, handoff_targets, recovery_policy, fallback_mcp, stuck_playbook, escalation_mode, metadata, created_at, updated_at FROM workers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]map[string]*Worker)
	for rows.Next() {
		var (
			id, nsID, name, kind, scope, skillsStr, taskTagsStr, promptTpl, requiredReadsStr, recommendedMCPStr, launchMode, handoffTargetsStr, recoveryPolicyStr, fallbackMCPStr, stuckPlaybook, escalationMode, metaStr, createdAtStr, updatedAtStr string
		)
		if err := rows.Scan(&id, &nsID, &name, &kind, &scope, &skillsStr, &taskTagsStr, &promptTpl, &requiredReadsStr, &recommendedMCPStr, &launchMode, &handoffTargetsStr, &recoveryPolicyStr, &fallbackMCPStr, &stuckPlaybook, &escalationMode, &metaStr, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339Nano, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtStr)
		w := &Worker{
			ID:               id,
			NamespaceID:      nsID,
			Name:             name,
			Kind:             kind,
			Scope:            scope,
			Skills:           mustUnmarshalStringSlice(skillsStr),
			TaskTags:         mustUnmarshalStringSlice(taskTagsStr),
			PromptTemplate:   promptTpl,
			RequiredReads:    mustUnmarshalStringSlice(requiredReadsStr),
			RecommendedMCP:   mustUnmarshalStringSlice(recommendedMCPStr),
			LaunchMode:       launchMode,
			HandoffTargets:   mustUnmarshalStringSlice(handoffTargetsStr),
			RecoveryPolicy:   mustUnmarshalStringSlice(recoveryPolicyStr),
			FallbackMCP:      mustUnmarshalStringSlice(fallbackMCPStr),
			StuckPlaybook:    stuckPlaybook,
			EscalationMode:   escalationMode,
			Metadata:         mustUnmarshalStringMap(metaStr),
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
		}
		if out[nsID] == nil {
			out[nsID] = make(map[string]*Worker)
		}
		out[nsID][id] = w
	}
	return out, rows.Err()
}
