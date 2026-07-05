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

type docWriteRecordRequest struct {
	NamespaceID string   `json:"namespace_id"`
	WorkerID    string   `json:"worker_id,omitempty"`
	TaskID      string   `json:"task_id"`
	Title       string   `json:"title,omitempty"`
	Content     string   `json:"content"`
	Path        string   `json:"path,omitempty"`
	Section     string   `json:"section,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	DocID       int64    `json:"doc_id,omitempty"`
}

type docWriteRecordResponse struct {
	DocID   int64    `json:"doc_id"`
	Title   string   `json:"title,omitempty"`
	Path    string   `json:"path,omitempty"`
	Section string   `json:"section,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

type btDocWriteProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTDocWriteProvider(s *Server) (*btDocWriteProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-doc-write-%d", time.Now().UnixNano())
	provider := &btDocWriteProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/doc-write-record",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/doc-write-record", provider.handleDocWriteRecord)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) docWriteRecordOnce(ctx context.Context, namespaceID, taskID, workerID, title, content, path, section string, tags []string, docID int64) (docWriteRecordResponse, error) {
	if strings.TrimSpace(content) == "" {
		return docWriteRecordResponse{}, fmt.Errorf("doc_write_record rejected: content is required")
	}

	task, err := s.engine.GetTask(ctx, namespaceID, taskID)
	if err != nil {
		return docWriteRecordResponse{}, err
	}
	if workerID == "" {
		return docWriteRecordResponse{}, fmt.Errorf("doc_write_record rejected: worker_id is required")
	}
	if _, err := s.engine.GetWorker(ctx, namespaceID, workerID); err != nil {
		return docWriteRecordResponse{}, err
	}
	if task.AssignedWorker != "" && task.AssignedWorker != workerID {
		return docWriteRecordResponse{}, fmt.Errorf("doc_write_record rejected: task %q is assigned to worker %q, not %q", taskID, task.AssignedWorker, workerID)
	}

	switch task.State {
	case engine.TaskAssigned, engine.TaskExecuting, engine.TaskReworkNeeded:
	default:
		return docWriteRecordResponse{}, fmt.Errorf("doc_write_record rejected: task %q is in state %q", taskID, task.State)
	}

	if strings.TrimSpace(title) == "" {
		title = task.Title
	}
	if strings.TrimSpace(section) == "" {
		section = "tasks"
	}
	if strings.TrimSpace(path) == "" {
		path = fmt.Sprintf("tasks/%s.md", taskID)
	}

	doc, err := s.engine.WriteProjectDoc(ctx, namespaceID, engine.ProjectDoc{
		ID:      docID,
		Section: section,
		Path:    path,
		Title:   title,
		Content: content,
		Tags:    tags,
	})
	if err != nil {
		return docWriteRecordResponse{}, err
	}

	return docWriteRecordResponse{
		DocID:   doc.ID,
		Title:   doc.Title,
		Path:    doc.Path,
		Section: doc.Section,
		Tags:    doc.Tags,
	}, nil
}

func (p *btDocWriteProvider) handleDocWriteRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input docWriteRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.TaskID == "" || input.WorkerID == "" || strings.TrimSpace(input.Content) == "" {
		http.Error(w, "namespace_id, task_id, worker_id, and content are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.docWriteRecordOnce(
		r.Context(),
		input.NamespaceID,
		input.TaskID,
		input.WorkerID,
		input.Title,
		input.Content,
		input.Path,
		input.Section,
		input.Tags,
		input.DocID,
	)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound), errors.Is(err, engine.ErrNamespaceNotFound), strings.Contains(err.Error(), "worker not found"):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(err.Error(), "assigned to worker"), strings.Contains(err.Error(), "state"), strings.Contains(err.Error(), "content is required"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btDocWriteProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
