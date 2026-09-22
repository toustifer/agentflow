package server

import (
	"github.com/toustifer/agentflow/pkg/engine"
)

type Server struct {
	engine        *engine.Engine
	cfg           Config
	hub           HubSyncer
	projector     hubProjector
	phaseProvider *btPhaseProvider
}

func New(e *engine.Engine, cfg Config) (*Server, error) {
	if e == nil {
		return nil, ErrEngineRequired
	}

	srv := &Server{
		engine: e,
		cfg:    cfg,
		// The L→H projection is always installed; what decides whether it dials
		// is pkg/hub config (team code + credential), not a server flag. With no
		// config it resolves to StatusSkipped and performs zero I/O.
		projector: realHubProjector{},
	}
	if cfg.HubEnabled {
		srv.hub = noopHubSyncer{}
	}

	// Lazy-init Python BT bridge on first BT tool call
	globalBTBridge = nil

	return srv, nil
}
