package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func clearHubEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"HUB_API_KEY", "HUB_TOKEN", "HUB_JWT", "HUB_BUSINESS_CODE", "HUB_BUSINESS",
		"HUB_BASE_URL", "HUB_SYNC", "HUB_DISABLED", "HUB_ENABLED",
	} {
		t.Setenv(k, "")
	}
}

func TestLoadMissingIsNotEnabled(t *testing.T) {
	clearHubEnv(t)
	cfg := Load("")
	if cfg.Enabled() {
		t.Fatalf("expected disabled without credentials: %+v", cfg)
	}
}

func TestLoadKillSwitchHUB_SYNC(t *testing.T) {
	clearHubEnv(t)
	t.Setenv("HUB_TOKEN", "jwt")
	t.Setenv("HUB_BUSINESS_CODE", "biz")
	t.Setenv("HUB_SYNC", "0")
	cfg := Load("")
	if cfg.Enabled() {
		t.Fatal("HUB_SYNC=0 must disable")
	}
	if !cfg.Disabled {
		t.Fatal("expected Disabled flag")
	}
}

func TestLoadKillSwitchHUB_ENABLED(t *testing.T) {
	clearHubEnv(t)
	t.Setenv("HUB_TOKEN", "jwt")
	t.Setenv("HUB_BUSINESS_CODE", "biz")
	t.Setenv("HUB_ENABLED", "false")
	cfg := Load("")
	if cfg.Enabled() {
		t.Fatal("HUB_ENABLED=false must disable")
	}
}

func TestLoadFromEnvToken(t *testing.T) {
	clearHubEnv(t)
	t.Setenv("HUB_TOKEN", "jwt-abc")
	t.Setenv("HUB_BUSINESS_CODE", "demo")
	t.Setenv("HUB_BASE_URL", "https://hub.example.test")
	cfg := Load("")
	if !cfg.Enabled() {
		t.Fatal("expected enabled")
	}
	if cfg.Token != "jwt-abc" || cfg.BusinessCode != "demo" || cfg.BaseURL != "https://hub.example.test" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestLoadFromWorkdirFile(t *testing.T) {
	clearHubEnv(t)
	dir := t.TempDir()
	my := filepath.Join(dir, ".mycompany")
	if err := os.MkdirAll(my, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"token":"file-jwt","business_code":"file-biz","base_url":"https://from.file"}`
	if err := os.WriteFile(filepath.Join(my, "hub-client.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Load(dir)
	if !cfg.Enabled() {
		t.Fatal("expected enabled from file")
	}
	if cfg.Token != "file-jwt" || cfg.BusinessCode != "file-biz" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestClientGuardDecoupled(t *testing.T) {
	clearHubEnv(t)
	c := NewFromWorkdir("")
	r := c.ReportBranch(t.Context(), BranchReport{Branch: "x", HeadSHA: "a"})
	if r.Status != StatusSkipped && r.Status != StatusDisabled {
		t.Fatalf("expected skip/disabled, got %+v", r)
	}
	// local-only path: note must be non-empty soft string
	if r.Note() == "" {
		t.Fatal("empty note")
	}
}

func TestSyncTaskSkippedWithoutConfig(t *testing.T) {
	clearHubEnv(t)
	c := New(nil)
	r := c.SyncTask(t.Context(), TaskProjection{TaskID: "T1", Title: "t", Status: "assigned"})
	if r.OK() {
		t.Fatalf("should not ok: %+v", r)
	}
	if !r.SoftSkip() && r.Status != StatusDisabled {
		// New(nil) → disabled
		if r.Status != StatusDisabled {
			t.Fatalf("got %+v", r)
		}
	}
}
