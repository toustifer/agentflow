package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	btBridgeStartupTimeout = 5 * time.Second
	btBridgeStopTimeout    = 2 * time.Second
	btBridgeMaxFrameBytes  = 4 * 1024 * 1024
)

// BTBridge manages a Python BT service subprocess over JSON-RPC stdio.
type BTBridge struct {
	mu               sync.Mutex
	cmd              *exec.Cmd
	stdin            io.WriteCloser
	stdout           *bufio.Reader
	started          bool
	nextID           int
	provider         *btPhaseProvider
	dispatchProvider *btDispatchProvider
}

func NewBTBridge() *BTBridge {
	return &BTBridge{}
}

func (b *BTBridge) Start(owner *Server) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return nil
	}

	provider, err := newBTPhaseProvider(owner)
	if err != nil {
		return fmt.Errorf("start phase provider: %w", err)
	}
	dispatchProvider, err := newBTDispatchProvider(owner)
	if err != nil {
		provider.close()
		return fmt.Errorf("start dispatch provider: %w", err)
	}

	pythonPath := findPython()
	serviceArgs := findBTScript()
	cmd := exec.Command(pythonPath, serviceArgs...)

	if root := findBTDir(); root != "" {
		cmd.Dir = root
	}

	if root := findBTDir(); root != "" {
		env := os.Environ()
		sep := string(os.PathListSeparator)
		pyPathVal := root
		for _, kv := range env {
			if len(kv) > 11 && kv[:11] == "PYTHONPATH=" {
				if existing := kv[11:]; existing != "" {
					pyPathVal = root + sep + existing
				}
				break
			}
		}
		env = append(env,
			"PYTHONPATH="+pyPathVal,
			"AGENTFLOW_BT_PHASE_URL="+provider.url,
			"AGENTFLOW_BT_PHASE_TOKEN="+provider.token,
			"AGENTFLOW_BT_DISPATCH_URL="+dispatchProvider.url,
			"AGENTFLOW_BT_DISPATCH_TOKEN="+dispatchProvider.token,
		)
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		provider.close()
		dispatchProvider.close()
		return fmt.Errorf("bt_service stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		provider.close()
		dispatchProvider.close()
		return fmt.Errorf("bt_service stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		provider.close()
		dispatchProvider.close()
		return fmt.Errorf("bt_service stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		provider.close()
		dispatchProvider.close()
		return fmt.Errorf("start bt_service: %w", err)
	}

	b.cmd = cmd
	b.stdin = stdin
	b.stdout = bufio.NewReader(stdout)
	b.started = true
	b.provider = provider
	b.dispatchProvider = dispatchProvider

	var stderrBuf bytes.Buffer
	go io.Copy(io.MultiWriter(os.Stderr, &stderrBuf), stderr)

	errCh := make(chan error, 1)
	go func() {
		_, err := b.rpcLocked("ping", map[string]any{
			"client":  "agentflow-go",
			"version": "dev",
		})
		errCh <- err
	}()

	select {
	case err := <-errCh:
		if err != nil {
			b.stopLocked()
			return fmt.Errorf("bt_service ping failed: %w", err)
		}
	case <-time.After(btBridgeStartupTimeout):
		b.stopLocked()
		return fmt.Errorf("bt_service startup timeout after %s; stderr=%q", btBridgeStartupTimeout, stderrBuf.String())
	}

	return nil
}

func (b *BTBridge) RPC(method string, params map[string]any) (map[string]any, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.started || b.stdin == nil || b.stdout == nil {
		return nil, fmt.Errorf("bt_service not started")
	}
	return b.rpcLocked(method, params)
}

func (b *BTBridge) rpcLocked(method string, params map[string]any) (map[string]any, error) {
	b.nextID++
	id := b.nextID

	req := btRPCRequest{JSONRPC: btRPCVersion, ID: id, Method: method, Params: params}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(payload))

	if _, err := io.WriteString(b.stdin, header); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}
	if _, err := b.stdin.Write(payload); err != nil {
		return nil, fmt.Errorf("write body: %w", err)
	}

	resp, err := b.readResponseLocked()
	if err != nil {
		return nil, err
	}
	if resp.JSONRPC != btRPCVersion {
		return nil, fmt.Errorf("bt_service invalid jsonrpc version: %q", resp.JSONRPC)
	}
	if resp.ID != id {
		return nil, fmt.Errorf("bt_service response id mismatch: got %d want %d", resp.ID, id)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("bt_service: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

func (b *BTBridge) readResponseLocked() (*btRPCResponse, error) {
	contentLength := 0
	for {
		line, err := b.stdout.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}
		line = trimCRLF(line)
		if line == "" {
			break
		}
		if len(line) > 16 && line[:16] == "Content-Length: " {
			fmt.Sscanf(line[16:], "%d", &contentLength)
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("bt_service: empty response")
	}
	if contentLength > btBridgeMaxFrameBytes {
		return nil, fmt.Errorf("bt_service frame too large: %d", contentLength)
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(b.stdout, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var resp btRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bt_service bad JSON: %w", err)
	}
	return &resp, nil
}

func (b *BTBridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopLocked()
}

func (b *BTBridge) stopLocked() {
	if b.stdin != nil {
		_ = b.stdin.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		done := make(chan struct{})
		go func() {
			_ = b.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(btBridgeStopTimeout):
			_ = b.cmd.Process.Kill()
			<-done
		}
	}
	if b.provider != nil {
		b.provider.close()
		b.provider = nil
	}
	if b.dispatchProvider != nil {
		b.dispatchProvider.close()
		b.dispatchProvider = nil
	}
	b.cmd = nil
	b.stdin = nil
	b.stdout = nil
	b.started = false
}

func findBTDir() string {
	if d := os.Getenv("AGENTFLOW_BT_DIR"); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		candidates := []string{
			filepath.Join(filepath.Dir(exe), "bt_service"),
			filepath.Join(filepath.Dir(exe), "..", "bt_service"),
			filepath.Join(filepath.Dir(exe), ".."),
		}
		for _, d := range candidates {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				if filepath.Base(d) == "bt_service" {
					return filepath.Dir(d)
				}
				return d
			}
		}
	}
	if _, err := os.Stat("D:/myprogram/agentflow/bt_service"); err == nil {
		return "D:/myprogram/agentflow"
	}
	if _, err := os.Stat("bt_service"); err == nil {
		if cwd, err := os.Getwd(); err == nil {
			return cwd
		}
	}
	return ""
}

func findPython() string {
	if p := os.Getenv("AGENTFLOW_PYTHON"); p != "" {
		return p
	}
	for _, name := range []string{"python", "python3", "py"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return "python"
}

func findBTScript() []string {
	return []string{"-m", "bt_service"}
}

func trimCRLF(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	if len(s) > 0 && s[len(s)-1] == '\r' {
		s = s[:len(s)-1]
	}
	return s
}
