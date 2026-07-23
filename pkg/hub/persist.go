package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// HomeConfigPath returns ~/.agent-hub/config.json (empty if home unknown).
func HomeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".agent-hub", "config.json")
}

// WorkdirClientPath returns {workdir}/.mycompany/hub-client.json.
func WorkdirClientPath(workdir string) string {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return ""
	}
	return filepath.Join(workdir, ".mycompany", "hub-client.json")
}

// homeConfigFile is the on-disk shape shared with agent-hub MCP.
type homeConfigFile struct {
	BaseURL      string `json:"base_url,omitempty"`
	HubURL       string `json:"hub_url,omitempty"`
	Token        string `json:"token,omitempty"`
	APIKey       string `json:"api_key,omitempty"`
	BusinessCode string `json:"business_code,omitempty"`
	LoginAt      string `json:"login_at,omitempty"`
	BoundAt      string `json:"bound_at,omitempty"`
}

func readHomeConfig() (homeConfigFile, error) {
	var cur homeConfigFile
	p := HomeConfigPath()
	if p == "" {
		return cur, nil
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return cur, nil
		}
		return cur, err
	}
	_ = json.Unmarshal(raw, &cur)
	return cur, nil
}

// SaveHomeConfig merges patch into ~/.agent-hub/config.json.
// Empty string values in patch are ignored (do not clear). Use ClearBusinessCode to unbind.
func SaveHomeConfig(patch homeConfigFile) error {
	p := HomeConfigPath()
	if p == "" {
		return os.ErrNotExist
	}
	cur, err := readHomeConfig()
	if err != nil {
		return err
	}
	if patch.BaseURL != "" {
		cur.BaseURL = strings.TrimRight(patch.BaseURL, "/")
	}
	if patch.HubURL != "" {
		cur.HubURL = strings.TrimRight(patch.HubURL, "/")
	}
	if patch.Token != "" {
		cur.Token = patch.Token
	}
	if patch.APIKey != "" {
		cur.APIKey = patch.APIKey
	}
	if patch.BusinessCode != "" {
		cur.BusinessCode = strings.TrimSpace(patch.BusinessCode)
	}
	if patch.LoginAt != "" {
		cur.LoginAt = patch.LoginAt
	}
	if patch.BoundAt != "" {
		cur.BoundAt = patch.BoundAt
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}

// ClearBusinessCode removes bound team from home config (local-only after).
func ClearBusinessCode() error {
	p := HomeConfigPath()
	if p == "" {
		return os.ErrNotExist
	}
	cur, err := readHomeConfig()
	if err != nil {
		return err
	}
	cur.BusinessCode = ""
	cur.BoundAt = ""
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}

// SaveWorkdirClient merges token/business into workdir hub-client.json when workdir set.
func SaveWorkdirClient(workdir string, patch homeConfigFile) error {
	p := WorkdirClientPath(workdir)
	if p == "" {
		return nil
	}
	var cur homeConfigFile
	if raw, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(raw, &cur)
	}
	if patch.BaseURL != "" {
		cur.BaseURL = strings.TrimRight(patch.BaseURL, "/")
	}
	if patch.HubURL != "" {
		cur.HubURL = strings.TrimRight(patch.HubURL, "/")
	}
	if patch.Token != "" {
		cur.Token = patch.Token
	}
	if patch.APIKey != "" {
		cur.APIKey = patch.APIKey
	}
	if patch.BusinessCode != "" {
		cur.BusinessCode = strings.TrimSpace(patch.BusinessCode)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, raw, 0o600)
}
