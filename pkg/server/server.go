package server

import "github.com/stifer/agentflow/pkg/engine"

type Server struct {
	engine *engine.Engine
	cfg    Config
	hub    HubSyncer
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

	return srv, nil
}
