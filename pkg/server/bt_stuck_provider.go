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

type reportStuckRequest struct {
	NamespaceID string `json:"namespace_id"`
	TaskID      string `json:"task_id"`
}

type reportStuckResponse struct {
	TaskID                 string           `json:"task_id"`
	Title                  string           `json:"title,omitempty"`
	State                  string           `json:"state"`
	AssignedWorker         string           `json:"assigned_worker,omitempty"`
	DAGID                  string           `json:"dag_id,omitempty"`
	AvailableTransitions   []string         `json:"available_transitions,omitempty"`
	Blockers               []map[string]any `json:"blockers,omitempty"`
	BlockerSummary         map[string]any   `json:"blocker_summary,omitempty"`
	SuggestedActions       []string         `json:"suggested_actions,omitempty"`
	RecoveryPolicy         []string         `json:"recovery_policy,omitempty"`
	FallbackMCP            []string         `json:"fallback_mcp,omitempty"`
	StuckPlaybook          string           `json:"stuck_playbook,omitempty"`
	EscalationMode         string           `json:"escalation_mode,omitempty"`
	OwnershipExpected      bool             `json:"ownership_expected"`
	ReassignmentExceptional bool            `json:"reassignment_exceptional"`
}

type btStuckProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTStuckProvider(s *Server) (*btStuckProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-stuck-%d", time.Now().UnixNano())
	provider := &btStuckProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/report-stuck",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/report-stuck", provider.handleReportStuck)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) reportStuckOnce(ctx context.Context, namespaceID, taskID string) (reportStuckResponse, error) {
	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return reportStuckResponse{}, err
	}

	switch task.State {
	case engine.TaskAssigned, engine.TaskReworkNeeded:
		// valid stuck anchor states
	default:
		return reportStuckResponse{}, fmt.Errorf("report_stuck rejected: task %q is in state %q", taskID, task.State)
	}

	blockers, err := s.engine.ProjectBlockers(ctx, namespaceID)
	if err != nil {
		return reportStuckResponse{}, err
	}

	items := make([]map[string]any, 0, len(blockers))
	dependencyCount := 0
	workerCount := 0
	for _, blocker := range blockers {
		items = append(items, map[string]any{
			"task_id":    blocker.TaskID,
			"title":      blocker.Title,
			"dag_id":     blocker.DAGID,
			"type":       blocker.Type,
			"blocked_by": blocker.BlockedBy,
		})
		switch blocker.Type {
		case "dependency":
			dependencyCount++
		case "worker":
			workerCount++
		}
	}

	suggestedActions := []string{"task_get", "worker_prompt_get", "worker_handbook_get", "find_knowledge", "find_pitfalls"}
	if len(items) > 0 {
		suggestedActions = append(suggestedActions, "project_blockers")
	}

	worker, workerErr := s.engine.GetWorker(ctx, namespaceID, task.AssignedWorker)
	if workerErr != nil {
		worker = nil
	}

	return reportStuckResponse{
		TaskID:               task.ID,
		Title:                task.Title,
		State:                string(task.State),
		AssignedWorker:       task.AssignedWorker,
		DAGID:                task.DAGID,
		AvailableTransitions: availableTransitionsToStrings(engine.AvailableTransitions(task)),
		Blockers:             items,
		BlockerSummary: map[string]any{
			"total":      len(items),
			"dependency": dependencyCount,
			"worker":     workerCount,
		},
		SuggestedActions:        suggestedActions,
		RecoveryPolicy:          workerRecoveryPolicy(worker),
		FallbackMCP:             workerFallbackMCP(worker),
		StuckPlaybook:           workerStuckPlaybook(worker),
		EscalationMode:          workerEscalationMode(worker),
		OwnershipExpected:       task.AssignedWorker != "",
		ReassignmentExceptional: true,
	}, nil
}

func workerRecoveryPolicy(w *engine.Worker) []string {
	if w == nil {
		return nil
	}
	return cloneStringSlice(w.RecoveryPolicy)
}

func workerFallbackMCP(w *engine.Worker) []string {
	if w == nil {
		return nil
	}
	return cloneStringSlice(w.FallbackMCP)
}

func workerStuckPlaybook(w *engine.Worker) string {
	if w == nil {
		return ""
	}
	return w.StuckPlaybook
}

func workerEscalationMode(w *engine.Worker) string {
	if w == nil {
		return ""
	}
	return w.EscalationMode
}

func (p *btStuckProvider) handleReportStuck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input reportStuckRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" {
		http.Error(w, "namespace_id and task_id are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.reportStuckOnce(r.Context(), input.NamespaceID, input.TaskID)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound):
			http.Error(w, err.Error(), http.StatusNotFound)
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

func (p *btStuckProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
