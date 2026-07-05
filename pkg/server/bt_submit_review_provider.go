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

type taskSubmitForReviewRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
	WorkerID    string `json:"worker_id,omitempty"`
}

type taskSubmitForReviewResponse struct {
	TaskID         string         `json:"task_id"`
	State          string         `json:"state"`
	AssignedWorker string         `json:"assigned_worker,omitempty"`
	Review         map[string]any `json:"review,omitempty"`
}

type btSubmitForReviewProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTSubmitForReviewProvider(s *Server) (*btSubmitForReviewProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-submit-review-%d", time.Now().UnixNano())
	provider := &btSubmitForReviewProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/task-submit-for-review",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/task-submit-for-review", provider.handleTaskSubmitForReview)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) taskSubmitForReviewOnce(ctx context.Context, namespaceID, taskID, workerID string) (taskSubmitForReviewResponse, error) {
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return taskSubmitForReviewResponse{}, err
	}
	if workerID != "" && task.AssignedWorker != "" && task.AssignedWorker != workerID {
		return taskSubmitForReviewResponse{}, fmt.Errorf("task_submit_for_review rejected: task %q is assigned to worker %q, not %q", taskID, task.AssignedWorker, workerID)
	}

	result, err := s.handleTaskTransition(ctx, map[string]any{
		"namespace_id": namespaceID,
		"task_id":      taskID,
		"transition":   "submit",
		"actor_role":   "worker",
	})
	if err != nil {
		return taskSubmitForReviewResponse{}, err
	}

	task = result.task
	return taskSubmitForReviewResponse{
		TaskID:         task.ID,
		State:          string(task.State),
		AssignedWorker: task.AssignedWorker,
		Review: map[string]any{
			"commit": task.Metadata["review.commit"],
			"diff":   task.Metadata["review.diff"],
		},
	}, nil
}

func (p *btSubmitForReviewProvider) handleTaskSubmitForReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input taskSubmitForReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" || input.WorkerID == "" {
		http.Error(w, "namespace_id, task_id, and worker_id are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.taskSubmitForReviewOnce(r.Context(), input.NamespaceID, input.TaskID, input.WorkerID)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound), errors.Is(err, engine.ErrNamespaceNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(err.Error(), "assigned to worker"), strings.Contains(err.Error(), "submit 被拒绝"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btSubmitForReviewProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
