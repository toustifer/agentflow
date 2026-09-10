package engine

import (
	"errors"
	"time"
)

var (
	ErrGoalNotFound        = errors.New("goal not found")
	ErrDuplicateGoal       = errors.New("goal already exists")
	ErrGoalAlreadyPromoted = errors.New("goal has already been promoted")
	ErrInvalidGoalStatus   = errors.New("invalid goal status")
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
