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

// dispatchTaskOnce prepares a ready task for Skill-primary launch.
// Skill-primary contract:
//   prepare (this call) → leader spawns a real Agent → task_transition(start)
// This function must NOT TransitionTask(TransStart) and must NOT mint synthetic worker_agent_id.
func (s *Server) dispatchTaskOnce(ctx context.Context, namespaceID, taskID string) (dispatchTaskResponse, error) {
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return dispatchTaskResponse{}, err
	}

	switch task.State {
	case engine.TaskAssigned, engine.TaskReworkNeeded:
		// prepare-only: worktree + launch ticket; state stays assigned/rework_needed
		ns, _, dag, metadata, err := s.prepareTaskStart(ctx, namespaceID, taskID, false)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		now := time.Now().UTC()
		ticket := issueLaunchTicket(now)
		for k, v := range launchTicketMetadata(ticket, now) {
			metadata[k] = v
		}
		task, err = s.engine.UpdateTask(ctx, namespaceID, taskID, engine.UpdateTaskRequest{Metadata: metadata})
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		// Record worktree runtime only — no lease holder (lease is taken on real start).
		_, err = s.engine.UpdateDAGRuntime(ctx, namespaceID, dag.ID, engine.UpdateDAGRuntimeRequest{
			WorktreePath:     metadata["git.worktree_path"],
			WorktreeStatus:   metadata["git.status"],
			HeadSHA:          metadata["git.head_sha"],
			RuntimeUpdatedAt: now.Format(time.RFC3339),
		})
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		resp := dispatchTaskResponse{
			TaskID:         task.ID,
			State:          string(task.State),
			AssignedWorker: task.AssignedWorker,
			WorktreePath:   task.Metadata["git.worktree_path"],
			Branch:         task.Metadata["git.branch"],
		}
		briefing, err := s.buildWorkerLaunchBriefing(ctx, ns, task)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		briefing["launch_ticket"] = ticket
		resp.WorkerLaunch = briefing
		return resp, nil
	case engine.TaskExecuting:
		// Already started by skill path: idempotent re-brief only.
		resp := dispatchTaskResponse{
			TaskID:         task.ID,
			State:          string(task.State),
			AssignedWorker: task.AssignedWorker,
		}
		if task.Metadata != nil {
			resp.WorktreePath = task.Metadata["git.worktree_path"]
			resp.Branch = task.Metadata["git.branch"]
		}
		ns, err := s.engine.GetNamespace(ctx, namespaceID)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		briefing, err := s.buildWorkerLaunchBriefing(ctx, ns, task)
		if err != nil {
			return dispatchTaskResponse{}, err
		}
		resp.WorkerLaunch = briefing
		return resp, nil
	default:
		return dispatchTaskResponse{}, fmt.Errorf("dispatch_task rejected: task %q is in state %q", taskID, task.State)
	}
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
