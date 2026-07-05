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

type diaryWriteEntryRequest struct {
	NamespaceID string   `json:"namespace_id"`
	WorkerID    string   `json:"worker_id"`
	TaskID      string   `json:"task_id,omitempty"`
	Date        string   `json:"date,omitempty"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags,omitempty"`
}

type diaryWriteEntryResponse struct {
	WorkerID string   `json:"worker_id"`
	TaskID   string   `json:"task_id,omitempty"`
	Date     string   `json:"date"`
	Tags     []string `json:"tags,omitempty"`
}

type btDiaryWriteProvider struct {
	server *Server
	http   *http.Server
	ln     net.Listener
	token  string
	url    string
}

func newBTDiaryWriteProvider(s *Server) (*btDiaryWriteProvider, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	token := fmt.Sprintf("bt-diary-write-%d", time.Now().UnixNano())
	provider := &btDiaryWriteProvider{
		server: s,
		ln:     ln,
		token:  token,
		url:    "http://" + ln.Addr().String() + "/diary-write-entry",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/diary-write-entry", provider.handleDiaryWriteEntry)
	provider.http = &http.Server{Handler: mux}
	go provider.http.Serve(ln)
	return provider, nil
}

func (s *Server) diaryWriteEntryOnce(ctx context.Context, namespaceID, workerID, taskID, date, content string, tags []string) (diaryWriteEntryResponse, error) {
	if workerID == "" {
		return diaryWriteEntryResponse{}, fmt.Errorf("diary_write_entry rejected: worker_id is required")
	}
	if strings.TrimSpace(content) == "" {
		return diaryWriteEntryResponse{}, fmt.Errorf("diary_write_entry rejected: content is required")
	}
	if _, err := s.engine.GetWorker(ctx, namespaceID, workerID); err != nil {
		return diaryWriteEntryResponse{}, err
	}

	if taskID != "" {
		task, err := s.engine.GetTask(ctx, namespaceID, taskID)
		if err != nil {
			return diaryWriteEntryResponse{}, err
		}
		if task.AssignedWorker != "" && task.AssignedWorker != workerID {
			return diaryWriteEntryResponse{}, fmt.Errorf("diary_write_entry rejected: task %q is assigned to worker %q, not %q", taskID, task.AssignedWorker, workerID)
		}
	}

	if strings.TrimSpace(date) == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	entry, err := s.engine.WriteWorkerDiary(ctx, engine.WriteDiaryRequest{
		NamespaceID: namespaceID,
		WorkerID:    workerID,
		TaskID:      taskID,
		Date:        date,
		Content:     content,
		Tags:        tags,
	})
	if err != nil {
		return diaryWriteEntryResponse{}, err
	}

	return diaryWriteEntryResponse{
		WorkerID: entry.WorkerID,
		TaskID:   entry.TaskID,
		Date:     entry.Date,
		Tags:     entry.Tags,
	}, nil
}

func (p *btDiaryWriteProvider) handleDiaryWriteEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Agentflow-BT-Token") != p.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var input diaryWriteEntryRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.NamespaceID == "" || input.WorkerID == "" || strings.TrimSpace(input.Content) == "" {
		http.Error(w, "namespace_id, worker_id, and content are required", http.StatusBadRequest)
		return
	}

	result, err := p.server.diaryWriteEntryOnce(
		r.Context(),
		input.NamespaceID,
		input.WorkerID,
		input.TaskID,
		input.Date,
		input.Content,
		input.Tags,
	)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrTaskNotFound), errors.Is(err, engine.ErrNamespaceNotFound), strings.Contains(err.Error(), "worker not found"):
			http.Error(w, err.Error(), http.StatusNotFound)
		case strings.Contains(err.Error(), "assigned to worker"), strings.Contains(err.Error(), "content is required"):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (p *btDiaryWriteProvider) close() {
	if p == nil || p.http == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = p.http.Shutdown(ctx)
}
