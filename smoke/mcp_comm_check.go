package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	agentflow := os.Getenv("AGENTFLOW_BIN")
	if agentflow == "" {
		agentflow = `C:\Users\15775\AppData\Local\Temp\agentflow.exe`
	}
	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("agentflow-smoke-%d.db", time.Now().UnixNano()))
	defer os.Remove(dbPath)
	cmd := exec.Command(agentflow, "stdio")
	cmd.Env = append(os.Environ(), "AGENTFLOW_DB_PATH="+dbPath)
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
		isError, _ := result["isError"].(bool)
		if content, ok := result["content"].([]any); ok && len(content) > 0 {
			if cm, ok := content[0].(map[string]any); ok {
				if text, ok := cm["text"].(string); ok {
					var v map[string]any
					if err := json.Unmarshal([]byte(text), &v); err == nil {
						return v
					}
					if isError {
						return map[string]any{"_error": text}
					}
				}
			}
		}
		if isError {
			return map[string]any{"_error": "tool call failed"}
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

	// 1. Bootstrap from an existing empty directory via project_init.
	tmpDir, _ := os.MkdirTemp("", "agentflow-smoke-*")
	defer os.RemoveAll(tmpDir)

	r := toolCall("project_init", map[string]any{
		"project_id": "comms-test",
		"workdir":    tmpDir,
		"init_git":   true,
		"user_name":  "Test",
		"user_email": "test@example.com",
	})
	initProject := extract(r)
	check("project_init", initProject != nil && initProject["namespace_id"] == "comms-test", fmt.Sprintf("%v", initProject))
	check("project_init has_head_commit=false", initProject != nil && initProject["has_head_commit"] == false, fmt.Sprintf("has_head=%v", initProject["has_head_commit"]))
	if initProject != nil {
		if rulesPath, _ := initProject["rules_file_path"].(string); rulesPath != "" {
			_, err := os.Stat(rulesPath)
			check("project_init writes agentflow-git.md", err == nil, fmt.Sprintf("path=%q err=%v", rulesPath, err))
		} else {
			check("project_init writes agentflow-git.md", false, "missing rules_file_path")
		}
	}

	r = toolCall("project_next_steps", map[string]any{"namespace_id": "comms-test", "workdir": tmpDir})
	ns := extract(r)
	check("project_next_steps requires initial commit", ns != nil && ns["phase"] == "setup" && ns["phase_name"] == "等待首个 commit", fmt.Sprintf("%v", ns))

	// Register workers early so we can verify the no-HEAD failure path cleanly.
	r = toolCall("worker_register", map[string]any{"namespace_id": "comms-test", "worker_id": "worker-ui", "name": "Worker UI", "prompt_template": "Task {task_id} in {worktree_path} on {branch}"})
	check("worker_register", extract(r) != nil, fmt.Sprintf("%v", extract(r)))
	r = toolCall("worker_register", map[string]any{"namespace_id": "comms-test", "worker_id": "reviewer-ui", "name": "Reviewer UI", "prompt_template": "rev_commit={review_commit} rev_diff={review.diff} task={task_id}"})
	check("worker_register reviewer", extract(r) != nil, fmt.Sprintf("%v", extract(r)))
	r = toolCall("dag_create", map[string]any{"namespace_id": "comms-test", "dag_id": "dag-bootstrap", "title": "Bootstrap Failure DAG", "branch": "feat/bootstrap"})
	check("dag_create bootstrap", extract(r) != nil, fmt.Sprintf("%v", extract(r)))
	r = toolCall("task_create", map[string]any{"namespace_id": "comms-test", "task_id": "T-bootstrap", "title": "bootstrap task", "assigned_worker": "worker-ui", "dag_id": "dag-bootstrap"})
	bootstrapTask := extract(r)
	check("task_create bootstrap", bootstrapTask != nil && bootstrapTask["state"] == "assigned", fmt.Sprintf("%v", bootstrapTask))
	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T-bootstrap", "transition": "start", "actor_role": "leader"})
	bootstrapStart := extract(r)
	check("start rejects repo without initial commit", bootstrapStart != nil && strings.Contains(fmt.Sprint(bootstrapStart["_error"]), "还没有首个 commit"), fmt.Sprintf("%v", bootstrapStart))

	// Seed the first commit so worktree-based lifecycle can proceed.
	os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# smoke\n"), 0o644)
	runGit(tmpDir, "add", ".")
	runGit(tmpDir, "commit", "-m", "init")

	r = toolCall("project_next_steps", map[string]any{"namespace_id": "comms-test", "workdir": tmpDir})
	ns = extract(r)
	check("project_next_steps advances after initial commit", ns != nil && ns["phase"] == "execute", fmt.Sprintf("%v", ns))

	// Register worker and DAG.
	r = toolCall("dag_create", map[string]any{"namespace_id": "comms-test", "dag_id": "dag-1", "title": "Smoke DAG", "branch": "feat/test"})
	check("dag_create", extract(r) != nil, fmt.Sprintf("%v", extract(r)))
	r = toolCall("dag_create", map[string]any{"namespace_id": "comms-test", "dag_id": "dag-2", "title": "Rework DAG", "branch": "feat/test2"})
	check("dag_create dag-2", extract(r) != nil, fmt.Sprintf("%v", extract(r)))
	r = toolCall("dag_create", map[string]any{"namespace_id": "comms-test", "dag_id": "dag-3", "title": "Compat DAG", "branch": "feat/test3"})
	check("dag_create dag-3", extract(r) != nil, fmt.Sprintf("%v", extract(r)))

	// 2. Create task — verify available_transitions appears.
	r = toolCall("task_create", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "title": "Hello World 页面", "assigned_worker": "worker-ui", "dag_id": "dag-1", "acceptance_criteria": []any{"页面包含标题", "按钮可点击弹出问候语"}})
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
	check("start (leader) -> executing", t["state"] == "executing", fmt.Sprintf("state=%v error=%v", t["state"], t["_error"]))
	if arr, ok := t["available_transitions"].([]any); ok {
		check("executing avail has 3 entries", len(arr) == 3,
			fmt.Sprintf("got %d: %v", len(arr), arr))
	}

	// Prepare worktree: get worktree path, write file, commit, write diary.
	taskDetail := toolCall("task_get", map[string]any{"namespace_id": "comms-test", "task_id": "T1"})
	td := extract(taskDetail)
	if td != nil {
		rawMeta, _ := td["metadata"]
		if meta, ok := rawMeta.(map[string]any); ok {
			wtPath, _ := meta["git.worktree_path"].(string)
			check("worktree_path T1", wtPath != "", fmt.Sprintf("path=%q", wtPath))
			if wtPath != "" {
				os.WriteFile(filepath.Join(wtPath, "work.txt"), []byte("done"), 0o644)
				runGit(wtPath, "add", ".")
				runGit(wtPath, "commit", "-m", "implement T1")
				check("git commit T1", true, "")
			}
		} else {
			check("worktree_path T1", false, "no metadata map")
		}
	} else {
		check("worktree_path T1", false, "task_get returned nil")
	}
	diaryDate := time.Now().UTC().Format("2006-01-02")
	r = toolCall("worker_diary_write", map[string]any{"namespace_id": "comms-test", "worker_id": "worker-ui", "date": diaryDate, "content": "finished T1", "task_id": "T1"})
	check("diary T1", extract(r) != nil, fmt.Sprintf("%v", extract(r)))

	// 5. Worker submits.
	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "transition": "submit", "actor_role": "worker"})
	t = extract(r)
	check("submit (worker) -> review_pending", t["state"] == "review_pending", fmt.Sprintf("state=%v error=%v", t["state"], t["_error"]))

	// 6. Reviewer passes.
	r = toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T1", "transition": "pass", "actor_role": "reviewer"})
	t = extract(r)
	check("pass (reviewer) -> done", t["state"] == "done", fmt.Sprintf("state=%v error=%v", t["state"], t["_error"]))
	check("done has nil avail", t["available_transitions"] == nil, fmt.Sprintf("%v error=%v", t["available_transitions"], t["_error"]))

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
	r = toolCall("task_create", map[string]any{"namespace_id": "comms-test", "task_id": "T2", "title": "返工测试", "assigned_worker": "worker-ui", "dag_id": "dag-2"})
	extract(r)
	toolCall("task_transition", map[string]any{"namespace_id": "comms-test", "task_id": "T2", "transition": "start", "actor_role": "leader"})
	taskDetail2 := toolCall("task_get", map[string]any{"namespace_id": "comms-test", "task_id": "T2"})
	td2 := extract(taskDetail2)
	if td2 != nil {
		rawMeta2, _ := td2["metadata"]
		if meta2, ok := rawMeta2.(map[string]any); ok {
			if wt2, _ := meta2["git.worktree_path"].(string); wt2 != "" {
				os.WriteFile(filepath.Join(wt2, "work2.txt"), []byte("done"), 0o644)
				runGit(wt2, "add", ".")
				runGit(wt2, "commit", "-m", "implement T2")
			}
		}
	}
	toolCall("worker_diary_write", map[string]any{"namespace_id": "comms-test", "worker_id": "worker-ui", "date": diaryDate, "content": "finished T2", "task_id": "T2"})
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
	r = toolCall("task_create", map[string]any{"namespace_id": "comms-test", "task_id": "T3", "title": "兼容测试", "assigned_worker": "worker-ui", "dag_id": "dag-3"})
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

func runGit(dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "git %v failed: %s\n", args, string(out))
		os.Exit(1)
	}
}
