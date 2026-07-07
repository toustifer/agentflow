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

type dispatchTaskRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
}

type dispatchTaskResponse struct {
	TaskID         string         `json:"task_id"`
	State          string         `json:"state"`
	AssignedWorker string         `json:"assigned_worker,omitempty"`
	WorktreePath   string         `json:"worktree_path,omitempty"`
	Branch         string         `json:"branch,omitempty"`
	WorkerLaunch   map[string]any `json:"worker_launch,omitempty"`
}

type btDispatchProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTDispatchProvider(s *Server) (*btDispatchProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-dispatch-%d", time.Now().UnixNano())
	provider := &btDispatchProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/dispatch-task",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/dispatch-task", provider.handleDispatchTask)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) dispatchTaskOnce(ctx context.Context, namespaceID, taskID string) (dispatchTaskResponse, error) {
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return dispatchTaskResponse{}, err
	}

	switch task.State {
	case engine.TaskAssigned:
		ns, err := s.engine.GetNamespace(ctx, namespaceID)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		if task.DAGID == "" {
			return dispatchTaskResponse{}, fmt.Errorf("start 被拒绝：task %q 没有关联 DAG，无法绑定 git branch/worktree", taskID)
		}
		dag, err := s.engine.GetDAG(ctx, namespaceID, task.DAGID)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		runtime, err := s.prepareTaskGitRuntime(ctx, ns, dag, task)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		metadata := map[string]string{
			"actor_role":        "leader",
			"git.branch":        runtime.Branch,
			"git.base_branch":   runtime.BaseBranch,
			"git.repo_path":     runtime.RepoPath,
			"git.worktree_path": runtime.WorktreePath,
			"git.head_sha":      runtime.HeadSHA,
			"git.status":        runtime.Status,
		}
		task, err = s.engine.TransitionTask(ctx, namespaceID, taskID, engine.TransStart, metadata)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
	case engine.TaskExecuting:
		// idempotent success
	default:
		return dispatchTaskResponse{}, fmt.Errorf("dispatch_task rejected: task %q is in state %q", taskID, task.State)
	}

	resp := dispatchTaskResponse{
		TaskID:         task.ID,
		State:          string(task.State),
		AssignedWorker: task.AssignedWorker,
	}
	if task.Metadata != nil {
		resp.WorktreePath = task.Metadata["git.worktree_path"]
		resp.Branch = task.Metadata["git.branch"]
	}
	if task.State == engine.TaskExecuting {
		ns, err := s.engine.GetNamespace(ctx, namespaceID)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		briefing, err := s.buildWorkerLaunchBriefing(ctx, ns, task)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		resp.WorkerLaunch = briefing
	}
	return resp, nil
}

func (p *btDispatchProvider) handleDispatchTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input dispatchTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" {
		http.Error(w, "namespace_id and task_id are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.dispatchTaskOnce(r.Context(), input.NamespaceID, input.TaskID)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, engine.ErrInvalidTransition):
			http.Error(w, err.Error(), http.StatusConflict)
		case strings.Contains(err.Error(), "state"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btDispatchProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
