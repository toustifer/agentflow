package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/toustifer/agentflow/pkg/engine"
)

type monitorTaskRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
}

type monitorTaskResponse struct {
	TaskID               string   `json:"task_id"`
	Title                string   `json:"title,omitempty"`
	State                string   `json:"state"`
	AssignedWorker       string   `json:"assigned_worker,omitempty"`
	ReviewCycle          int      `json:"review_cycle,omitempty"`
	AvailableTransitions []string `json:"available_transitions,omitempty"`
	WorktreePath         string   `json:"worktree_path,omitempty"`
	Branch               string   `json:"branch,omitempty"`
	ActiveTaskID         string   `json:"active_task_id,omitempty"`
	LeaseHolderTaskID    string   `json:"lease_holder_task_id,omitempty"`
	LeaseHolderWorkerID  string   `json:"lease_holder_worker_id,omitempty"`
	LeaseHolderAgentID   string   `json:"lease_holder_agent_id,omitempty"`
}

type btMonitorProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTMonitorProvider(s *Server) (*btMonitorProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-monitor-%d", time.Now().UnixNano())
	provider := &btMonitorProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/monitor-task",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/monitor-task", provider.handleMonitorTask)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) monitorTaskOnce(ctx context.Context, namespaceID, taskID string) (monitorTaskResponse, error) {
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return monitorTaskResponse{}, err
	}
	resp := monitorTaskResponse{
		TaskID:               task.ID,
		Title:                task.Title,
		State:                string(task.State),
		AssignedWorker:       task.AssignedWorker,
		ReviewCycle:          task.ReviewCycle,
		AvailableTransitions: availableTransitionsToStrings(engine.AvailableTransitions(task)),
	}
	var dag *engine.DAG
	if task.DAGID != "" {
		dag, _ = s.engine.GetDAG(ctx, namespaceID, task.DAGID)
	}
	if dag != nil {
		resp.WorktreePath = dag.WorktreePath
		resp.Branch = dag.ExecutionBranch
		resp.ActiveTaskID = dag.ActiveTaskID
		resp.LeaseHolderTaskID = dag.LeaseHolderTaskID
		resp.LeaseHolderWorkerID = dag.LeaseHolderWorkerID
		resp.LeaseHolderAgentID = dag.LeaseHolderAgentID
	} else if task.Metadata != nil {
		resp.WorktreePath = task.Metadata["git.worktree_path"]
		resp.Branch = task.Metadata["git.branch"]
	}
	return resp, nil
}

func (p *btMonitorProvider) handleMonitorTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input monitorTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" {
		http.Error(w, "namespace_id and task_id are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.monitorTaskOnce(r.Context(), input.NamespaceID, input.TaskID)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btMonitorProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}

func availableTransitionsToStrings(values []engine.AvailableTransition) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.Transition)
	}
	return out
}
