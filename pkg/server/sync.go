package server

import (
	"context"

	"github.com/toustifer/agentflow/pkg/engine"
)

type HubSyncer interface {
	SyncTask(context.Context, *engine.Task) error
	SyncNamespace(context.Context, *engine.Namespace) error
	Ping(context.Context) error
}

type noopHubSyncer struct{}

func (noopHubSyncer) SyncTask(context.Context, *engine.Task) error {
	return nil
}

func (noopHubSyncer) SyncNamespace(context.Context, *engine.Namespace) error {
	return nil
}

func (noopHubSyncer) Ping(context.Context) error {
	return nil
}

func (s *Server) syncTask(ctx context.Context, task *engine.Task) {
	if s.hub == nil || task == nil {
		return
	}
	_ = s.hub.SyncTask(ctx, task)
}

func (s *Server) syncNamespace(ctx context.Context, ns *engine.Namespace) {
	if s.hub == nil || ns == nil {
		return
	}
	_ = s.hub.SyncNamespace(ctx, ns)
}

func (s *Server) syncPing(ctx context.Context) {
	if s.hub == nil {
		return
	}
	_ = s.hub.Ping(ctx)
}
