package server

import (
	"context"
	"testing"

	"github.com/toustifer/agentflow/pkg/engine"
)

func TestProjectTaskWhitelist(t *testing.T) {
	task := &engine.Task{
		ID:             "T1",
		Title:          "Do thing",
		State:          engine.TaskAssigned,
		AssignedWorker: "alice",
		DependsOn:      []string{"T0"},
		OutputFiles:    []string{"a.go"},
		Metadata: map[string]string{
			"git.branch":   "feature/x",
			"git.head_sha": "abc123",
		},
	}
	p := projectTask(task, "", "")
	if p.TaskID != "T1" || p.Title != "Do thing" || p.Status != "assigned" {
		t.Fatalf("basic fields: %+v", p)
	}
	if p.Branch != "feature/x" || p.HeadSHA != "abc123" {
		t.Fatalf("git refs: %+v", p)
	}
	if p.AssignedWorker != "alice" || len(p.DependsOn) != 1 {
		t.Fatalf("worker/deps: %+v", p)
	}
}

func TestSoftHubAfterTaskNoConfig(t *testing.T) {
	t.Setenv("HUB_TOKEN", "")
	t.Setenv("HUB_JWT", "")
	t.Setenv("HUB_API_KEY", "")
	t.Setenv("HUB_BUSINESS_CODE", "")
	t.Setenv("HUB_BUSINESS", "")
	t.Setenv("HUB_SYNC", "")
	task := &engine.Task{ID: "T9", Title: "x", State: engine.TaskExecuting}
	note := softHubAfterTask(context.Background(), "", task, "", "")
	if note == "" {
		t.Fatal("expected note")
	}
	// must soft-skip, never panic
	if !(containsAny(note, "skipped", "disabled")) {
		t.Fatalf("expected skip/disabled, got %q", note)
	}
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if len(p) > 0 && (len(s) >= len(p)) {
			for i := 0; i+len(p) <= len(s); i++ {
				if s[i:i+len(p)] == p {
					return true
				}
			}
		}
	}
	return false
}
