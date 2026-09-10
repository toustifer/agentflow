package engine

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEngineInitializesGoalsMapAndSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_goals_init.db")

	eng, err := NewEngine(NewEngineConfig{DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	if eng.goals == nil {
		t.Fatalf("eng.goals should be initialized")
	}

	// Verify goals table exists in sqlite
	var count int
	err = eng.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('goals') WHERE name = 'id'`).Scan(&count)
	if err != nil || count == 0 {
		t.Fatalf("goals table was not properly initialized: %v", err)
	}
}

func TestGoalCRUDAndAutoID(t *testing.T) {
	eng, err := NewEngine(NewEngineConfig{})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	ctx := context.Background()
	_, err = eng.CreateNamespace(ctx, CreateNamespaceRequest{ID: "ns-1", Name: "Namespace 1"})
	if err != nil {
		t.Fatalf("failed to create namespace: %v", err)
	}

	// 1. Auto-generate G-1
	g1, err := eng.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-1",
		Title:       "Auto Goal 1",
		Priority:    10,
		Tags:        []string{"feat"},
	})
	if err != nil {
		t.Fatalf("CreateGoal failed: %v", err)
	}
	if g1.ID != "G-1" {
		t.Fatalf("expected ID G-1, got %s", g1.ID)
	}
	if g1.Status != GoalPending {
		t.Fatalf("expected default status pending, got %s", g1.Status)
	}

	// 2. Auto-generate G-2
	g2, err := eng.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-1",
		Title:       "Auto Goal 2",
		Priority:    20,
	})
	if err != nil {
		t.Fatalf("CreateGoal 2 failed: %v", err)
	}
	if g2.ID != "G-2" {
		t.Fatalf("expected ID G-2, got %s", g2.ID)
	}

	// 3. Custom ID support
	gCustom, err := eng.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-1",
		ID:          "CUSTOM-99",
		Title:       "Custom Goal",
	})
	if err != nil {
		t.Fatalf("CreateGoal custom failed: %v", err)
	}
	if gCustom.ID != "CUSTOM-99" {
		t.Fatalf("expected ID CUSTOM-99, got %s", gCustom.ID)
	}

	// 4. Next auto-generate G-3 (skips custom)
	g3, err := eng.CreateGoal(ctx, CreateGoalRequest{
		NamespaceID: "ns-1",
		Title:       "Auto Goal 3",
	})
	if err != nil {
		t.Fatalf("CreateGoal 3 failed: %v", err)
	}
	if g3.ID != "G-3" {
		t.Fatalf("expected ID G-3, got %s", g3.ID)
	}

	// 5. GetGoal
	fetched, err := eng.GetGoal(ctx, "ns-1", "G-1")
	if err != nil {
		t.Fatalf("GetGoal failed: %v", err)
	}
	if fetched.Title != "Auto Goal 1" {
		t.Fatalf("unexpected fetched title: %s", fetched.Title)
	}

	// 6. UpdateGoal (defer it)
	updated, err := eng.UpdateGoal(ctx, UpdateGoalRequest{
		NamespaceID: "ns-1",
		ID:          "G-1",
		Status:      GoalDeferred,
		Priority:    ptrInt(15),
	})
	if err != nil {
		t.Fatalf("UpdateGoal failed: %v", err)
	}
	if updated.Status != GoalDeferred || updated.Priority != 15 {
		t.Fatalf("unexpected updated goal: %+v", updated)
	}

	// 7. ListGoals with default filtering (pending + deferred, sorted by priority desc)
	list, err := eng.ListGoals(ctx, GoalFilter{NamespaceID: "ns-1"})
	if err != nil {
		t.Fatalf("ListGoals failed: %v", err)
	}
	if len(list) != 4 { // G-2 (pri 20), G-1 (pri 15), G-3 (pri 0), CUSTOM-99 (pri 0)
		t.Fatalf("expected 4 active goals, got %d", len(list))
	}
	if list[0].ID != "G-2" || list[1].ID != "G-1" {
		t.Fatalf("expected priority ordering G-2, G-1, got %s, %s", list[0].ID, list[1].ID)
	}
}

func ptrInt(v int) *int { return &v }

