package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrimCRLF(t *testing.T) {
	if got := trimCRLF("hello\r\n"); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
	if got := trimCRLF("hello\n"); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestPhaseProviderResultToMap(t *testing.T) {
	p := phaseProviderResult{
		Phase:          "execute",
		PhaseName:      "执行中（1/3）",
		Progress:       "33%",
		Actions:        []string{"task_transition start"},
		NextTasks:      []map[string]any{{"task_id": "t1"}},
		HasNextTasks:   true,
		HasActiveTasks: false,
		HasStuckTasks:  false,
	}
	m := p.toMap()
	if m["phase"] != "execute" {
		t.Fatalf("wrong phase: %#v", m)
	}
	if m["has_next_tasks"] != true {
		t.Fatalf("wrong flags: %#v", m)
	}
}

func TestBTRPCResponseUnmarshal(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":7,"result":{"status":"success"}}`
	var resp btRPCResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.JSONRPC != btRPCVersion || resp.ID != 7 {
		t.Fatalf("bad response parse: %#v", resp)
	}
}

func TestFindBTScript(t *testing.T) {
	args := findBTScript()
	if len(args) != 2 || args[0] != "-m" || args[1] != "bt_service" {
		t.Fatalf("unexpected bt script args: %#v", args)
	}
}

func TestFindPythonReturnsSomething(t *testing.T) {
	p := findPython()
	if strings.TrimSpace(p) == "" {
		t.Fatal("findPython returned empty string")
	}
}
