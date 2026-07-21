package server

import (
	"context"
	"testing"
)

func TestHubStatusNoConfig(t *testing.T) {
	t.Setenv("HUB_TOKEN", "")
	t.Setenv("HUB_JWT", "")
	t.Setenv("HUB_API_KEY", "")
	t.Setenv("HUB_BUSINESS_CODE", "")
	t.Setenv("HUB_BUSINESS", "")
	t.Setenv("HUB_SYNC", "")
	t.Setenv("HUB_DISABLED", "")
	t.Setenv("HUB_ENABLED", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	s := &Server{}
	out, err := s.handleHubStatus(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if out["bound"] == true {
		t.Fatalf("expected unbound: %+v", out)
	}
	next, _ := out["next"].([]string)
	if len(next) == 0 {
		t.Fatalf("expected next steps: %+v", out)
	}
}

func TestHubBindTeamRequiresCode(t *testing.T) {
	s := &Server{}
	out, err := s.handleHubBindTeam(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if out["status"] != "failed" {
		t.Fatalf("got %+v", out)
	}
}

func TestToolsIncludeHubBind(t *testing.T) {
	s := &Server{}
	names := map[string]bool{}
	for _, tool := range s.Tools() {
		names[tool.Name] = true
	}
	for _, want := range []string{"hub_status", "hub_login", "hub_list_teams", "hub_bind_team"} {
		if !names[want] {
			t.Fatalf("missing tool %s", want)
		}
	}
}
