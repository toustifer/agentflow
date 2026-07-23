package engine

import (
	"context"
	"errors"
	"testing"
)

func TestWriteProjectDocCASConflict(t *testing.T) {
	e, err := NewEngine(NewEngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx := context.Background()
	_, err = e.CreateNamespace(ctx, CreateNamespaceRequest{ID: "ns-cas", Name: "CAS"})
	if err != nil {
		t.Fatal(err)
	}

	created, err := e.WriteProjectDoc(ctx, "ns-cas", ProjectDoc{
		Section: "arch",
		Path:    "a.md",
		Title:   "A",
		Content: "v1 body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Version != 1 {
		t.Fatalf("version want 1 got %d", created.Version)
	}

	ok, err := e.WriteProjectDocCAS(ctx, "ns-cas", ProjectDoc{
		ID:      created.ID,
		Section: "arch",
		Path:    "a.md",
		Title:   "A",
		Content: "v2 body",
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ok.Version != 2 {
		t.Fatalf("version want 2 got %d", ok.Version)
	}

	_, err = e.WriteProjectDocCAS(ctx, "ns-cas", ProjectDoc{
		ID:      created.ID,
		Content: "stale write",
	}, 1)
	if err == nil {
		t.Fatal("expected version conflict")
	}
	if !errors.Is(err, ErrDocVersionConflict) {
		t.Fatalf("want ErrDocVersionConflict, got %v", err)
	}

	force, err := e.WriteProjectDoc(ctx, "ns-cas", ProjectDoc{
		ID:      created.ID,
		Content: "forced",
		Title:   "A",
		Section: "arch",
		Path:    "a.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if force.Version != 3 {
		t.Fatalf("force version want 3 got %d", force.Version)
	}
}
