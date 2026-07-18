package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// btPhaseProvider serves a narrow local HTTP endpoint for refresh_phase.
// It is intentionally specific to project_next_steps / phase data.
type btPhaseProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTPhaseProvider(s *Server) (*btPhaseProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-phase-%d", time.Now().UnixNano())
	provider := &btPhaseProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/phase",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/phase", provider.handlePhase)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (p *btPhaseProvider) handlePhase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var input struct {
		NamespaceID string `json:"namespace_id"`
		DAGID       string `json:"dag_id,omitempty"`
		Workdir     string `json:"workdir,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := p.server.buildPhaseProviderResult(r.Context(), input.NamespaceID, input.DAGID)
	if err != nil {
		if _, ok := err.(*MultiDAGFocusError); ok {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(result.toMap())
}

func (p *btPhaseProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
