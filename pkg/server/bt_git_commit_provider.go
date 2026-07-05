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

type gitCommitChangesRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
	WorkerID    string `json:"worker_id,omitempty"`
}

type gitCommitChangesResponse struct {
	TaskID         string         `json:"task_id"`
	State          string         `json:"state"`
	AssignedWorker string         `json:"assigned_worker,omitempty"`
	Git            map[string]any `json:"git,omitempty"`
	Review         map[string]any `json:"review,omitempty"`
}

type btGitCommitProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTGitCommitProvider(s *Server) (*btGitCommitProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-git-commit-%d", time.Now().UnixNano())
	provider := &btGitCommitProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/git-commit-changes",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/git-commit-changes", provider.handleGitCommitChanges)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) gitCommitChangesOnce(ctx context.Context, namespaceID, taskID, workerID string) (gitCommitChangesResponse, error) {
	ns, err := s.engine.GetNamespace(ctx, namespaceID)
	if err != nil {
		return gitCommitChangesResponse{}, err
	}
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return gitCommitChangesResponse{}, err
	}
	if workerID != "" && task.AssignedWorker != "" && task.AssignedWorker != workerID {
		return gitCommitChangesResponse{}, fmt.Errorf("git_commit_changes rejected: task %q is assigned to worker %q, not %q", taskID, task.AssignedWorker, workerID)
	}
	if task.State != engine.TaskAssigned && task.State != engine.TaskExecuting && task.State != engine.TaskReworkNeeded {
		return gitCommitChangesResponse{}, fmt.Errorf("git_commit_changes rejected: task %q is in state %q", taskID, task.State)
	}

	var dag *engine.DAG
	if task.DAGID != "" {
		dag, err = s.engine.GetDAG(ctx, namespaceID, task.DAGID)
		if err != nil {
			return gitCommitChangesResponse{}, err
		}
	}
	runtime, err := s.getTaskGitRuntime(ctx, ns, dag, task)
	if err != nil {
		return gitCommitChangesResponse{}, err
	}
	if runtime.Status != "clean" {
		return gitCommitChangesResponse{}, fmt.Errorf("git_commit_changes rejected: task %q worktree status is %q; commit or clean the worktree before recording review metadata", taskID, runtime.Status)
	}

	head, err := runGit(ctx, runtime.WorktreePath, "rev-parse", "HEAD")
	if err != nil {
		return gitCommitChangesResponse{}, err
	}
	diff := ""
	if runtime.BaseBranch != "" && runtime.Branch != "" {
		if value, err := runGit(ctx, runtime.WorktreePath, "diff", runtime.BaseBranch+"..."+runtime.Branch); err == nil {
			diff = value
		}
	}

	metadata := map[string]string{}
	for k, v := range task.Metadata {
		metadata[k] = v
	}
	metadata["review.commit"] = head
	metadata["review.diff"] = diff
	_, err = s.engine.UpdateTask(ctx, namespaceID, taskID, engine.UpdateTaskRequest{ReviewMetadata: metadata})
	if err != nil {
		return gitCommitChangesResponse{}, err
	}

	return gitCommitChangesResponse{
		TaskID:         task.ID,
		State:          string(task.State),
		AssignedWorker: task.AssignedWorker,
		Git: map[string]any{
			"repo_path":     runtime.RepoPath,
			"worktree_path": runtime.WorktreePath,
			"branch":        runtime.Branch,
			"base_branch":   runtime.BaseBranch,
			"head_sha":      runtime.HeadSHA,
			"status":        runtime.Status,
		},
		Review: map[string]any{
			"commit": head,
			"diff":   diff,
		},
	}, nil
}

func (p *btGitCommitProvider) handleGitCommitChanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input gitCommitChangesRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" || input.WorkerID == "" {
		http.Error(w, "namespace_id, task_id, and worker_id are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.gitCommitChangesOnce(r.Context(), input.NamespaceID, input.TaskID, input.WorkerID)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound), errors.Is(err, engine.ErrNamespaceNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(err.Error(), "assigned to worker"), strings.Contains(err.Error(), "state"), strings.Contains(err.Error(), "worktree status"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btGitCommitProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
