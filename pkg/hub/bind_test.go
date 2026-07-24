package hub

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/toustifer/agentflow/pkg/engine"
)

func TestBindNamespaceTeam_WritesNSAndWorkdir(t *testing.T) {
	clearHubCodeEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// home has JWT but different fallback code
	if err := os.MkdirAll(filepath.Join(home, ".agent-hub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".agent-hub", "config.json"),
		[]byte(`{"token":"jwt-test","business_code":"zk9a"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	wd := t.TempDir()
	e, err := engine.NewEngine(engine.NewEngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	ctx := context.Background()
	_, err = e.CreateNamespace(ctx, engine.CreateNamespaceRequest{
		ID:   "insighttutor",
		Name: "InsightTutor",
		Metadata: map[string]string{
			"workdir": wd,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := BindNamespaceTeam(ctx, e, "insighttutor", "zhiji-z8gw", BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.BusinessCode != "z8gw" {
		t.Fatalf("code=%s", res.BusinessCode)
	}
	if res.Source != "namespace" {
		t.Fatalf("source=%s", res.Source)
	}
	if !res.WorkdirWrote {
		t.Fatal("expected workdir write")
	}
	// home already had a code and SetHomeFallback false → should not overwrite
	if res.HomeWrote {
		t.Fatal("should not overwrite existing home code without flag")
	}

	ns, err := e.GetNamespace(ctx, "insighttutor")
	if err != nil {
		t.Fatal(err)
	}
	if ns.Metadata[MetaBusinessCode] != "z8gw" {
		t.Fatalf("meta=%v", ns.Metadata)
	}
	if ns.Metadata["workdir"] != wd {
		t.Fatalf("workdir wiped: %v", ns.Metadata)
	}

	raw, err := os.ReadFile(filepath.Join(wd, ".mycompany", "hub-client.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]string
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	if file["business_code"] != "z8gw" {
		t.Fatalf("workdir file: %v", file)
	}

	// home fallback unchanged
	if HomeBusinessCode() != "zk9a" {
		t.Fatalf("home code changed: %s", HomeBusinessCode())
	}

	// second namespace different code
	_, err = e.CreateNamespace(ctx, engine.CreateNamespaceRequest{
		ID:   "other",
		Name: "Other",
	})
	if err != nil {
		t.Fatal(err)
	}
	res2, err := BindNamespaceTeam(ctx, e, "other", "4b4w", BindOptions{SetHomeFallback: true})
	if err != nil {
		t.Fatal(err)
	}
	if res2.BusinessCode != "4b4w" || !res2.HomeWrote {
		t.Fatalf("res2=%+v", res2)
	}
	ns1, _ := e.GetNamespace(ctx, "insighttutor")
	if ns1.Metadata[MetaBusinessCode] != "z8gw" {
		t.Fatalf("first ns clobbered: %v", ns1.Metadata)
	}
	if HomeBusinessCode() != "4b4w" {
		t.Fatalf("home fallback not updated: %s", HomeBusinessCode())
	}
}

func TestSnapshotForNamespace(t *testing.T) {
	clearHubCodeEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.MkdirAll(filepath.Join(home, ".agent-hub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".agent-hub", "config.json"),
		[]byte(`{"token":"jwt","business_code":"zk9a"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	snap := SnapshotForNamespace(map[string]string{MetaBusinessCode: "z8gw"}, "")
	if snap.BusinessCode != "z8gw" || snap.Source != "namespace" || !snap.Bound {
		t.Fatalf("%+v", snap)
	}
	if snap.NSStoredCode != "z8gw" || snap.HomeCode != "zk9a" {
		t.Fatalf("%+v", snap)
	}
}
