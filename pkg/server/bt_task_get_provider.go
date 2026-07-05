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

type taskGetConfirmRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
	WorkerID    string `json:"worker_id,omitempty"`
}

type taskGetConfirmResponse struct {
	Task             map[string]any `json:"task"`
	TaskID           string         `json:"task_id"`
	Title            string         `json:"title,omitempty"`
	State            string         `json:"state"`
	AssignedWorker   string         `json:"assigned_worker,omitempty"`
	DAG              map[string]any `json:"dag,omitempty"`
	Git              map[string]any `json:"git,omitempty"`
	SuggestedActions []string       `json:"suggested_actions,omitempty"`
}

type btTaskGetProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTTaskGetProvider(s *Server) (*btTaskGetProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-task-get-%d", time.Now().UnixNano())
	provider := &btTaskGetProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/task-get-confirm",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/task-get-confirm", provider.handleTaskGetConfirm)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) taskGetConfirmOnce(ctx context.Context, namespaceID, taskID, workerID string) (taskGetConfirmResponse, error) {
	ns, err := s.engine.GetNamespace(ctx, namespaceID)
	if err != nil {
		return taskGetConfirmResponse{}, err
	}
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return taskGetConfirmResponse{}, err
	}
	if workerID != "" && task.AssignedWorker != "" && task.AssignedWorker != workerID {
		return taskGetConfirmResponse{}, fmt.Errorf("task_get_confirm rejected: task %q is assigned to worker %q, not %q", taskID, task.AssignedWorker, workerID)
	}

	switch task.State {
	case engine.TaskAssigned, engine.TaskExecuting, engine.TaskReworkNeeded:
	default:
		return taskGetConfirmResponse{}, fmt.Errorf("task_get_confirm rejected: task %q is in state %q", taskID, task.State)
	}

	var dag *engine.DAG
	var dagMap map[string]any
	if task.DAGID != "" {
		dag, err = s.engine.GetDAG(ctx, namespaceID, task.DAGID)
		if err != nil {
			return taskGetConfirmResponse{}, err
		}
		dagMap = dagToSummaryMap(dag)
	}

	runtime, err := s.resolveTaskGitRuntime(ctx, ns, dag, task)
	if err != nil {
		return taskGetConfirmResponse{}, err
	}

	return taskGetConfirmResponse{
		Task:           taskToMap(task),
		TaskID:         task.ID,
		Title:          task.Title,
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
		SuggestedActions: []string{"worktree_get", "worker_prompt_get"},
	}, nil
}

func (p *btTaskGetProvider) handleTaskGetConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input taskGetConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" {
		http.Error(w, "namespace_id and task_id are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.taskGetConfirmOnce(r.Context(), input.NamespaceID, input.TaskID, input.WorkerID)
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

func (p *btTaskGetProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
