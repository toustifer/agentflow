package server

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
)

func TestPickResumeDAGSkipsLegacyInProgressDAG(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	dags := []engine.DAG{
		{
			ID:        "legacy",
			Title:     "Legacy Integration",
			Status:    engine.DAGInProgress,
			UpdatedAt: now.Add(-2 * time.Hour),
			Metadata: map[string]string{
				dagMetadataLegacy:         "true",
				dagMetadataResumePriority: string(resumePriorityDeprioritized),
			},
		},
		{
			ID:        "current",
			Title:     "Current Release",
			Status:    engine.DAGInProgress,
			UpdatedAt: now,
		},
	}

	picked := pickResumeDAG(dags, "")
	require.NotNil(t, picked)
	require.Equal(t, "current", picked.ID)
}

func TestPickResumeDAGHonorsExplicitTarget(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	dags := []engine.DAG{
		{
			ID:        "legacy",
			Title:     "Legacy Integration",
			Status:    engine.DAGInProgress,
			UpdatedAt: now.Add(-2 * time.Hour),
			Metadata: map[string]string{
				dagMetadataLegacy:         "true",
				dagMetadataResumePriority: string(resumePriorityDeprioritized),
			},
		},
		{
			ID:        "current",
			Title:     "Current Release",
			Status:    engine.DAGInProgress,
			UpdatedAt: now,
		},
	}

	picked := pickResumeDAG(dags, "legacy")
	require.NotNil(t, picked)
	require.Equal(t, "legacy", picked.ID)
}

func TestProjectNextStepsSkipsLegacyDAGByDefault(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-current",
		Name:           "Worker Current",
		PromptTemplate: "Task {task_id}",
	})
	require.NoError(t, err)
	_, err = srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-legacy",
		Name:           "Worker Legacy",
		PromptTemplate: "Task {task_id}",
	})
	require.NoError(t, err)

	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "legacy-dag",
		Title:           "Legacy DAG",
		ExecutionBranch: "feature/legacy",
		BaseBranch:      "main",
		Metadata: map[string]string{
			dagMetadataLegacy:         "true",
			dagMetadataResumePriority: string(resumePriorityDeprioritized),
		},
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "current-dag",
		Title:           "Current DAG",
		ExecutionBranch: "feature/current",
		BaseBranch:      "main",
	})
	require.NoError(t, err)

	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "legacy-task",
		Title:          "legacy task",
		AssignedWorker: "worker-legacy",
		DAGID:          "legacy-dag",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "current-task",
		Title:          "current task",
		AssignedWorker: "worker-current",
		DAGID:          "current-dag",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "project_next_steps", map[string]any{
		"namespace_id": "ns-1",
	})
	require.NoError(t, err)
	dag := result["dag"].(map[string]any)
	require.Equal(t, "current-dag", dag["id"])
	nextTasks := result["next_tasks"].([]map[string]any)
	require.Len(t, nextTasks, 1)
	require.Equal(t, "current-task", nextTasks[0]["task_id"])
	skipped := result["legacy_dags_skipped"].([]map[string]any)
	require.Len(t, skipped, 1)
	require.Equal(t, "legacy-dag", skipped[0]["dag_id"])
}

func TestProjectNextStepsHonorsExplicitDAGTarget(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-current",
		Name:           "Worker Current",
		PromptTemplate: "Task {task_id}",
	})
	require.NoError(t, err)
	_, err = srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-legacy",
		Name:           "Worker Legacy",
		PromptTemplate: "Task {task_id}",
	})
	require.NoError(t, err)

	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "legacy-dag",
		Title:           "Legacy DAG",
		ExecutionBranch: "feature/legacy",
		BaseBranch:      "main",
		Metadata: map[string]string{
			dagMetadataLegacy:         "true",
			dagMetadataResumePriority: string(resumePriorityDeprioritized),
		},
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "current-dag",
		Title:           "Current DAG",
		ExecutionBranch: "feature/current",
		BaseBranch:      "main",
	})
	require.NoError(t, err)

	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "legacy-task",
		Title:          "legacy task",
		AssignedWorker: "worker-legacy",
		DAGID:          "legacy-dag",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "current-task",
		Title:          "current task",
		AssignedWorker: "worker-current",
		DAGID:          "current-dag",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "project_next_steps", map[string]any{
		"namespace_id": "ns-1",
		"dag_id":       "legacy-dag",
	})
	require.NoError(t, err)
	dag := result["dag"].(map[string]any)
	require.Equal(t, "legacy-dag", dag["id"])
	nextTasks := result["next_tasks"].([]map[string]any)
	require.Len(t, nextTasks, 1)
	require.Equal(t, "legacy-task", nextTasks[0]["task_id"])
}

func TestProjectInspectShowsLegacyDagFlags(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-current",
		Name:           "Worker Current",
		PromptTemplate: "Task {task_id}",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "legacy-dag",
		Title:           "Legacy DAG",
		ExecutionBranch: "feature/legacy",
		BaseBranch:      "main",
		Metadata: map[string]string{
			dagMetadataLegacy:         "true",
			dagMetadataResumePriority: string(resumePriorityDeprioritized),
			dagMetadataSupersededBy:   "current-dag",
		},
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "legacy-task",
		Title:          "legacy task",
		AssignedWorker: "worker-current",
		DAGID:          "legacy-dag",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "project_inspect", map[string]any{
		"namespace_id": "ns-1",
		"focus":        "project",
	})
	require.NoError(t, err)
	dags := result["dags"].([]any)
	require.NotEmpty(t, dags)
	first := dags[0].(map[string]any)
	dag := first["dag"].(map[string]any)
	require.Contains(t, dag, "legacy")
	require.Contains(t, dag, "resume_priority")
	require.Contains(t, dag, "superseded_by")
}

func TestProjectInspectHonorsTargetedDAGForNextSteps(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	_, err := srv.engine.RegisterWorker(context.Background(), engine.RegisterWorkerRequest{
		NamespaceID:    "ns-1",
		ID:             "worker-current",
		Name:           "Worker Current",
		PromptTemplate: "Task {task_id}",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "legacy-dag",
		Title:           "Legacy DAG",
		ExecutionBranch: "feature/legacy",
		BaseBranch:      "main",
		Metadata: map[string]string{
			dagMetadataLegacy:         "true",
			dagMetadataResumePriority: string(resumePriorityDeprioritized),
			dagMetadataSupersededBy:   "current-dag",
		},
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateDAG(context.Background(), engine.CreateDAGRequest{
		NamespaceID:     "ns-1",
		ID:              "current-dag",
		Title:           "Current DAG",
		ExecutionBranch: "feature/current",
		BaseBranch:      "main",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "legacy-task",
		Title:          "legacy task",
		AssignedWorker: "worker-current",
		DAGID:          "legacy-dag",
	})
	require.NoError(t, err)
	_, err = srv.engine.CreateTask(context.Background(), engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "current-task",
		Title:          "current task",
		AssignedWorker: "worker-current",
		DAGID:          "current-dag",
	})
	require.NoError(t, err)

	result, err := srv.Handle(context.Background(), "project_inspect", map[string]any{
		"namespace_id": "ns-1",
		"focus":        "dag",
		"dag_id":       "legacy-dag",
	})
	require.NoError(t, err)

	// next_tasks / summary should follow the explicitly targeted legacy DAG
	rawNext := result["next_tasks"].([]any)
	require.Len(t, rawNext, 1)
	require.Equal(t, "legacy-task", rawNext[0].(map[string]any)["task_id"])

	summary := result["summary"].(map[string]any)
	require.Equal(t, 1, summary["ready_count"])

	focused := result["focused_dag"].(map[string]any)
	require.Equal(t, "legacy-dag", focused["id"])
	require.Equal(t, true, focused["legacy"])
}
