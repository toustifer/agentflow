package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewServerRequiresEngine(t *testing.T) {
	t.Parallel()

	_, err := New(nil, Config{})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEngineRequired)
}
