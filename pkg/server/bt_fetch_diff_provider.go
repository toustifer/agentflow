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

type fetchWorkDiffRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
	WorkerID    string `json:"worker_id,omitempty"`
}

type fetchWorkDiffResponse struct {
	TaskID         string         `json:"task_id"`
	State          string         `json:"state"`
	AssignedWorker string         `json:"assigned_worker,omitempty"`
	Review         map[string]any `json:"review,omitempty"`
	Prompt         string         `json:"prompt,omitempty"`
}

type btFetchWorkDiffProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTFetchWorkDiffProvider(s *Server) (*btFetchWorkDiffProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-fetch-diff-%d", time.Now().UnixNano())
	provider := &btFetchWorkDiffProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/fetch-work-diff",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/fetch-work-diff", provider.handleFetchWorkDiff)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) fetchWorkDiffOnce(ctx context.Context, namespaceID, taskID, workerID string) (fetchWorkDiffResponse, error) {
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return fetchWorkDiffResponse{}, err
	}
	if task.State != engine.TaskReviewPending {
		return fetchWorkDiffResponse{}, fmt.Errorf("fetch_work_diff rejected: task %q is in state %q", taskID, task.State)
	}
	commit := ""
	diff := ""
	if task.Metadata != nil {
		commit = task.Metadata["review.commit"]
		diff = task.Metadata["review.diff"]
	}
	if commit == "" {
		return fetchWorkDiffResponse{}, fmt.Errorf("fetch_work_diff rejected: task %q has no review.commit metadata", taskID)
	}

	prompt, err := s.engine.WorkerPromptGet(ctx, namespaceID, workerID, taskID, task.Title, true)
	if err != nil {
		return fetchWorkDiffResponse{}, err
	}

	return fetchWorkDiffResponse{
		TaskID:         task.ID,
		State:          string(task.State),
		AssignedWorker: task.AssignedWorker,
		Review: map[string]any{
			"commit": commit,
			"diff":   diff,
		},
		Prompt: prompt,
	}, nil
}

func (p *btFetchWorkDiffProvider) handleFetchWorkDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input fetchWorkDiffRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" || input.WorkerID == "" {
		http.Error(w, "namespace_id, task_id, and worker_id are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.fetchWorkDiffOnce(r.Context(), input.NamespaceID, input.TaskID, input.WorkerID)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound), errors.Is(err, engine.ErrNamespaceNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(err.Error(), "state"), strings.Contains(err.Error(), "review.commit"), strings.Contains(err.Error(), "assigned to worker"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btFetchWorkDiffProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
