package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

type reportDoneRequest struct {
	NamespaceID string `json:"namespace_id"`
}

type reportDoneResponse struct {
	Phase            string         `json:"phase"`
	PhaseName        string         `json:"phase_name,omitempty"`
	Progress         string         `json:"progress,omitempty"`
	CompletedTasks   int            `json:"completed_tasks"`
	TotalTasks       int            `json:"total_tasks"`
	CompletionPct    int            `json:"completion_pct"`
	DAG              map[string]any `json:"dag,omitempty"`
	SuggestedActions []string       `json:"suggested_actions,omitempty"`
	NextSteps        []string       `json:"next_steps,omitempty"`
}

type btDoneProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTDoneProvider(s *Server) (*btDoneProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-done-%d", time.Now().UnixNano())
	provider := &btDoneProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/report-done",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/report-done", provider.handleReportDone)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) reportDoneOnce(ctx context.Context, namespaceID string) (reportDoneResponse, error) {
	result, err := s.handleProjectNextSteps(ctx, map[string]any{"namespace_id": namespaceID})
	if err != nil {
		return reportDoneResponse{}, err
	}
	phase := toString(result["phase"])
	if phase != "done" {
		return reportDoneResponse{}, fmt.Errorf("report_done rejected: namespace %q is in phase %q", namespaceID, phase)
	}

	completedTasks := toInt(result["completed_tasks"])
	totalTasks := toInt(result["total_tasks"])
	completionPct := 0
	if totalTasks > 0 {
		completionPct = completedTasks * 100 / totalTasks
	}

	return reportDoneResponse{
		Phase:            phase,
		PhaseName:        toString(result["phase_name"]),
		Progress:         toString(result["progress"]),
		CompletedTasks:   completedTasks,
		TotalTasks:       totalTasks,
		CompletionPct:    completionPct,
		DAG:              toMap(result["dag"]),
		SuggestedActions: toStringSlice(result["actions"]),
		NextSteps:        toStringSlice(result["next_steps"]),
	}, nil
}

func (p *btDoneProvider) handleReportDone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input reportDoneRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" {
		http.Error(w, "namespace_id is required", http.StatusBadRequest)
		return
	}

	result, err := p.server.reportDoneOnce(r.Context(), input.NamespaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btDoneProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}

func toInt(v any) int {
	switch value := v.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func toMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}
