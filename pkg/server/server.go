package server

import (
	"github.com/toustifer/agentflow/pkg/engine"
)

type Server struct {
	engine        *engine.Engine
	cfg           Config
	hub           HubSyncer
	phaseProvider *btPhaseProvider
}

func New(e *engine.Engine, cfg Config) (*Server, error) {
	if e == nil {
		return nil, ErrEngineRequired
	}

	srv := &Server{
		engine: e,
		cfg:    cfg,
	}
	if cfg.HubEnabled {
		srv.hub = noopHubSyncer{}
	}

	// Lazy-init Python BT bridge on first BT tool call
	globalBTBridge = nil

	return srv, nil
}
