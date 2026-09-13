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
		agentflow = `.\bin\agentflow.exe`
	}
	dbPath := filepath.Join(os.TempDir(), fmt.Sprintf("agentflow-ndjson-smoke-%d.db", time.Now().UnixNano()))
	defer os.Remove(dbPath)

	cmd := exec.Command(agentflow, "stdio")
	cmd.Env = append(os.Environ(), "AGENTFLOW_DB_PATH="+dbPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdin pipe error: %v\n", err)
		os.Exit(1)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdout pipe error: %v\n", err)
		os.Exit(1)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stderr pipe error: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start error: %v\n", err)
		os.Exit(1)
	}
	go func() { _, _ = io.ReadAll(stderr) }()

	reader := bufio.NewReader(stdout)
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

	// 1. Send initialize via Newline-Delimited JSON (NDJSON).
	fmt.Println("--- Testing Newline-Delimited JSON mode ---")
	reqInit := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	if _, err := io.WriteString(stdin, reqInit); err != nil {
		fmt.Fprintf(os.Stderr, "write error: %v\n", err)
		os.Exit(1)
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}
	check("no content-length header for NDJSON", !strings.Contains(line, "Content-Length:"), line)

	var initResp map[string]any
	err = json.Unmarshal([]byte(strings.TrimSpace(line)), &initResp)
	check("valid JSON response for NDJSON initialize", err == nil, fmt.Sprintf("err=%v", err))
	check("correct response ID", initResp["id"] == float64(1), fmt.Sprintf("id=%v", initResp["id"]))

	// 2. Send tools/list via NDJSON.
	reqTools := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n"
	_, _ = io.WriteString(stdin, reqTools)
	line, err = reader.ReadString('\n')
	check("tools/list NDJSON read", err == nil, "")
	var toolsResp map[string]any
	_ = json.Unmarshal([]byte(strings.TrimSpace(line)), &toolsResp)
	result, _ := toolsResp["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	check("tools list returned", len(tools) > 0, fmt.Sprintf("tool count=%d", len(tools)))

	// 3. Send initialize via Content-Length framing.
	fmt.Println("--- Testing Content-Length mode ---")
	clPayload := `{"jsonrpc":"2.0","id":3,"method":"initialize","params":{}}`
	clReq := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(clPayload), clPayload)
	_, _ = io.WriteString(stdin, clReq)

	// Read Content-Length header.
	clHeader, err := reader.ReadString('\n')
	check("content-length header returned", strings.HasPrefix(clHeader, "Content-Length:"), clHeader)
	clVal, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(clHeader, "Content-Length:")))
	// Read separator blank line.
	for {
		b, _ := reader.ReadByte()
		if b == '\n' {
			break
		}
	}
	body := make([]byte, clVal)
	_, _ = io.ReadFull(reader, body)
	var clResp map[string]any
	_ = json.Unmarshal(body, &clResp)
	check("Content-Length response ID", clResp["id"] == float64(3), fmt.Sprintf("id=%v", clResp["id"]))

	stdin.Close()
	_ = cmd.Wait()

	fmt.Printf("\n==============================\n")
	fmt.Printf("NDJSON Smoke Results: %d PASS, %d FAIL of %d\n", pass, fail, pass+fail)
	if fail > 0 {
		fmt.Println("SOME TESTS FAILED!")
		os.Exit(1)
	}
	fmt.Println("ALL NDJSON SMOKE TESTS PASSED!")
}
