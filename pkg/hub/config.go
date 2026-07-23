package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config is optional Hub collaboration credentials.
// Empty / disabled config means agentflow runs fully local (no Hub I/O).
type Config struct {
	BaseURL      string
	Token        string // JWT preferred
	APIKey       string // optional machine path
	BusinessCode string
	// Disabled is true when env kill-switch is on (even if files have credentials).
	Disabled bool
	// Source describes how credentials were resolved (for notes/debug).
	Source string
}

// Enabled reports whether Hub soft-sync may run.
// Decoupled default: false unless business_code + (token|api_key) and not killed.
func (c *Config) Enabled() bool {
	if c == nil || c.Disabled {
		return false
	}
	if strings.TrimSpace(c.BusinessCode) == "" {
		return false
	}
	if strings.TrimSpace(c.Token) == "" && strings.TrimSpace(c.APIKey) == "" {
		return false
	}
	return true
}

// HasJWT is true when human/MCP login token is present.
func (c *Config) HasJWT() bool {
	return c != nil && strings.TrimSpace(c.Token) != ""
}

// Load reads Hub client config without ever panicking or requiring Hub.
//
// Kill switches (any): HUB_SYNC=0|false|off, HUB_DISABLED=1|true, HUB_ENABLED=0|false|off
//
// Credential priority:
//  1. env: HUB_BASE_URL, HUB_TOKEN|HUB_JWT, HUB_API_KEY, HUB_BUSINESS_CODE|HUB_BUSINESS
//  2. {workdir}/.mycompany/hub-client.json
//  3. ~/.agent-hub/config.json (MCP hub_login / future agentflow login)
//
// File fields only fill empty env slots (env wins).
func Load(workdir string) *Config {
	cfg := &Config{
		BaseURL:      strings.TrimRight(os.Getenv("HUB_BASE_URL"), "/"),
		Token:        firstNonEmpty(os.Getenv("HUB_TOKEN"), os.Getenv("HUB_JWT")),
		APIKey:       os.Getenv("HUB_API_KEY"),
		BusinessCode: firstNonEmpty(os.Getenv("HUB_BUSINESS_CODE"), os.Getenv("HUB_BUSINESS")),
		Source:       "env",
	}
	if killSwitchOn() {
		cfg.Disabled = true
		cfg.Source = "disabled"
		return cfg
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://hub.stifer.xyz"
	}

	candidates := make([]string, 0, 2)
	if workdir != "" {
		candidates = append(candidates, filepath.Join(workdir, ".mycompany", "hub-client.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".agent-hub", "config.json"))
	}
	for _, p := range candidates {
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var file struct {
			BaseURL      string `json:"base_url"`
			HubURL       string `json:"hub_url"`
			Token        string `json:"token"`
			APIKey       string `json:"api_key"`
			BusinessCode string `json:"business_code"`
		}
		if json.Unmarshal(raw, &file) != nil {
			continue
		}
		fileBase := firstNonEmpty(file.BaseURL, file.HubURL)
		if fileBase != "" && (cfg.BaseURL == "" || cfg.BaseURL == "https://hub.stifer.xyz") {
			// Only override default base from file when env did not set a custom base.
			if os.Getenv("HUB_BASE_URL") == "" {
				cfg.BaseURL = strings.TrimRight(fileBase, "/")
			}
		}
		if file.Token != "" && cfg.Token == "" {
			cfg.Token = file.Token
			cfg.Source = p
		}
		if file.APIKey != "" && cfg.APIKey == "" {
			cfg.APIKey = file.APIKey
			if cfg.Source == "env" || cfg.Source == "" {
				cfg.Source = p
			}
		}
		if file.BusinessCode != "" && cfg.BusinessCode == "" {
			cfg.BusinessCode = file.BusinessCode
			if cfg.Source == "env" || cfg.Source == "" {
				cfg.Source = p
			}
		}
	}
	if !cfg.Enabled() && !cfg.Disabled {
		cfg.Source = "none"
	}
	return cfg
}

func killSwitchOn() bool {
	if truthyDisable(os.Getenv("HUB_SYNC")) {
		return true
	}
	if truthyEnable(os.Getenv("HUB_DISABLED")) {
		return true
	}
	// HUB_ENABLED=0|false|off means disabled; empty means default (auto by credentials).
	if v := strings.TrimSpace(os.Getenv("HUB_ENABLED")); v != "" && truthyDisable(v) {
		return true
	}
	return false
}

// truthyDisable: 0 / false / off / no → treat as "sync off"
func truthyDisable(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "off", "no", "disabled":
		return true
	default:
		return false
	}
}

// truthyEnable: 1 / true / on / yes → treat as flag on
func truthyEnable(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes", "disabled":
		// "disabled" as value of HUB_DISABLED
		return true
	default:
		return false
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
