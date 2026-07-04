package bt

import "context"

// SubTree embeds another registered tree. A child blackboard is created
// linked to the parent, so Get() traverses up the chain. The child
// blackboard persists while the subtree is Running, and resets on completion.
type SubTree struct {
	treeName string
	registry *Registry
	childBB  *Blackboard
}

// NewSubTree creates a SubTree that ticks the named tree from registry.
func NewSubTree(treeName string, registry *Registry) *SubTree {
	return &SubTree{
		treeName: treeName,
		registry: registry,
	}
}

func (st *SubTree) Tick(ctx context.Context) Status {
	parentBB := BlackboardFromContext(ctx)

	if st.childBB == nil {
		st.childBB = NewBlackboard()
	}
	st.childBB.SetParent(parentBB)

	childCtx := ContextWithBlackboard(ctx, st.childBB)
	status, _ := st.registry.Tick(childCtx, st.treeName)

	if status != Running {
		st.childBB = nil
	}

	return status
}

func (st *SubTree) Halt() {
	// If the subtree itself is Haltable, halt it.
	if st.childBB != nil {
		if h, ok := st.registry.Get(st.treeName); ok {
			if ht, ok := h.(Haltable); ok {
				ht.Halt()
			}
		}
	}
	st.childBB = nil
}
