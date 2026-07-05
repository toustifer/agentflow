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

type enterWorktreeRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
	WorkerID    string `json:"worker_id,omitempty"`
}

type enterWorktreeResponse struct {
	TaskID         string         `json:"task_id"`
	State          string         `json:"state"`
	AssignedWorker string         `json:"assigned_worker,omitempty"`
	DAG            map[string]any `json:"dag,omitempty"`
	Git            map[string]any `json:"git,omitempty"`
}

type btEnterWorktreeProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTEnterWorktreeProvider(s *Server) (*btEnterWorktreeProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-enter-worktree-%d", time.Now().UnixNano())
	provider := &btEnterWorktreeProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/enter-worktree",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/enter-worktree", provider.handleEnterWorktree)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) enterWorktreeOnce(ctx context.Context, namespaceID, taskID, workerID string) (enterWorktreeResponse, error) {
	ns, err := s.engine.GetNamespace(ctx, namespaceID)
	if err != nil {
		return enterWorktreeResponse{}, err
	}
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return enterWorktreeResponse{}, err
	}
	if workerID != "" && task.AssignedWorker != "" && task.AssignedWorker != workerID {
		return enterWorktreeResponse{}, fmt.Errorf("enter_worktree rejected: task %q is assigned to worker %q, not %q", taskID, task.AssignedWorker, workerID)
	}

	switch task.State {
	case engine.TaskAssigned, engine.TaskExecuting, engine.TaskReworkNeeded:
	default:
		return enterWorktreeResponse{}, fmt.Errorf("enter_worktree rejected: task %q is in state %q", taskID, task.State)
	}

	var dag *engine.DAG
	var dagMap map[string]any
	if task.DAGID != "" {
		dag, err = s.engine.GetDAG(ctx, namespaceID, task.DAGID)
		if err != nil {
			return enterWorktreeResponse{}, err
		}
		dagMap = dagToSummaryMap(dag)
	}

	runtime, err := s.prepareTaskGitRuntime(ctx, ns, dag, task)
	if err != nil {
		return enterWorktreeResponse{}, err
	}

	metadata := map[string]string{}
	for k, v := range task.Metadata {
		metadata[k] = v
	}
	metadata["git.branch"] = runtime.Branch
	metadata["git.base_branch"] = runtime.BaseBranch
	metadata["git.repo_path"] = runtime.RepoPath
	metadata["git.worktree_path"] = runtime.WorktreePath
	metadata["git.head_sha"] = runtime.HeadSHA
	metadata["git.status"] = runtime.Status
	_, err = s.engine.UpdateTask(ctx, namespaceID, taskID, engine.UpdateTaskRequest{ReviewMetadata: metadata})
	if err != nil {
		return enterWorktreeResponse{}, err
	}

	return enterWorktreeResponse{
		TaskID:         task.ID,
		State:          string(task.State),
		AssignedWorker: task.AssignedWorker,
		DAG:            dagMap,
		Git: map[string]any{
			"repo_path":     runtime.RepoPath,
			"worktree_path": runtime.WorktreePath,
			"branch":        runtime.Branch,
			"base_branch":   runtime.BaseBranch,
			"head_sha":      runtime.HeadSHA,
			"status":        runtime.Status,
		},
	}, nil
}

func (p *btEnterWorktreeProvider) handleEnterWorktree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input enterWorktreeRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" {
		http.Error(w, "namespace_id and task_id are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.enterWorktreeOnce(r.Context(), input.NamespaceID, input.TaskID, input.WorkerID)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound), errors.Is(err, engine.ErrNamespaceNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(err.Error(), "state"), strings.Contains(err.Error(), "assigned to worker"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btEnterWorktreeProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
