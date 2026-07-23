package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotNoConfig(t *testing.T) {
	clearHubEnv(t)
	// isolate home so we don't pick up real user config
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	snap := Snapshot("")
	if snap.Bound || snap.Enabled {
		t.Fatalf("expected unbound: %+v", snap)
	}
	if snap.Hint == "" {
		t.Fatal("expected hint")
	}
}

func TestBindTeamRequiresLogin(t *testing.T) {
	clearHubEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	c := NewFromWorkdir("")
	r := c.BindTeam(t.Context(), "demo", "")
	if r.OK() {
		t.Fatalf("should not bind without login: %+v", r)
	}
	if r.Status != StatusSkipped && r.Status != StatusFailed {
		t.Fatalf("got %+v", r)
	}
}

func TestSaveAndLoadBusinessCode(t *testing.T) {
	clearHubEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// simulate login token already on disk
	dir := filepath.Join(home, ".agent-hub")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"token":"jwt-test","hub_url":"https://hub.example.test"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// Bind without network membership check: force by using API key path... actually BindTeam lists teams with JWT.
	// For unit test, write business_code via SaveHomeConfig directly then Load.
	if err := SaveHomeConfig(homeConfigFile{BusinessCode: "team-a", BoundAt: "t"}); err != nil {
		t.Fatal(err)
	}
	cfg := Load("")
	if cfg.Token != "jwt-test" || cfg.BusinessCode != "team-a" {
		t.Fatalf("got %+v", cfg)
	}
	if !cfg.Enabled() {
		t.Fatal("expected enabled after token+code")
	}
}

func TestUnbindViaClear(t *testing.T) {
	clearHubEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := SaveHomeConfig(homeConfigFile{Token: "x", BusinessCode: "y"}); err != nil {
		t.Fatal(err)
	}
	if err := ClearBusinessCode(); err != nil {
		t.Fatal(err)
	}
	cfg := Load("")
	if cfg.BusinessCode != "" {
		t.Fatalf("expected clear: %+v", cfg)
	}
	if cfg.Token != "x" {
		t.Fatalf("token should remain: %+v", cfg)
	}
}

func TestParseBusinessStructs(t *testing.T) {
	body := []byte(`{"data":[{"code":"acme","name":"Acme Inc","role":"owner","status":"active"}]}`)
	teams := parseBusinessStructs(body)
	if len(teams) != 1 || teams[0].Code != "acme" || teams[0].Role != "owner" {
		t.Fatalf("got %+v", teams)
	}
}
