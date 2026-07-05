package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/toustifer/agentflow/pkg/engine"
)

type implementCodeRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
	WorkerID    string `json:"worker_id,omitempty"`
}

type implementCodeResponse struct {
	Task             map[string]any `json:"task"`
	DAG              map[string]any `json:"dag,omitempty"`
	Git              map[string]any `json:"git,omitempty"`
	Worker           map[string]any `json:"worker,omitempty"`
	Prompt           string         `json:"prompt,omitempty"`
	SuggestedActions []string       `json:"suggested_actions,omitempty"`
}

type btImplementCodeProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTImplementCodeProvider(s *Server) (*btImplementCodeProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-implement-code-%d", time.Now().UnixNano())
	provider := &btImplementCodeProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/implement-code",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/implement-code", provider.handleImplementCode)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) implementCodeOnce(ctx context.Context, namespaceID, taskID, workerID string) (implementCodeResponse, error) {
	ns, err := s.engine.GetNamespace(ctx, namespaceID)
	if err != nil {
		return implementCodeResponse{}, err
	}
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return implementCodeResponse{}, err
	}
	if workerID != "" && task.AssignedWorker != "" && task.AssignedWorker != workerID {
		return implementCodeResponse{}, fmt.Errorf("implement_code rejected: task %q is assigned to worker %q, not %q", taskID, task.AssignedWorker, workerID)
	}

	var dag *engine.DAG
	var dagMap map[string]any
	if task.DAGID != "" {
		dag, err = s.engine.GetDAG(ctx, namespaceID, task.DAGID)
		if err != nil {
			return implementCodeResponse{}, err
		}
		dagMap = dagToSummaryMap(dag)
	}

	runtime, err := s.getTaskGitRuntime(ctx, ns, dag, task)
	if err != nil {
		return implementCodeResponse{}, err
	}

	worker, err := s.engine.GetWorker(ctx, namespaceID, workerID)
	if err != nil {
		return implementCodeResponse{}, err
	}
	prompt, err := s.engine.WorkerPromptGet(ctx, namespaceID, workerID, taskID, task.Title, false)
	if err != nil {
		return implementCodeResponse{}, err
	}

	return implementCodeResponse{
		Task: taskToMap(task),
		DAG:  dagMap,
		Git: map[string]any{
			"repo_path":     runtime.RepoPath,
			"worktree_path": runtime.WorktreePath,
			"branch":        runtime.Branch,
			"base_branch":   runtime.BaseBranch,
			"head_sha":      runtime.HeadSHA,
			"status":        runtime.Status,
		},
		Worker: map[string]any{
			"id":    worker.ID,
			"name":  worker.Name,
			"scope": worker.Scope,
		},
		Prompt:           prompt,
		SuggestedActions: []string{"worker_prompt_get", "worktree_get", "git_status"},
	}, nil
}

func (p *btImplementCodeProvider) handleImplementCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input implementCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" || input.WorkerID == "" {
		http.Error(w, "namespace_id, task_id, and worker_id are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.implementCodeOnce(r.Context(), input.NamespaceID, input.TaskID, input.WorkerID)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound), errors.Is(err, engine.ErrNamespaceNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(err.Error(), "assigned to worker"), strings.Contains(err.Error(), "worktree 不存在"), strings.Contains(err.Error(), "worker has no prompt template configured"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btImplementCodeProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
