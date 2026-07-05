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

type btProviderCloser interface {
	close()
}

// BTBridge manages a Python BT service subprocess over JSON-RPC stdio.
type BTBridge struct {
	mu                    sync.Mutex
	cmd                   *exec.Cmd
	stdin                 io.WriteCloser
	stdout                *bufio.Reader
	started               bool
	nextID                int
	provider              *btPhaseProvider
	dispatchProvider      *btDispatchProvider
	monitorProvider       *btMonitorProvider
	stuckProvider         *btStuckProvider
	doneProvider          *btDoneProvider
	taskGetProvider       *btTaskGetProvider
	enterWorktreeProvider *btEnterWorktreeProvider
	implementProvider     *btImplementCodeProvider
	gitCommitProvider     *btGitCommitProvider
	docWriteProvider      *btDocWriteProvider
	diaryWriteProvider    *btDiaryWriteProvider
	submitReviewProvider  *btSubmitForReviewProvider
	fetchDiffProvider     *btFetchWorkDiffProvider
	reviewPassProvider    *btReviewPassProvider
	reviewReworkProvider  *btReviewReworkProvider
}

func NewBTBridge() *BTBridge {
	return &BTBridge{}
}

func closeBTProviders(providers ...btProviderCloser) {
	for _, provider := range providers {
		if provider != nil {
			provider.close()
		}
	}
}

func appendBTProvider[T btProviderCloser](cleanup []btProviderCloser, provider T) []btProviderCloser {
	return append(cleanup, provider)
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
	cleanupProviders := appendBTProvider(nil, provider)
	dispatchProvider, err := newBTDispatchProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start dispatch provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, dispatchProvider)
	monitorProvider, err := newBTMonitorProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start monitor provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, monitorProvider)
	stuckProvider, err := newBTStuckProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start stuck provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, stuckProvider)
	doneProvider, err := newBTDoneProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start done provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, doneProvider)
	taskGetProvider, err := newBTTaskGetProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start task_get provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, taskGetProvider)
	enterWorktreeProvider, err := newBTEnterWorktreeProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start enter_worktree provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, enterWorktreeProvider)
	implementProvider, err := newBTImplementCodeProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start implement_code provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, implementProvider)
	gitCommitProvider, err := newBTGitCommitProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start git_commit_changes provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, gitCommitProvider)
	docWriteProvider, err := newBTDocWriteProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start doc_write_record provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, docWriteProvider)
	diaryWriteProvider, err := newBTDiaryWriteProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start diary_write_entry provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, diaryWriteProvider)
	submitReviewProvider, err := newBTSubmitForReviewProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start task_submit_for_review provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, submitReviewProvider)
	fetchDiffProvider, err := newBTFetchWorkDiffProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start fetch_work_diff provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, fetchDiffProvider)
	reviewPassProvider, err := newBTReviewPassProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start task_review_pass provider: %w", err)
	}
	cleanupProviders = appendBTProvider(cleanupProviders, reviewPassProvider)
	reviewReworkProvider, err := newBTReviewReworkProvider(owner)
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start task_review_rework provider: %w", err)
	}

	pythonPath := findPython()
	serviceArgs := findBTScript()
	cmd := exec.Command(pythonPath, serviceArgs...)

	root := findBTDir()
	if root != "" {
		cmd.Dir = root
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
			"AGENTFLOW_BT_MONITOR_URL="+monitorProvider.url,
			"AGENTFLOW_BT_MONITOR_TOKEN="+monitorProvider.token,
			"AGENTFLOW_BT_STUCK_URL="+stuckProvider.url,
			"AGENTFLOW_BT_STUCK_TOKEN="+stuckProvider.token,
			"AGENTFLOW_BT_DONE_URL="+doneProvider.url,
			"AGENTFLOW_BT_DONE_TOKEN="+doneProvider.token,
			"AGENTFLOW_BT_TASK_GET_URL="+taskGetProvider.url,
			"AGENTFLOW_BT_TASK_GET_TOKEN="+taskGetProvider.token,
			"AGENTFLOW_BT_ENTER_WORKTREE_URL="+enterWorktreeProvider.url,
			"AGENTFLOW_BT_ENTER_WORKTREE_TOKEN="+enterWorktreeProvider.token,
			"AGENTFLOW_BT_IMPLEMENT_CODE_URL="+implementProvider.url,
			"AGENTFLOW_BT_IMPLEMENT_CODE_TOKEN="+implementProvider.token,
			"AGENTFLOW_BT_GIT_COMMIT_URL="+gitCommitProvider.url,
			"AGENTFLOW_BT_GIT_COMMIT_TOKEN="+gitCommitProvider.token,
			"AGENTFLOW_BT_DOC_WRITE_URL="+docWriteProvider.url,
			"AGENTFLOW_BT_DOC_WRITE_TOKEN="+docWriteProvider.token,
			"AGENTFLOW_BT_DIARY_WRITE_URL="+diaryWriteProvider.url,
			"AGENTFLOW_BT_DIARY_WRITE_TOKEN="+diaryWriteProvider.token,
			"AGENTFLOW_BT_SUBMIT_REVIEW_URL="+submitReviewProvider.url,
			"AGENTFLOW_BT_SUBMIT_REVIEW_TOKEN="+submitReviewProvider.token,
			"AGENTFLOW_BT_FETCH_DIFF_URL="+fetchDiffProvider.url,
			"AGENTFLOW_BT_FETCH_DIFF_TOKEN="+fetchDiffProvider.token,
			"AGENTFLOW_BT_REVIEW_PASS_URL="+reviewPassProvider.url,
			"AGENTFLOW_BT_REVIEW_PASS_TOKEN="+reviewPassProvider.token,
			"AGENTFLOW_BT_REVIEW_REWORK_URL="+reviewReworkProvider.url,
			"AGENTFLOW_BT_REVIEW_REWORK_TOKEN="+reviewReworkProvider.token,
		)
		cmd.Env = env
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("bt_service stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("bt_service stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("bt_service stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		closeBTProviders(cleanupProviders...)
		return fmt.Errorf("start bt_service: %w", err)
	}

	b.cmd = cmd
	b.stdin = stdin
	b.stdout = bufio.NewReader(stdout)
	b.started = true
	b.provider = provider
	b.dispatchProvider = dispatchProvider
	b.monitorProvider = monitorProvider
	b.stuckProvider = stuckProvider
	b.doneProvider = doneProvider
	b.taskGetProvider = taskGetProvider
	b.enterWorktreeProvider = enterWorktreeProvider
	b.implementProvider = implementProvider
	b.gitCommitProvider = gitCommitProvider
	b.docWriteProvider = docWriteProvider
	b.diaryWriteProvider = diaryWriteProvider
	b.submitReviewProvider = submitReviewProvider
	b.fetchDiffProvider = fetchDiffProvider
	b.reviewPassProvider = reviewPassProvider
	b.reviewReworkProvider = reviewReworkProvider

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
		closeBTProviders(
			b.provider,
			b.dispatchProvider,
			b.monitorProvider,
			b.stuckProvider,
			b.doneProvider,
			b.taskGetProvider,
			b.enterWorktreeProvider,
			b.implementProvider,
			b.gitCommitProvider,
			b.docWriteProvider,
			b.diaryWriteProvider,
			b.submitReviewProvider,
			b.fetchDiffProvider,
			b.reviewPassProvider,
			b.reviewReworkProvider,
		)
		b.provider = nil
		b.dispatchProvider = nil
		b.monitorProvider = nil
		b.stuckProvider = nil
		b.doneProvider = nil
		b.taskGetProvider = nil
		b.enterWorktreeProvider = nil
		b.implementProvider = nil
		b.gitCommitProvider = nil
		b.docWriteProvider = nil
		b.diaryWriteProvider = nil
		b.submitReviewProvider = nil
		b.fetchDiffProvider = nil
		b.reviewPassProvider = nil
		b.reviewReworkProvider = nil
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
