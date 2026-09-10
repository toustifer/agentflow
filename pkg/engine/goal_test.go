package engine

import (
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
