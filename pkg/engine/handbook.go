package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// WorkerHandbook
// ---------------------------------------------------------------------------

type KnowledgeItem struct {
	Topic   string   `json:"topic"`
	Content string   `json:"content"`
	Tags    []string `json:"tags,omitempty"`
	Source  string   `json:"source,omitempty"`
}

type PitfallItem struct {
	Scenario string   `json:"scenario"`
	Problem  string   `json:"problem"`
	Solution string   `json:"solution"`
	Tags     []string `json:"tags,omitempty"`
	Source   string   `json:"source,omitempty"`
}

type WorkerHandbook struct {
	WorkerID    string          `json:"worker_id"`
	NamespaceID string          `json:"namespace_id"`
	Scope       string          `json:"scope,omitempty"`
	TechStack   []string        `json:"tech_stack,omitempty"`
	Knowledge   []KnowledgeItem `json:"knowledge,omitempty"`
	Pitfalls    []PitfallItem   `json:"pitfalls,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type WriteHandbookRequest struct {
	NamespaceID string
	WorkerID    string
	Scope       string
	TechStack   []string
	Knowledge   []KnowledgeItem
	Pitfalls    []PitfallItem
}

func (e *Engine) WriteHandbook(ctx context.Context, req WriteHandbookRequest) (*WorkerHandbook, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[req.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	if _, ok := e.workers[req.NamespaceID][req.WorkerID]; !ok {
		return nil, errors.New("worker not found")
	}

	now := time.Now().UTC()

	if existing, ok := e.handbooks[req.NamespaceID][req.WorkerID]; ok {
		// Merge: update scalar fields, append knowledge & pitfalls
		if req.Scope != "" {
			existing.Scope = req.Scope
		}
		if req.TechStack != nil {
			existing.TechStack = cloneStrings(req.TechStack)
		}
		if req.Knowledge != nil {
			existing.Knowledge = append(existing.Knowledge, req.Knowledge...)
		}
		if req.Pitfalls != nil {
			existing.Pitfalls = append(existing.Pitfalls, req.Pitfalls...)
		}
		existing.UpdatedAt = now

		if e.db != nil {
			if err := upsertHandbook(e.db, existing); err != nil {
				return nil, fmt.Errorf("persist handbook: %w", err)
			}
		}
		return cloneHandbook(existing), nil
	}

	// Create new
	hb := &WorkerHandbook{
		WorkerID:    req.WorkerID,
		NamespaceID: req.NamespaceID,
		Scope:       req.Scope,
		TechStack:   cloneStrings(req.TechStack),
		Knowledge:   copyKnowledge(req.Knowledge),
		Pitfalls:    copyPitfalls(req.Pitfalls),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if e.handbooks[req.NamespaceID] == nil {
		e.handbooks[req.NamespaceID] = make(map[string]*WorkerHandbook)
	}
	e.handbooks[req.NamespaceID][req.WorkerID] = hb

	if e.db != nil {
		if err := upsertHandbook(e.db, hb); err != nil {
			delete(e.handbooks[req.NamespaceID], req.WorkerID)
			return nil, fmt.Errorf("persist handbook: %w", err)
		}
	}
	return cloneHandbook(hb), nil
}

func (e *Engine) GetHandbook(ctx context.Context, nsID, workerID string) (*WorkerHandbook, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	hb, ok := e.handbooks[nsID][workerID]
	if !ok {
		return nil, errors.New("handbook not found")
	}
	return cloneHandbook(hb), nil
}

func (e *Engine) ListHandbooks(ctx context.Context, nsID string) ([]WorkerHandbook, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	out := make([]WorkerHandbook, 0, len(e.handbooks[nsID]))
	for _, hb := range e.handbooks[nsID] {
		out = append(out, *cloneHandbook(hb))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].WorkerID < out[j].WorkerID })
	return out, nil
}

// ---------------------------------------------------------------------------
// Knowledge & Pitfall search
// ---------------------------------------------------------------------------

func (e *Engine) FindKnowledge(ctx context.Context, nsID, query string) ([]KnowledgeResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	q := strings.ToLower(query)
	out := make([]KnowledgeResult, 0)

	for wid, hb := range e.handbooks[nsID] {
		for _, k := range hb.Knowledge {
			if matchAny(q, append([]string{k.Topic, k.Content}, k.Tags...)...) {
				out = append(out, KnowledgeResult{
					WorkerID: wid,
					Item:     k,
				})
			}
		}
	}
	return out, nil
}

func (e *Engine) FindPitfalls(ctx context.Context, nsID, query string) ([]PitfallResult, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	q := strings.ToLower(query)
	out := make([]PitfallResult, 0)

	for wid, hb := range e.handbooks[nsID] {
		for _, p := range hb.Pitfalls {
			if matchAny(q, append([]string{p.Scenario, p.Problem, p.Solution}, p.Tags...)...) {
				out = append(out, PitfallResult{
					WorkerID: wid,
					Item:     p,
				})
			}
		}
	}
	return out, nil
}

type KnowledgeResult struct {
	WorkerID string        `json:"worker_id"`
	Item     KnowledgeItem `json:"item"`
}

type PitfallResult struct {
	WorkerID string      `json:"worker_id"`
	Item     PitfallItem `json:"item"`
}

func matchAny(q string, fields ...string) bool {
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// WorkerDiary
// ---------------------------------------------------------------------------

type WorkerDiary struct {
	WorkerID    string   `json:"worker_id"`
	NamespaceID string   `json:"namespace_id"`
	Date        string   `json:"date"`
	TaskID      string   `json:"task_id,omitempty"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags,omitempty"`
	CreatedAt   time.Time
}

type WriteDiaryRequest struct {
	NamespaceID string
	WorkerID    string
	Date        string
	TaskID      string
	Content     string
	Tags        []string
}

func (e *Engine) WriteWorkerDiary(ctx context.Context, req WriteDiaryRequest) (*WorkerDiary, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[req.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	if _, ok := e.workers[req.NamespaceID][req.WorkerID]; !ok {
		return nil, errors.New("worker not found")
	}

	now := time.Now().UTC()
	key := diaryKey(req.WorkerID, req.Date)

	if existing, ok := e.workerDiaries[req.NamespaceID][key]; ok {
		// Append content to existing entry for the same day
		existing.Content += "\n---\n" + req.Content
		if req.Tags != nil {
			existing.Tags = append(existing.Tags, req.Tags...)
		}
		if req.TaskID != "" {
			existing.TaskID = req.TaskID
		}
		if e.db != nil {
			if err := upsertWorkerDiary(e.db, existing); err != nil {
				return nil, fmt.Errorf("persist diary: %w", err)
			}
		}
		return cloneWorkerDiary(existing), nil
	}

	d := &WorkerDiary{
		WorkerID:    req.WorkerID,
		NamespaceID: req.NamespaceID,
		Date:        req.Date,
		TaskID:      req.TaskID,
		Content:     req.Content,
		Tags:        cloneStrings(req.Tags),
		CreatedAt:   now,
	}
	if e.workerDiaries[req.NamespaceID] == nil {
		e.workerDiaries[req.NamespaceID] = make(map[string]*WorkerDiary)
	}
	e.workerDiaries[req.NamespaceID][key] = d

	if e.db != nil {
		if err := upsertWorkerDiary(e.db, d); err != nil {
			delete(e.workerDiaries[req.NamespaceID], key)
			return nil, fmt.Errorf("persist diary: %w", err)
		}
	}
	return cloneWorkerDiary(d), nil
}

func (e *Engine) GetWorkerDiary(ctx context.Context, nsID, workerID, date string) (*WorkerDiary, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	d, ok := e.workerDiaries[nsID][diaryKey(workerID, date)]
	if !ok {
		return nil, errors.New("diary entry not found")
	}
	return cloneWorkerDiary(d), nil
}

func (e *Engine) ListWorkerDiaries(ctx context.Context, nsID, workerID string) ([]WorkerDiary, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	out := make([]WorkerDiary, 0)
	for _, d := range e.workerDiaries[nsID] {
		if d.WorkerID == workerID {
			out = append(out, *cloneWorkerDiary(d))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

// ---------------------------------------------------------------------------
// LeaderDiary
// ---------------------------------------------------------------------------

type DiaryEntry struct {
	Type      string    `json:"type"`
	DAGID     string    `json:"dag_id,omitempty"`
	TaskID    string    `json:"task_id,omitempty"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type LeaderDiary struct {
	NamespaceID string       `json:"namespace_id"`
	Date        string       `json:"date"`
	Entries     []DiaryEntry `json:"entries"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type WriteLeaderDiaryRequest struct {
	NamespaceID string
	Date        string
	Entry       DiaryEntry
}

func (e *Engine) WriteLeaderDiary(ctx context.Context, req WriteLeaderDiaryRequest) (*LeaderDiary, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[req.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}

	now := time.Now().UTC()

	if existing, ok := e.leaderDiaries[req.NamespaceID][req.Date]; ok {
		req.Entry.Timestamp = now
		existing.Entries = append(existing.Entries, req.Entry)
		existing.UpdatedAt = now
		if e.db != nil {
			if err := upsertLeaderDiary(e.db, existing); err != nil {
				return nil, fmt.Errorf("persist leader diary: %w", err)
			}
		}
		return cloneLeaderDiary(existing), nil
	}

	ld := &LeaderDiary{
		NamespaceID: req.NamespaceID,
		Date:        req.Date,
		Entries:     []DiaryEntry{req.Entry},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if e.leaderDiaries[req.NamespaceID] == nil {
		e.leaderDiaries[req.NamespaceID] = make(map[string]*LeaderDiary)
	}
	e.leaderDiaries[req.NamespaceID][req.Date] = ld

	if e.db != nil {
		if err := upsertLeaderDiary(e.db, ld); err != nil {
			delete(e.leaderDiaries[req.NamespaceID], req.Date)
			return nil, fmt.Errorf("persist leader diary: %w", err)
		}
	}
	return cloneLeaderDiary(ld), nil
}

func (e *Engine) GetLeaderDiary(ctx context.Context, nsID, date string) (*LeaderDiary, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	ld, ok := e.leaderDiaries[nsID][date]
	if !ok {
		return nil, errors.New("leader diary entry not found")
	}
	return cloneLeaderDiary(ld), nil
}

func (e *Engine) ListLeaderDiaries(ctx context.Context, nsID string) ([]LeaderDiary, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	out := make([]LeaderDiary, 0, len(e.leaderDiaries[nsID]))
	for _, ld := range e.leaderDiaries[nsID] {
		out = append(out, *cloneLeaderDiary(ld))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func diaryKey(workerID, date string) string { return workerID + "|" + date }

func cloneHandbook(hb *WorkerHandbook) *WorkerHandbook {
	if hb == nil {
		return nil
	}
	cpy := *hb
	cpy.TechStack = cloneStrings(hb.TechStack)
	cpy.Knowledge = copyKnowledge(hb.Knowledge)
	cpy.Pitfalls = copyPitfalls(hb.Pitfalls)
	return &cpy
}

func copyKnowledge(items []KnowledgeItem) []KnowledgeItem {
	if items == nil {
		return nil
	}
	out := make([]KnowledgeItem, len(items))
	copy(out, items)
	return out
}

func copyPitfalls(items []PitfallItem) []PitfallItem {
	if items == nil {
		return nil
	}
	out := make([]PitfallItem, len(items))
	copy(out, items)
	return out
}

func cloneWorkerDiary(d *WorkerDiary) *WorkerDiary {
	if d == nil {
		return nil
	}
	cpy := *d
	cpy.Tags = cloneStrings(d.Tags)
	return &cpy
}

func cloneLeaderDiary(ld *LeaderDiary) *LeaderDiary {
	if ld == nil {
		return nil
	}
	cpy := *ld
	cpy.Entries = make([]DiaryEntry, len(ld.Entries))
	copy(cpy.Entries, ld.Entries)
	return &cpy
}

// Ensure imports used
var _ = errors.New("unused")
