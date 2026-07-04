package bt

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlackboardSetGet(t *testing.T) {
	bb := NewBlackboard()
	bb.Set("key1", "value1")
	bb.Set("key2", 42)
	bb.Set("key3", true)

	v, ok := bb.Get("key1")
	require.True(t, ok)
	require.Equal(t, "value1", v)

	v, ok = bb.Get("key2")
	require.True(t, ok)
	require.Equal(t, 42, v)

	v, ok = bb.Get("nope")
	require.False(t, ok)
	require.Nil(t, v)
}

func TestBlackboardHas(t *testing.T) {
	bb := NewBlackboard()
	bb.Set("a", 1)
	require.True(t, bb.Has("a"))
	require.False(t, bb.Has("b"))
}

func TestBlackboardGetString(t *testing.T) {
	bb := NewBlackboard()
	bb.Set("s", "hello")
	bb.Set("n", 42)
	require.Equal(t, "hello", bb.GetString("s"))
	require.Equal(t, "", bb.GetString("n"))
	require.Equal(t, "", bb.GetString("missing"))
}

func TestBlackboardGetBool(t *testing.T) {
	bb := NewBlackboard()
	bb.Set("t", true)
	bb.Set("f", false)
	bb.Set("s", "yes")
	require.True(t, bb.GetBool("t"))
	require.False(t, bb.GetBool("f"))
	require.False(t, bb.GetBool("s"))
	require.False(t, bb.GetBool("missing"))
}

func TestBlackboardParentChain(t *testing.T) {
	parent := NewBlackboard()
	parent.Set("from_parent", "pval")
	parent.Set("override", "original")

	child := NewBlackboard()
	child.Set("from_child", "cval")
	child.Set("override", "child_val")
	child.SetParent(parent)

	v, ok := child.Get("from_parent")
	require.True(t, ok)
	require.Equal(t, "pval", v)

	v, ok = child.Get("override")
	require.True(t, ok)
	require.Equal(t, "child_val", v)

	v, ok = child.Get("from_child")
	require.True(t, ok)
	require.Equal(t, "cval", v)

	_, ok = parent.Get("from_child")
	require.False(t, ok)
}

func TestBlackboardDeepParentChain(t *testing.T) {
	root := NewBlackboard()
	root.Set("a", "root_a")

	mid := NewBlackboard()
	mid.Set("b", "mid_b")
	mid.SetParent(root)

	leaf := NewBlackboard()
	leaf.Set("c", "leaf_c")
	leaf.SetParent(mid)

	v, ok := leaf.Get("a")
	require.True(t, ok)
	require.Equal(t, "root_a", v)

	v, ok = leaf.Get("b")
	require.True(t, ok)
	require.Equal(t, "mid_b", v)

	v, ok = leaf.Get("c")
	require.True(t, ok)
	require.Equal(t, "leaf_c", v)

	require.True(t, leaf.Has("a"))
	require.True(t, leaf.Has("b"))
	require.False(t, mid.Has("c"))
}

func TestBlackboardConcurrentSafe(t *testing.T) {
	bb := NewBlackboard()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			bb.Set("k", n)
			bb.Get("k")
			bb.Has("k")
		}(i)
	}
	wg.Wait()
}

func TestBlackboardContextRoundTrip(t *testing.T) {
	bb := NewBlackboard()
	bb.Set("a", "b")

	ctx := ContextWithBlackboard(context.Background(), bb)
	got := BlackboardFromContext(ctx)
	require.NotNil(t, got)

	v, ok := got.Get("a")
	require.True(t, ok)
	require.Equal(t, "b", v)
}

func TestBlackboardFromContextReturnsNil(t *testing.T) {
	bb := BlackboardFromContext(context.Background())
	require.Nil(t, bb)
}

func TestBlackboardSetParentNil(t *testing.T) {
	child := NewBlackboard()
	child.Set("x", 1)
	child.SetParent(nil)
	v, ok := child.Get("x")
	require.True(t, ok)
	require.Equal(t, 1, v)
	_, ok = child.Get("missing")
	require.False(t, ok)
}

func TestBlackboardParent(t *testing.T) {
	parent := NewBlackboard()
	child := NewBlackboard()
	child.SetParent(parent)
	require.Equal(t, parent, child.Parent())
	require.Nil(t, parent.Parent())
}
