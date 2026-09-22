package hub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DefaultBaseURL is the production Hub. Only used when nothing else supplies one.
const DefaultBaseURL = "https://hub.stifer.xyz"

// Config is the optional Hub federation credential set.
//
// Empty / disabled config means agentflow runs fully local with zero Hub I/O.
// Construct via LoadForWorkdir or LoadForNamespace; the zero value is a valid
// "disabled" config.
type Config struct {
	BaseURL      string
	Token        string // JWT, preferred credential
	APIKey       string // optional machine credential
	BusinessCode string
	// Disabled is true when the env kill-switch is on (even if files hold credentials).
	Disabled bool
	// Source is where the credential (token/api_key) came from: "env", the config
	// file path that supplied it, "none" when no credential was found at all, or
	// "disabled" when the kill-switch won.
	Source string
	// BusinessCodeSource is where the team code came from: "env", "namespace"
	// (LoadForNamespace only), the workdir config path, or "" when unbound.
	// ~/.agent-hub/config.json never contributes a team code.
	BusinessCodeSource string
	// Workdir is the directory whose .mycompany/hub-client.json was consulted.
	Workdir string
}

// Enabled reports whether Hub soft-sync may run: a business_code AND (token OR
// api_key), and the kill-switch off. Everything else is a soft skip.
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

// HasJWT reports whether a human/MCP login token is present.
func (c *Config) HasJWT() bool {
	return c != nil && strings.TrimSpace(c.Token) != ""
}

// HasAPIKey reports whether a machine API key is present.
func (c *Config) HasAPIKey() bool {
	return c != nil && strings.TrimSpace(c.APIKey) != ""
}

// Load reads Hub client config for a workdir.
//
// Kill switch (any of, and it always wins):
//
//	HUB_SYNC=0|false|off|no|disabled
//	HUB_DISABLED=1|true|on|yes|disabled
//	HUB_ENABLED=0|false|off|no|disabled
//
// Credential layering — env wins, then workdir, then home; a file only fills a
// slot env left empty. The layers are, in order: env (HUB_BASE_URL,
// HUB_TOKEN|HUB_JWT, HUB_API_KEY, HUB_BUSINESS_CODE|HUB_BUSINESS), then
// {workdir}/.mycompany/hub-client.json, then ~/.agent-hub/config.json.
//
// It never panics, never dials, and never requires Hub to be reachable.
//
// business_code is deliberately NOT read from ~/.agent-hub/config.json. That
// file is JWT-only by product design: home is machine-wide, so honouring its
// team code would make two namespaces on one machine fight over one Hub team.
// The namespace/workdir bind (BindNamespaceTeam, meta "hub.business_code") is
// the source of truth — use LoadForNamespace when a namespace is in hand.
func Load(workdir string) *Config {
	return loadConfig(workdir, "")
}

// LoadForNamespace resolves the team code from namespace metadata first
// (env > namespace metadata > workdir file, via ResolveBusinessCode) and layers
// credentials on top of Load.
//
// This is the entry point for call sites that know the namespace: it is the
// only way Load-style config can see the namespace-bound team code.
// env HUB_BUSINESS_CODE still wins, matching ResolveBusinessCode.
func LoadForNamespace(nsMeta map[string]string, workdir string) *Config {
	code, _ := ResolveBusinessCode(nsMeta, workdir)
	return loadConfig(workdir, code)
}

func loadConfig(workdir, nsCode string) *Config {
	workdir = strings.TrimSpace(workdir)
	cfg := &Config{
		BaseURL:      strings.TrimRight(strings.TrimSpace(os.Getenv("HUB_BASE_URL")), "/"),
		Token:        firstNonEmpty(os.Getenv("HUB_TOKEN"), os.Getenv("HUB_JWT")),
		APIKey:       strings.TrimSpace(os.Getenv("HUB_API_KEY")),
		BusinessCode: firstNonEmpty(os.Getenv("HUB_BUSINESS_CODE"), os.Getenv("HUB_BUSINESS")),
		Workdir:      workdir,
	}
	envBase, envToken := cfg.BaseURL, cfg.Token
	envKey, envCode := cfg.APIKey, cfg.BusinessCode
	credSource := ""
	if envToken != "" || envKey != "" {
		credSource = "env"
	}
	if envCode != "" {
		cfg.BusinessCodeSource = "env"
	}

	if killSwitchOn() {
		cfg.Disabled = true
		cfg.Source = "disabled"
		if cfg.BaseURL == "" {
			cfg.BaseURL = DefaultBaseURL
		}
		return cfg
	}
	if nsCode = strings.TrimSpace(nsCode); nsCode != "" && cfg.BusinessCode == "" {
		cfg.BusinessCode = nsCode
		cfg.BusinessCodeSource = "namespace"
	}

	for _, p := range layeredConfigPaths(workdir) {
		fromHome := !isWorkdirConfigPath(p) // home business_code is ignored by design
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
		if envBase == "" && cfg.BaseURL == "" {
			if base := firstNonEmpty(file.BaseURL, file.HubURL); base != "" {
				cfg.BaseURL = strings.TrimRight(base, "/")
			}
		}
		if envToken == "" && cfg.Token == "" && strings.TrimSpace(file.Token) != "" {
			cfg.Token = strings.TrimSpace(file.Token)
			if credSource == "" {
				credSource = p
			}
		}
		if envKey == "" && cfg.APIKey == "" && strings.TrimSpace(file.APIKey) != "" {
			cfg.APIKey = strings.TrimSpace(file.APIKey)
			if credSource == "" {
				credSource = p
			}
		}
		if !fromHome && envCode == "" && cfg.BusinessCode == "" && strings.TrimSpace(file.BusinessCode) != "" {
			cfg.BusinessCode = strings.TrimSpace(file.BusinessCode)
			cfg.BusinessCodeSource = p
		}
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if credSource == "" {
		credSource = "none"
	}
	cfg.Source = credSource
	return cfg
}

// layeredConfigPaths returns workdir config first, then home (env is handled above).
func layeredConfigPaths(workdir string) []string {
	paths := make([]string, 0, 2)
	if p := WorkdirClientPath(workdir); p != "" {
		paths = append(paths, p)
	}
	if p := HomeConfigPath(); p != "" {
		paths = append(paths, p)
	}
	return paths
}

func isWorkdirConfigPath(p string) bool {
	// Both end in the same filename; workdir paths always contain the .mycompany segment.
	return strings.Contains(filepath.ToSlash(p), "/.mycompany/")
}

// killSwitchOn reports whether an env variable forces Hub I/O off.
func killSwitchOn() bool {
	if truthyDisable(os.Getenv("HUB_SYNC")) {
		return true
	}
	if truthyEnable(os.Getenv("HUB_DISABLED")) {
		return true
	}
	// HUB_ENABLED: empty means "auto by credentials"; an explicit falsy value kills I/O.
	if v := os.Getenv("HUB_ENABLED"); strings.TrimSpace(v) != "" && truthyDisable(v) {
		return true
	}
	return false
}

// truthyDisable reports whether v is an explicit "off".
func truthyDisable(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "off", "no", "disabled":
		return true
	default:
		return false
	}
}

// truthyEnable reports whether v is an explicit "on" (used for HUB_DISABLED,
// where the affirmative value means "do disable").
func truthyEnable(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes", "disabled":
		return true
	default:
		return false
	}
}
