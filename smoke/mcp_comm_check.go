package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func main() {
	agentflow := os.Getenv("AGENTFLOW_BIN")
	if agentflow == "" {
		agentflow = `C:\Users\15775\AppData\Local\Temp\agentflow.exe`
	}
	cmd := exec.Command(agentflow, "stdio")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	cmd.Start()

	// Drain stderr in background.
	go func() { io.ReadAll(stderr) }()

	reader := bufio.NewReader(stdout)
	send := func(obj map[string]any) {
		body, _ := json.Marshal(obj)
		fmt.Fprintf(stdin, "Content-Length: %d\r\n\r\n%s", len(body), body)
	}
	recv := func() map[string]any {
		// Read Content-Length header line.
		line, _ := reader.ReadString('\n')
		cl, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:")))
		// Consume blank line.
		for {
			b, _ := reader.ReadByte()
			if b == '\n' {
				break
			}
		}
		// Read body.
		body := make([]byte, cl)
		io.ReadFull(reader, body)
		var v map[string]any
		json.Unmarshal(body, &v)
		return v
	}
	var callID int = 1
	toolCall := func(name string, args map[string]any) map[string]any {
		callID++
		send(map[string]any{"jsonrpc": "2.0", "id": callID, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
		return recv()
	}
	extract := func(resp map[string]any) map[string]any {
		result, _ := resp["result"].(map[string]any)
		if result == nil {
			if e, ok := resp["error"]; ok {
				return map[string]any{"_error": e.(map[string]any)["message"]}
			}
			return nil
		}
		if content, ok := result["content"].([]any); ok && len(content) > 0 {
			if cm, ok := content[0].(map[string]any); ok {
				if text, ok := cm["text"].(string); ok {
					var v map[string]any
					json.Unmarshal([]byte(text), &v)
					return v
				}
			}
		}
		return nil
	}

	// Initialize.
	send(map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "test", "version": "1"}}})
	initResp := recv()
	fmt.Printf("[init] server=%v\n", initResp["result"].(map[string]any)["serverInfo"].(map[string]any)["name"])
	// Skip notifications/initialized — agentflow doesn't require it.

	pass, fail := 0, 0
	check := func(name string, cond bool, detail string) {
		if cond {
			pass++
			fmt.Printf("  PASS [%s] %s\n", name, detail)
		} else {
			fail++
			fmt.Printf("  FAIL [%s] %s\n", name, detail)
		}
	}

	// 1. Create namespace.
	r := toolCall("namespace_create", map[string]any{"id": "comms-test", "name": "通信测试"})
	ns := extract(r)
	check("namespace_create", ns != nil && ns["id"] == "comms-test", fmt.Sprintf("%v", ns))

	// 2. Create task — verify available_transitions appears.
	r = toolCall("task_create", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "title": "Hello World 页面", "assigned_worker": "worker-ui", "acceptance_criteria": []any{"页面包含标题", "按钮可点击弹出问候语"}})
	t := extract(r)
	check("task_create", t["state"] == "assigned", fmt.Sprintf("state=%v", t["state"]))
	avail := t["available_transitions"]
	check("available_transitions on assigned", avail != nil, fmt.Sprintf("avail=%v", avail))
	if arr, ok := avail.([]any); ok {
		check("avail[0].transition=start", len(arr) > 0 && arr[0].(map[string]any)["transition"] == "start",
			fmt.Sprintf("first=%v", arr[0]))
		check("avail[0].role=leader", len(arr) > 0 && arr[0].(map[string]any)["role"] == "leader", "")
	}

	// 3. task_get includes available_transitions.
	r = toolCall("task_get", map[string]any{"namespace_id": "comms-test", "task_id": "T1"})
	t = extract(r)
	avail2 := t["available_transitions"]
	check("task_get has available_transitions", avail2 != nil, fmt.Sprintf("%v", avail2))

	// 4. Leader starts with actor_role.
	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "transition": "start", "actor_role": "leader"})
	t = extract(r)
	check("start (leader) -> executing", t["state"] == "executing", fmt.Sprintf("state=%v", t["state"]))
	if arr, ok := t["available_transitions"].([]any); ok {
		check("executing avail has submit+cancel", len(arr) == 2,
			fmt.Sprintf("got %d: %v", len(arr), arr))
	}

	// 5. Worker submits.
	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "transition": "submit", "actor_role": "worker"})
	t = extract(r)
	check("submit (worker) -> review_pending", t["state"] == "review_pending", fmt.Sprintf("state=%v", t["state"]))

	// 6. Reviewer passes.
	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "transition": "pass", "actor_role": "reviewer"})
	t = extract(r)
	check("pass (reviewer) -> done", t["state"] == "done", fmt.Sprintf("state=%v", t["state"]))
	check("done has nil avail", t["available_transitions"] == nil, fmt.Sprintf("%v", t["available_transitions"]))

	// 7. Negative tests.
	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "transition": "pass", "actor_role": "worker"})
	err := extract(r)
	check("worker cannot pass", err != nil && strings.Contains(fmt.Sprint(err["_error"]), "not allowed"),
		fmt.Sprintf("error=%v", err["_error"]))

	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "transition": "start", "actor_role": "reviewer"})
	err = extract(r)
	check("reviewer cannot start", err != nil && strings.Contains(fmt.Sprint(err["_error"]), "not allowed"),
		fmt.Sprintf("error=%v", err["_error"]))

	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "transition": "start", "actor_role": "alien"})
	err = extract(r)
	check("alien role rejected", err != nil && strings.Contains(fmt.Sprint(err["_error"]), "unknown actor_role"),
		fmt.Sprintf("error=%v", err["_error"]))

	// 8. Full rework cycle.
	r = toolCall("task_create", map[string]any{"namespace_id": "comms-test", "task_id": "T2", "title": "返工测试", "assigned_worker": "worker-ui"})
	extract(r)
	toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T2", "transition": "start", "actor_role": "leader"})
	toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T2", "transition": "submit", "actor_role": "worker"})
	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T2", "transition": "rework", "actor_role": "reviewer"})
	t = extract(r)
	check("rework -> rework_needed", t["state"] == "rework_needed", fmt.Sprintf("state=%v cycle=%v", t["state"], t["review_cycle"]))
	if arr, ok := t["available_transitions"].([]any); ok {
		check("rework avail has 3 entries", len(arr) == 3,
			fmt.Sprintf("got %d: %v", len(arr), arr))
	}

	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T2", "transition": "resume", "actor_role": "leader"})
	t = extract(r)
	check("resume -> executing", t["state"] == "executing", fmt.Sprintf("state=%v", t["state"]))

	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T2", "transition": "cancel", "actor_role": "leader"})
	t = extract(r)
	check("cancel -> cancelled", t["state"] == "cancelled", fmt.Sprintf("state=%v", t["state"]))

	// 9. Backward compat (no actor_role).
	r = toolCall("task_create", map[string]any{"namespace_id": "comms-test", "task_id": "T3", "title": "兼容测试", "assigned_worker": "w"})
	extract(r)
	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T3", "transition": "start"})
	t = extract(r)
	check("backward compat (no actor_role)", t["state"] == "executing", fmt.Sprintf("state=%v", t["state"]))

	stdin.Close()
	cmd.Wait()

	fmt.Printf("\n==============================\n")
	fmt.Printf("Results: %d PASS, %d FAIL of %d\n", pass, fail, pass+fail)
	if fail > 0 {
		fmt.Println("SOME TESTS FAILED!")
		os.Exit(1)
	}
	fmt.Println("ALL TESTS PASSED!")
}
