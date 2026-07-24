package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func clearHubCodeEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HUB_BUSINESS_CODE", "HUB_BUSINESS"} {
		t.Setenv(k, "")
	}
}

func TestNormalizeBusinessCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"z8gw", "z8gw", true},
		{"Z8GW", "z8gw", true},
		{"zhiji-z8gw", "z8gw", true},
		{"insighttutor-z8gw", "z8gw", true},
		{"https://hub.stifer.xyz/team/zhiji-z8gw", "z8gw", true},
		{"zhiji", "", false},
		{"", "", false},
		{"z8gwx", "", false},
		{"ab", "", false},
	}
	for _, tc := range cases {
		got, err := NormalizeBusinessCode(tc.in)
		if tc.ok {
			if err != nil {
				t.Fatalf("Normalize(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("Normalize(%q)=%q want %q", tc.in, got, tc.want)
			}
		} else if err == nil {
			t.Fatalf("Normalize(%q) expected error, got %q", tc.in, got)
		}
	}
}

func TestResolveBusinessCode_Priority(t *testing.T) {
	clearHubCodeEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// home fallback
	if err := os.MkdirAll(filepath.Join(home, ".agent-hub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".agent-hub", "config.json"),
		[]byte(`{"business_code":"zk9a","token":"t"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	// unbound with empty ns / workdir → home
	code, src := ResolveBusinessCode(nil, "")
	if code != "zk9a" || src != "home" {
		t.Fatalf("home only: code=%s src=%s", code, src)
	}

	// workdir beats home
	wd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(wd, ".mycompany"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, ".mycompany", "hub-client.json"),
		[]byte(`{"business_code":"4b4w"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, src = ResolveBusinessCode(nil, wd)
	if code != "4b4w" || src != "workdir" {
		t.Fatalf("workdir: code=%s src=%s", code, src)
	}

	// namespace beats workdir
	meta := map[string]string{
		MetaBusinessCode: "z8gw",
		"workdir":        wd,
	}
	code, src = ResolveBusinessCode(meta, wd)
	if code != "z8gw" || src != "namespace" {
		t.Fatalf("namespace: code=%s src=%s", code, src)
	}

	// env beats namespace
	t.Setenv("HUB_BUSINESS_CODE", "aryd")
	code, src = ResolveBusinessCode(meta, wd)
	if code != "aryd" || src != "env" {
		t.Fatalf("env: code=%s src=%s", code, src)
	}

	// display path in env still normalizes
	t.Setenv("HUB_BUSINESS_CODE", "zhiji-z8gw")
	code, src = ResolveBusinessCode(meta, wd)
	if code != "z8gw" || src != "env" {
		t.Fatalf("env path: code=%s src=%s", code, src)
	}
}

func TestResolveBusinessCode_Unbound(t *testing.T) {
	clearHubCodeEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	code, src := ResolveBusinessCode(nil, "")
	if code != "" || src != "unbound" {
		t.Fatalf("want unbound, got code=%q src=%s", code, src)
	}
}
