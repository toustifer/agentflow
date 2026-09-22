package hub

import (
	"path/filepath"
	"testing"
)

// TestLoadPrecedenceEnvBeatsWorkdirBeatsHome pins the documented layering:
// every slot is filled by env first, then the workdir file, then home.
func TestLoadPrecedenceEnvBeatsWorkdirBeatsHome(t *testing.T) {
	home := isolateHubEnv(t)
	workdir := t.TempDir()

	writeHubFile(t, filepath.Join(home, ".agent-hub", "config.json"),
		`{"base_url":"https://home.invalid","token":"home-token","api_key":"home-key"}`)
	writeHubFile(t, WorkdirClientPath(workdir),
		`{"base_url":"https://workdir.invalid","token":"workdir-token","api_key":"workdir-key","business_code":"4b4w"}`)

	// Layer 3 only: home supplies credentials, workdir supplies the team code.
	cfg := Load(workdir)
	if cfg.Token != "workdir-token" {
		t.Fatalf("workdir token must beat home: %q", cfg.Token)
	}
	if cfg.APIKey != "workdir-key" {
		t.Fatalf("workdir api_key must beat home: %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://workdir.invalid" {
		t.Fatalf("workdir base_url must beat the built-in default: %q", cfg.BaseURL)
	}
	if cfg.BusinessCode != "4b4w" {
		t.Fatalf("workdir business_code: %q", cfg.BusinessCode)
	}
	if cfg.Source != WorkdirClientPath(workdir) {
		t.Fatalf("credential source should point at the workdir file: %q", cfg.Source)
	}
	if cfg.BusinessCodeSource != WorkdirClientPath(workdir) {
		t.Fatalf("code source should point at the workdir file: %q", cfg.BusinessCodeSource)
	}

	// Layer 2 only: no workdir file -> home credentials are used. The team code
	// has to come from env, since home never supplies one.
	bare := t.TempDir()
	t.Setenv("HUB_BUSINESS_CODE", "4b4w")
	homeCfg := Load(bare)
	if homeCfg.Token != "home-token" {
		t.Fatalf("home token fallback: %q", homeCfg.Token)
	}
	if homeCfg.APIKey != "home-key" {
		t.Fatalf("home api_key fallback: %q", homeCfg.APIKey)
	}
	if homeCfg.BaseURL != "https://home.invalid" {
		t.Fatalf("home base_url fallback: %q", homeCfg.BaseURL)
	}
	if !homeCfg.Enabled() {
		t.Fatalf("env code + home credentials must be enabled: %+v", homeCfg)
	}
	if homeCfg.Source != HomeConfigPath() {
		t.Fatalf("credential source should point at home: %q", homeCfg.Source)
	}
	if homeCfg.BusinessCodeSource != "env" {
		t.Fatalf("code source should be env: %q", homeCfg.BusinessCodeSource)
	}

	// Incomplete config: no team code anywhere, so nothing is usable — but the
	// credential origin is still reported for diagnostics.
	t.Setenv("HUB_BUSINESS_CODE", "")
	incomplete := Load(bare)
	if incomplete.Enabled() {
		t.Fatalf("config without a team code must not be enabled: %+v", incomplete)
	}
	if incomplete.BusinessCodeSource != "" {
		t.Fatalf("unbound code source=%q want empty", incomplete.BusinessCodeSource)
	}
	if incomplete.Source != HomeConfigPath() {
		t.Fatalf("credential source=%q want the home file", incomplete.Source)
	}

	// Layer 1: env beats both files.
	t.Setenv("HUB_TOKEN", "env-token")
	t.Setenv("HUB_API_KEY", "env-key")
	t.Setenv("HUB_BASE_URL", "https://env.invalid")
	envCfg := Load(workdir)
	if envCfg.Token != "env-token" || envCfg.APIKey != "env-key" {
		t.Fatalf("env must win: %+v", envCfg)
	}
	if envCfg.BaseURL != "https://env.invalid" {
		t.Fatalf("env base url must win: %q", envCfg.BaseURL)
	}
	if envCfg.Source != "env" {
		t.Fatalf("source=env: %q", envCfg.Source)
	}
	// Credentials came from env, the team code still came from the workdir file.
	if envCfg.BusinessCodeSource != WorkdirClientPath(workdir) {
		t.Fatalf("code source=%q want the workdir file", envCfg.BusinessCodeSource)
	}
}

// TestLoadBusinessCodeNeverComesFromHome pins the master invariant: home is a
// JWT-only file, so its business_code is ignored (two namespaces on one machine
// must not fight over one Hub team).
func TestLoadBusinessCodeNeverComesFromHome(t *testing.T) {
	home := isolateHubEnv(t)
	writeHubFile(t, filepath.Join(home, ".agent-hub", "config.json"),
		`{"token":"home-token","business_code":"zk9a"}`)

	cfg := Load("")
	if cfg.Token != "home-token" {
		t.Fatalf("home token must still be honoured: %q", cfg.Token)
	}
	if cfg.BusinessCode != "" {
		t.Fatalf("home business_code must be ignored, got %q", cfg.BusinessCode)
	}
	if cfg.Enabled() {
		t.Fatal("token without a team code must not be enabled")
	}
	if cfg.BusinessCodeSource != "" {
		t.Fatalf("home must not supply a code source: %q", cfg.BusinessCodeSource)
	}
	if cfg.Source != HomeConfigPath() {
		t.Fatalf("the token still came from home: %q", cfg.Source)
	}
}

// TestLoadBusinessCodeEnvBeatsWorkdir covers the code slot explicitly.
func TestLoadBusinessCodeEnvBeatsWorkdir(t *testing.T) {
	isolateHubEnv(t)
	workdir := t.TempDir()
	writeHubFile(t, WorkdirClientPath(workdir), `{"business_code":"4b4w","token":"t"}`)

	if got := Load(workdir).BusinessCode; got != "4b4w" {
		t.Fatalf("workdir code: %q", got)
	}
	t.Setenv("HUB_BUSINESS_CODE", "aryd")
	if got := Load(workdir).BusinessCode; got != "aryd" {
		t.Fatalf("env code must win: %q", got)
	}
	// HUB_BUSINESS is the documented alias.
	t.Setenv("HUB_BUSINESS_CODE", "")
	t.Setenv("HUB_BUSINESS", "z8gw")
	if got := Load(workdir).BusinessCode; got != "z8gw" {
		t.Fatalf("HUB_BUSINESS alias: %q", got)
	}
}

// TestLoadForNamespaceUsesNamespaceMetadata covers the namespace-aware entry
// point that Load itself cannot see.
func TestLoadForNamespaceUsesNamespaceMetadata(t *testing.T) {
	isolateHubEnv(t)
	workdir := t.TempDir()
	writeHubFile(t, WorkdirClientPath(workdir), `{"business_code":"4b4w"}`)

	// Namespace metadata beats the workdir file (ResolveBusinessCode order).
	cfg := LoadForNamespace(map[string]string{MetaBusinessCode: "z8gw", "workdir": workdir}, "")
	if cfg.BusinessCode != "z8gw" {
		t.Fatalf("namespace code: %q", cfg.BusinessCode)
	}
	// env still beats namespace.
	t.Setenv("HUB_BUSINESS_CODE", "aryd")
	if got := LoadForNamespace(map[string]string{MetaBusinessCode: "z8gw"}, "").BusinessCode; got != "aryd" {
		t.Fatalf("env must beat namespace: %q", got)
	}
}

func TestConfigEnabledRequiresCodeAndCredential(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"code+token", Config{BusinessCode: "z8gw", Token: "jwt"}, true},
		{"code+api_key", Config{BusinessCode: "z8gw", APIKey: "k"}, true},
		{"code only", Config{BusinessCode: "z8gw"}, false},
		{"token only", Config{Token: "jwt"}, false},
		{"blank spaces", Config{BusinessCode: "  ", Token: "  "}, false},
		{"killed with full creds", Config{BusinessCode: "z8gw", Token: "jwt", Disabled: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Fatalf("Enabled()=%v want %v", got, tc.want)
			}
		})
	}
	var nilCfg *Config
	if nilCfg.Enabled() || nilCfg.HasJWT() || nilCfg.HasAPIKey() {
		t.Fatal("nil *Config must be inert")
	}
}

// TestKillSwitchDisablesEvenWithFullCredentials pins the总开关: every documented
// spelling of "off" wins over present credentials.
func TestKillSwitchDisablesEvenWithFullCredentials(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  string
	}{
		{"HUB_SYNC=0", "HUB_SYNC", "0"},
		{"HUB_SYNC=false", "HUB_SYNC", "false"},
		{"HUB_SYNC=off", "HUB_SYNC", "off"},
		{"HUB_DISABLED=1", "HUB_DISABLED", "1"},
		{"HUB_DISABLED=true", "HUB_DISABLED", "true"},
		{"HUB_ENABLED=0", "HUB_ENABLED", "0"},
		{"HUB_ENABLED=false", "HUB_ENABLED", "false"},
		{"HUB_ENABLED=off", "HUB_ENABLED", "off"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHubEnv(t)
			workdir := t.TempDir()
			writeHubFile(t, WorkdirClientPath(workdir), `{"business_code":"z8gw","token":"jwt"}`)
			t.Setenv(tc.key, tc.val)

			cfg := Load(workdir)
			if !cfg.Disabled {
				t.Fatalf("%s=%s must set Disabled", tc.key, tc.val)
			}
			if cfg.Enabled() {
				t.Fatal("killed config must not be enabled")
			}
			if cfg.Source != "disabled" {
				t.Fatalf("source=%q want disabled", cfg.Source)
			}
			if cfg.BaseURL == "" {
				t.Fatal("base url must still be populated for diagnostics")
			}
		})
	}
}

// TestKillSwitchOffValuesAreInert: a truthy/empty value must not disable.
func TestKillSwitchOffValuesAreInert(t *testing.T) {
	for _, tc := range []struct{ key, val string }{
		{"HUB_SYNC", "1"}, {"HUB_ENABLED", "1"}, {"HUB_ENABLED", "true"},
		{"HUB_DISABLED", "0"}, {"HUB_DISABLED", "false"},
	} {
		t.Run(tc.key+"="+tc.val, func(t *testing.T) {
			isolateHubEnv(t)
			workdir := t.TempDir()
			writeHubFile(t, WorkdirClientPath(workdir), `{"business_code":"z8gw","token":"jwt"}`)
			t.Setenv(tc.key, tc.val)
			if cfg := Load(workdir); cfg.Disabled || !cfg.Enabled() {
				t.Fatalf("%s=%s must stay enabled: %+v", tc.key, tc.val, cfg)
			}
		})
	}
}

// TestLoadMalformedFileIsIgnored: a corrupt JSON file must not panic or win.
func TestLoadMalformedFileIsIgnored(t *testing.T) {
	isolateHubEnv(t)
	workdir := t.TempDir()
	writeHubFile(t, WorkdirClientPath(workdir), `{not json`)
	cfg := Load(workdir)
	if cfg.Enabled() {
		t.Fatal("malformed file must not enable Hub")
	}
	if cfg.BaseURL == "" {
		t.Fatal("base url must always be populated")
	}
}

// TestLoadDefaultBaseURLUsesConstant keeps tests free of the production literal
// while still pinning the fallback.
func TestLoadDefaultBaseURLUsesConstant(t *testing.T) {
	isolateHubEnv(t)
	cfg := Load("")
	if cfg.BaseURL != DefaultBaseURL {
		t.Fatalf("BaseURL=%q want %q", cfg.BaseURL, DefaultBaseURL)
	}
}
