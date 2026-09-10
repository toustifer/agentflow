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

