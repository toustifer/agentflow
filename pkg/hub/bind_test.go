package hub

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/toustifer/agentflow/pkg/engine"
)

func TestBindNamespaceTeam_WritesNSAndWorkdir_NotHome(t *testing.T) {
	clearHubCodeEnv(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// home has JWT + legacy global code — bind must clear legacy, not use it
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
	if !res.HomeCleared {
		t.Fatal("expected home legacy code cleared")
	}
	if HomeBusinessCodeLegacy() != "" {
		t.Fatalf("home legacy still present: %s", HomeBusinessCodeLegacy())
	}
	// token preserved
	if !HasHomeToken() {
		t.Fatal("token should remain")
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

	// second namespace different code — no clash via home
	_, err = e.CreateNamespace(ctx, engine.CreateNamespaceRequest{
		ID:   "other",
		Name: "Other",
	})
	if err != nil {
		t.Fatal(err)
	}
	res2, err := BindNamespaceTeam(ctx, e, "other", "4b4w", BindOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res2.BusinessCode != "4b4w" {
		t.Fatalf("res2=%+v", res2)
	}
	ns1, _ := e.GetNamespace(ctx, "insighttutor")
	if ns1.Metadata[MetaBusinessCode] != "z8gw" {
		t.Fatalf("first ns clobbered: %v", ns1.Metadata)
	}
	// home still no team code
	if HomeBusinessCodeLegacy() != "" {
		t.Fatalf("home should stay team-free: %s", HomeBusinessCodeLegacy())
	}
}

func TestSnapshotForNamespace_IgnoresHome(t *testing.T) {
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

	// unbound ns despite home code
	snap := SnapshotForNamespace(nil, "")
	if snap.Bound || snap.BusinessCode != "" {
		t.Fatalf("must not bind from home: %+v", snap)
	}
	if snap.HomeLegacyCode != "zk9a" {
		t.Fatalf("legacy not reported: %+v", snap)
	}

	snap2 := SnapshotForNamespace(map[string]string{MetaBusinessCode: "z8gw"}, "")
	if snap2.BusinessCode != "z8gw" || snap2.Source != "namespace" || !snap2.Bound {
		t.Fatalf("%+v", snap2)
	}
}
