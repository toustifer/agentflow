package server

import (
	"github.com/toustifer/agentflow/pkg/engine"
)

type Server struct {
	engine        *engine.Engine
	cfg           Config
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
		//
		// There is deliberately no second, process-local Hub seam: the old
		// pkg/server.HubSyncer (installed only when Config.HubEnabled was set,
		// which cmd/agentflow never did) was removed — see docs/SYNC_CONTRACT.md
		// §4.4. One seam, and it is this one.
		projector: realHubProjector{},
	}

	// Lazy-init Python BT bridge on first BT tool call
	globalBTBridge = nil

	return srv, nil
}
