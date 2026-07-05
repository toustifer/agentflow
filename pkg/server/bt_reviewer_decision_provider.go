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

type reviewerDecisionRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
	WorkerID    string `json:"worker_id,omitempty"`
}

type reviewerDecisionResponse struct {
	TaskID string `json:"task_id"`
	State  string `json:"state"`
}

type btReviewPassProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

type btReviewReworkProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTReviewPassProvider(s *Server) (*btReviewPassProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-review-pass-%d", time.Now().UnixNano())
	provider := &btReviewPassProvider{server: s, ln: ln, token: token, url: "http://" + ln.Addr().String() + "/task-review-pass"}
	mux := http.NewServeMux()
	mux.HandleFunc("/task-review-pass", provider.handleTaskReviewPass)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func newBTReviewReworkProvider(s *Server) (*btReviewReworkProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-review-rework-%d", time.Now().UnixNano())
	provider := &btReviewReworkProvider{server: s, ln: ln, token: token, url: "http://" + ln.Addr().String() + "/task-review-rework"}
	mux := http.NewServeMux()
	mux.HandleFunc("/task-review-rework", provider.handleTaskReviewRework)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) taskReviewPassOnce(ctx context.Context, namespaceID, taskID, workerID string) (reviewerDecisionResponse, error) {
	if workerID != "" {
		if _, err := s.engine.GetWorker(ctx, namespaceID, workerID); err != nil {
			return reviewerDecisionResponse{}, err
		}
	}
	result, err := s.handleTaskTransition(ctx, map[string]any{
		"namespace_id": namespaceID,
		"task_id":      taskID,
		"transition":   "pass",
		"actor_role":   "reviewer",
		"metadata": map[string]any{
			"review.reviewer_id": workerID,
		},
	})
	if err != nil {
		return reviewerDecisionResponse{}, err
	}
	return reviewerDecisionResponse{TaskID: result.task.ID, State: string(result.task.State)}, nil
}

func (s *Server) taskReviewReworkOnce(ctx context.Context, namespaceID, taskID, workerID string) (reviewerDecisionResponse, error) {
	if workerID != "" {
		if _, err := s.engine.GetWorker(ctx, namespaceID, workerID); err != nil {
			return reviewerDecisionResponse{}, err
		}
	}
	result, err := s.handleTaskTransition(ctx, map[string]any{
		"namespace_id": namespaceID,
		"task_id":      taskID,
		"transition":   "rework",
		"actor_role":   "reviewer",
		"metadata": map[string]any{
			"review.reviewer_id": workerID,
		},
	})
	if err != nil {
		return reviewerDecisionResponse{}, err
	}
	return reviewerDecisionResponse{TaskID: result.task.ID, State: string(result.task.State)}, nil
}

func (p *btReviewPassProvider) handleTaskReviewPass(w http.ResponseWriter, r *http.Request) {
	handleReviewerDecision(w, r, p.token, func(ctx context.Context, nsID, taskID, workerID string) (reviewerDecisionResponse, error) {
		return p.server.taskReviewPassOnce(ctx, nsID, taskID, workerID)
	})
}

func (p *btReviewReworkProvider) handleTaskReviewRework(w http.ResponseWriter, r *http.Request) {
	handleReviewerDecision(w, r, p.token, func(ctx context.Context, nsID, taskID, workerID string) (reviewerDecisionResponse, error) {
		return p.server.taskReviewReworkOnce(ctx, nsID, taskID, workerID)
	})
}

func handleReviewerDecision(w http.ResponseWriter, r *http.Request, token string, fn func(context.Context, string, string, string) (reviewerDecisionResponse, error)) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var input reviewerDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" {
		http.Error(w, "namespace_id and task_id are required", http.StatusBadRequest)
		return
	}
	result, err := fn(r.Context(), input.NamespaceID, input.TaskID, input.WorkerID)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound), errors.Is(err, engine.ErrNamespaceNotFound), strings.Contains(err.Error(), "worker not found"):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(err.Error(), "transition") || strings.Contains(err.Error(), "state"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btReviewPassProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}

func (p *btReviewReworkProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
