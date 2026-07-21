package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadHubClientConfigMissingIsNil(t *testing.T) {
	t.Setenv("HUB_API_KEY", "")
	t.Setenv("HUB_TOKEN", "")
	t.Setenv("HUB_JWT", "")
	t.Setenv("HUB_BUSINESS_CODE", "")
	t.Setenv("HUB_BUSINESS", "")
	t.Setenv("HUB_SYNC", "")
	t.Setenv("HUB_DISABLED", "")
	t.Setenv("HUB_ENABLED", "")
	if cfg := loadHubClientConfig(""); cfg != nil {
		t.Fatalf("expected nil without credentials, got %+v", cfg)
	}
}

func TestLoadHubClientConfigFromEnvAPIKey(t *testing.T) {
	t.Setenv("HUB_API_KEY", "test-key")
	t.Setenv("HUB_TOKEN", "")
	t.Setenv("HUB_JWT", "")
	t.Setenv("HUB_BUSINESS_CODE", "demo-biz")
	t.Setenv("HUB_BASE_URL", "https://hub.example.test")
	t.Setenv("HUB_SYNC", "")
	t.Setenv("HUB_DISABLED", "")
	t.Setenv("HUB_ENABLED", "")
	cfg := loadHubClientConfig("")
	if cfg == nil {
		t.Fatal("expected config from env")
	}
	if cfg.APIKey != "test-key" || cfg.BusinessCode != "demo-biz" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.BaseURL != "https://hub.example.test" {
		t.Fatalf("base url: %s", cfg.BaseURL)
	}
}

func TestLoadHubClientConfigFromEnvToken(t *testing.T) {
	t.Setenv("HUB_API_KEY", "")
	t.Setenv("HUB_TOKEN", "jwt-abc")
	t.Setenv("HUB_JWT", "")
	t.Setenv("HUB_BUSINESS_CODE", "demo-biz")
	t.Setenv("HUB_SYNC", "")
	cfg := loadHubClientConfig("")
	if cfg == nil {
		t.Fatal("expected config from JWT env")
	}
	if cfg.Token != "jwt-abc" || cfg.BusinessCode != "demo-biz" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestLoadHubClientConfigFromWorkdirFile(t *testing.T) {
	t.Setenv("HUB_API_KEY", "")
	t.Setenv("HUB_TOKEN", "")
	t.Setenv("HUB_JWT", "")
	t.Setenv("HUB_BUSINESS_CODE", "")
	t.Setenv("HUB_BUSINESS", "")
	t.Setenv("HUB_SYNC", "")
	dir := t.TempDir()
	my := filepath.Join(dir, ".mycompany")
	if err := os.MkdirAll(my, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"token":"file-jwt","business_code":"file-biz","base_url":"https://from.file"}`
	if err := os.WriteFile(filepath.Join(my, "hub-client.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := loadHubClientConfig(dir)
	if cfg == nil {
		t.Fatal("expected config from hub-client.json")
	}
	if cfg.Token != "file-jwt" || cfg.BusinessCode != "file-biz" {
		t.Fatalf("got %+v", cfg)
	}
}

func TestReportBranchToHubSkipsWithoutConfig(t *testing.T) {
	t.Setenv("HUB_API_KEY", "")
	t.Setenv("HUB_TOKEN", "")
	t.Setenv("HUB_JWT", "")
	t.Setenv("HUB_BUSINESS_CODE", "")
	t.Setenv("HUB_BUSINESS", "")
	t.Setenv("HUB_SYNC", "")
	note := reportBranchToHub(context.Background(), "", "", "feature/x", "abc", "task", "T1", "w1")
	if !strings.Contains(note, "skipped") && !strings.Contains(note, "disabled") {
		t.Fatalf("expected skip/disabled note, got %q", note)
	}
}

func TestReportBranchToHubSkipsEmptyBranch(t *testing.T) {
	t.Setenv("HUB_API_KEY", "")
	t.Setenv("HUB_TOKEN", "jwt")
	t.Setenv("HUB_BUSINESS_CODE", "b")
	t.Setenv("HUB_SYNC", "")
	note := reportBranchToHub(context.Background(), "", "", "", "abc", "task", "T1", "w1")
	if !strings.Contains(note, "empty branch") {
		t.Fatalf("got %q", note)
	}
}

func TestReportBranchKillSwitch(t *testing.T) {
	t.Setenv("HUB_TOKEN", "jwt")
	t.Setenv("HUB_BUSINESS_CODE", "b")
	t.Setenv("HUB_SYNC", "0")
	note := reportBranchToHub(context.Background(), "", "", "feature/x", "abc", "task", "T1", "w1")
	if !strings.Contains(note, "disabled") {
		t.Fatalf("expected disabled, got %q", note)
	}
}
