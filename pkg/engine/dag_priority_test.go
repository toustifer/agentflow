package engine

import (
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
