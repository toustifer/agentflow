package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"
)

var (
	ErrGoalNotFound        = errors.New("goal not found")
	ErrDuplicateGoal       = errors.New("goal already exists")
	ErrGoalAlreadyPromoted = errors.New("goal has already been promoted")
	ErrInvalidGoalStatus   = errors.New("invalid goal status")
	goalIDSeqRegex         = regexp.MustCompile(`^G-(\d+)$`)
)

type GoalStatus string

const (
	GoalPending  GoalStatus = "pending"
	GoalDeferred GoalStatus = "deferred"
	GoalPromoted GoalStatus = "promoted"
	GoalDropped  GoalStatus = "dropped"
)

type Goal struct {
	ID          string            `json:"id"`
	NamespaceID string            `json:"namespace_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      GoalStatus        `json:"status"`
	Priority    int               `json:"priority"`
	Tags        []string          `json:"tags"`
	Context     string            `json:"context"`
	DAGID       string            `json:"dag_id"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type CreateGoalRequest struct {
	NamespaceID string
	ID          string
	Title       string
	Description string
	Priority    int
	Tags        []string
	Context     string
	Metadata    map[string]string
}

type UpdateGoalRequest struct {
	NamespaceID string
	ID          string
	Title       *string
	Description *string
	Status      GoalStatus
	Priority    *int
	Tags        []string
	Context     *string
	Metadata    map[string]string
}

type GoalFilter struct {
	NamespaceID string
	Statuses    []GoalStatus
	Tags        []string
	PriorityGTE *int
}

func (e *Engine) nextGoalIDLocked(nsID string) string {
	maxSeq := 0
	if goals, ok := e.goals[nsID]; ok {
		for id := range goals {
			matches := goalIDSeqRegex.FindStringSubmatch(id)
			if len(matches) == 2 {
				if seq, err := strconv.Atoi(matches[1]); err == nil && seq > maxSeq {
					maxSeq = seq
				}
			}
		}
	}
	return fmt.Sprintf("G-%d", maxSeq+1)
}

func cloneGoal(g *Goal) *Goal {
	if g == nil {
		return nil
	}
	cpy := *g
	if g.Tags != nil {
		cpy.Tags = make([]string, len(g.Tags))
		copy(cpy.Tags, g.Tags)
	}
	if g.Metadata != nil {
		cpy.Metadata = make(map[string]string, len(g.Metadata))
		for k, v := range g.Metadata {
			cpy.Metadata[k] = v
		}
	}
	return &cpy
}

func (e *Engine) CreateGoal(ctx context.Context, req CreateGoalRequest) (*Goal, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[req.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	if req.Title == "" {
		return nil, errors.New("goal title cannot be empty")
	}

	if e.goals[req.NamespaceID] == nil {
		e.goals[req.NamespaceID] = make(map[string]*Goal)
	}

	goalID := req.ID
	if goalID == "" {
		goalID = e.nextGoalIDLocked(req.NamespaceID)
	} else if _, exists := e.goals[req.NamespaceID][goalID]; exists {
		return nil, ErrDuplicateGoal
	}

	now := time.Now()
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}
	meta := req.Metadata
	if meta == nil {
		meta = make(map[string]string)
	}

	goal := &Goal{
		ID:          goalID,
		NamespaceID: req.NamespaceID,
		Title:       req.Title,
		Description: req.Description,
		Status:      GoalPending,
		Priority:    req.Priority,
		Tags:        tags,
		Context:     req.Context,
		DAGID:       "",
		Metadata:    meta,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if e.db != nil {
		if err := insertGoal(e.db, goal); err != nil {
			return nil, err
		}
	}

	e.goals[req.NamespaceID][goalID] = goal
	return cloneGoal(goal), nil
}

func (e *Engine) GetGoal(ctx context.Context, nsID, id string) (*Goal, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	goal, ok := e.goals[nsID][id]
	if !ok {
		return nil, ErrGoalNotFound
	}
	return cloneGoal(goal), nil
}

func (e *Engine) UpdateGoal(ctx context.Context, req UpdateGoalRequest) (*Goal, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.namespaces[req.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}
	goal, ok := e.goals[req.NamespaceID][req.ID]
	if !ok {
		return nil, ErrGoalNotFound
	}

	if goal.Status == GoalPromoted && req.Status != "" && req.Status != GoalPromoted {
		return nil, fmt.Errorf("cannot modify status of already promoted goal: %w", ErrGoalAlreadyPromoted)
	}

	if req.Status != "" {
		switch req.Status {
		case GoalPending, GoalDeferred, GoalDropped, GoalPromoted:
			goal.Status = req.Status
		default:
			return nil, ErrInvalidGoalStatus
		}
	}

	if req.Title != nil {
		goal.Title = *req.Title
	}
	if req.Description != nil {
		goal.Description = *req.Description
	}
	if req.Priority != nil {
		goal.Priority = *req.Priority
	}
	if req.Tags != nil {
		goal.Tags = req.Tags
	}
	if req.Context != nil {
		goal.Context = *req.Context
	}
	if req.Metadata != nil {
		for k, v := range req.Metadata {
			goal.Metadata[k] = v
		}
	}
	goal.UpdatedAt = time.Now()

	if e.db != nil {
		if err := updateGoalRecord(e.db, goal); err != nil {
			return nil, err
		}
	}

	return cloneGoal(goal), nil
}

func (e *Engine) ListGoals(ctx context.Context, filter GoalFilter) ([]Goal, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[filter.NamespaceID]; !ok {
		return nil, ErrNamespaceNotFound
	}

	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []GoalStatus{GoalPending, GoalDeferred}
	}
	statusMap := make(map[GoalStatus]bool, len(statuses))
	for _, s := range statuses {
		statusMap[s] = true
	}

	out := make([]Goal, 0)
	for _, g := range e.goals[filter.NamespaceID] {
		if !statusMap[g.Status] {
			continue
		}
		if filter.PriorityGTE != nil && g.Priority < *filter.PriorityGTE {
			continue
		}
		if len(filter.Tags) > 0 {
			matched := false
			for _, reqTag := range filter.Tags {
				for _, tag := range g.Tags {
					if tag == reqTag {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				continue
			}
		}
		out = append(out, *cloneGoal(g))
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})

	return out, nil
}
