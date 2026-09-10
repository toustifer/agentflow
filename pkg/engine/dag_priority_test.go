package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDAGPriorityNormalizationAndWeights(t *testing.T) {
	p, err := NormalizeDAGPriority("p0")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP0, p)
	require.Equal(t, 100, DAGPriorityWeight(p))

	p, err = NormalizeDAGPriority("P1")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP1, p)
	require.Equal(t, 50, DAGPriorityWeight(p))

	p, err = NormalizeDAGPriority("P2")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP2, p)
	require.Equal(t, 20, DAGPriorityWeight(p))

	p, err = NormalizeDAGPriority("p3")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP3, p)
	require.Equal(t, 0, DAGPriorityWeight(p))

	p, err = NormalizeDAGPriority("")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP2, p)

	_, err = NormalizeDAGPriority("invalid")
	require.Error(t, err)
}

func TestDAGPriorityPersistenceAndReopen(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dag_priority.db")

	ctx := context.Background()
	eng1, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)

	_, err = eng1.CreateNamespace(ctx, CreateNamespaceRequest{ID: "ns-pri", Name: "Namespace Pri"})
	require.NoError(t, err)

	d1, err := eng1.CreateDAG(ctx, CreateDAGRequest{
		NamespaceID:     "ns-pri",
		ID:              "dag-p0",
		Title:           "Emergency Fix",
		Priority:        "P0",
		ExecutionBranch: "feat/p0",
	})
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP0, d1.Priority)

	require.NoError(t, eng1.Close())

	// Reopen
	eng2, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	require.NoError(t, err)
	defer eng2.Close()

	d1Reopened, err := eng2.GetDAG(ctx, "ns-pri", "dag-p0")
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP0, d1Reopened.Priority)
}

func TestListDAGsPriorityOrdering(t *testing.T) {
	eng, err := NewEngine(NewEngineConfig{})
	require.NoError(t, err)
	ctx := context.Background()

	_, err = eng.CreateNamespace(ctx, CreateNamespaceRequest{ID: "ns-order", Name: "Order NS"})
	require.NoError(t, err)

	_, err = eng.CreateDAG(ctx, CreateDAGRequest{
		NamespaceID:     "ns-order",
		ID:              "dag-p2",
		Title:           "P2 Task",
		Priority:        "P2",
		ExecutionBranch: "feat/p2",
	})
	require.NoError(t, err)

	_, err = eng.CreateDAG(ctx, CreateDAGRequest{
		NamespaceID:     "ns-order",
		ID:              "dag-p0",
		Title:           "P0 Task",
		Priority:        "P0",
		ExecutionBranch: "feat/p0",
	})
	require.NoError(t, err)

	_, err = eng.CreateDAG(ctx, CreateDAGRequest{
		NamespaceID:     "ns-order",
		ID:              "dag-p1",
		Title:           "P1 Task",
		Priority:        "P1",
		ExecutionBranch: "feat/p1",
	})
	require.NoError(t, err)

	dags, err := eng.ListDAGs(ctx, "ns-order")
	require.NoError(t, err)
	require.Len(t, dags, 3)
	require.Equal(t, "dag-p0", dags[0].ID)
	require.Equal(t, "dag-p1", dags[1].ID)
	require.Equal(t, "dag-p2", dags[2].ID)

	// Update dag-p2 to P0
	p0 := "P0"
	upd, err := eng.UpdateDAG(ctx, "ns-order", "dag-p2", UpdateDAGRequest{Priority: &p0})
	require.NoError(t, err)
	require.Equal(t, DAGPriorityP0, upd.Priority)
}


