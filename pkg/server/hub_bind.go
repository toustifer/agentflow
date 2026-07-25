package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/toustifer/agentflow/pkg/hub"
)

// handleHubStatus reports resolved business_code + source layers for a namespace.
// Home config is JWT-only; residual home business_code is reported as legacy and not used.
func (s *Server) handleHubStatus(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, _ := optionalString(input, "namespace_id")
	workdir, _ := optionalString(input, "workdir")

	var nsMeta map[string]string
	if nsID != "" {
		ns, err := s.engine.GetNamespace(ctx, nsID)
		if err != nil {
			return nil, err
		}
		nsMeta = ns.Metadata
		if workdir == "" && nsMeta != nil {
			workdir = strings.TrimSpace(nsMeta["workdir"])
		}
	}

	snap := hub.SnapshotForNamespace(nsMeta, workdir)
	snap.NamespaceID = nsID
	out := map[string]any{
		"namespace_id":          snap.NamespaceID,
		"business_code":         snap.BusinessCode,
		"source":                snap.Source,
		"namespace_stored_code": snap.NSStoredCode,
		"workdir":               snap.Workdir,
		"workdir_code":          snap.WorkdirCode,
		"home_legacy_code":      snap.HomeLegacyCode,
		"has_token":             snap.HasToken,
		"bound":                 snap.Bound,
		"hint":                  snap.Hint,
		"resolve_order":         []string{"env", "namespace", "workdir"},
		"note":                  "No machine-wide team bind. ~/.agent-hub/config.json is JWT-only; one namespace ↔ one Hub team.",
	}
	if !snap.Bound {
		out["next"] = []string{
			"Ensure Hub JWT in ~/.agent-hub/config.json (Hub MCP hub_login)",
			"hub_bind_team({namespace_id, business_code}) with 4-char code (e.g. z8gw)",
		}
	}
	return out, nil
}

// handleHubBindTeam binds a Hub team (4-char code) to one namespace (+ workdir file).
// Never writes team code to home config.
func (s *Server) handleHubBindTeam(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	rawCode, err := requiredString(input, "business_code")
	if err != nil {
		return nil, err
	}
	workdir, _ := optionalString(input, "workdir")

	res, err := hub.BindNamespaceTeam(ctx, s.engine, nsID, rawCode, hub.BindOptions{
		Workdir: workdir,
	})
	if err != nil {
		return map[string]any{
			"status": "failed",
			"error":  err.Error(),
			"hint":   "Pass namespace_id + 4-char business_code (or display path like zhiji-z8gw). No home team bind.",
		}, nil
	}

	if ns, gerr := s.engine.GetNamespace(ctx, nsID); gerr == nil {
		s.syncNamespace(ctx, ns)
	}

	return map[string]any{
		"status":              "ok",
		"namespace_id":        res.NamespaceID,
		"business_code":       res.BusinessCode,
		"source":              res.Source,
		"workdir":             res.Workdir,
		"workdir_wrote":       res.WorkdirWrote,
		"home_legacy_cleared": res.HomeCleared,
		"bound_at":            res.BoundAt,
		"message":             res.Message,
		"hint": fmt.Sprintf(
			"Product truth is namespace.metadata[%q]=%s (not ~/.agent-hub/config.json)",
			hub.MetaBusinessCode, res.BusinessCode,
		),
	}, nil
}
