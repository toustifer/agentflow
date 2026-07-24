package hub

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/toustifer/agentflow/pkg/engine"
)

// BindOptions control BindNamespaceTeam side effects.
type BindOptions struct {
	// Workdir overrides ns.metadata["workdir"] when non-empty.
	Workdir string
	// SetHomeFallback writes business_code into ~/.agent-hub/config.json
	// when true, or when home has no business_code yet.
	SetHomeFallback bool
	// TeamName optional display metadata only.
	TeamName string
}

// BindResult is the durable bind outcome (no network).
type BindResult struct {
	NamespaceID  string `json:"namespace_id"`
	BusinessCode string `json:"business_code"`
	Source       string `json:"source"` // after bind, typically "namespace"
	Workdir      string `json:"workdir,omitempty"`
	WorkdirWrote bool   `json:"workdir_wrote"`
	HomeWrote    bool   `json:"home_wrote"`
	BoundAt      string `json:"bound_at"`
	Message      string `json:"message"`
}

// BindNamespaceTeam normalizes code, merges hub.* into namespace metadata,
// writes workdir hub-client.json when workdir is known, and optionally
// sets home fallback business_code. JWT is never written to SQLite.
func BindNamespaceTeam(ctx context.Context, eng *engine.Engine, namespaceID, rawCode string, opts BindOptions) (*BindResult, error) {
	if eng == nil {
		return nil, fmt.Errorf("engine is required")
	}
	namespaceID = strings.TrimSpace(namespaceID)
	if namespaceID == "" {
		return nil, fmt.Errorf("namespace_id is required")
	}
	code, err := NormalizeBusinessCode(rawCode)
	if err != nil {
		return nil, err
	}

	ns, err := eng.GetNamespace(ctx, namespaceID)
	if err != nil {
		return nil, err
	}

	workdir := strings.TrimSpace(opts.Workdir)
	if workdir == "" && ns.Metadata != nil {
		workdir = strings.TrimSpace(ns.Metadata["workdir"])
	}

	boundAt := time.Now().UTC().Format(time.RFC3339)
	meta := map[string]string{
		MetaBusinessCode: code,
		MetaBoundAt:      boundAt,
	}
	if name := strings.TrimSpace(opts.TeamName); name != "" {
		meta[MetaTeamName] = name
	}

	updated, err := eng.UpdateNamespace(ctx, engine.UpdateNamespaceRequest{
		ID:       namespaceID,
		Metadata: meta,
	})
	if err != nil {
		return nil, fmt.Errorf("update namespace: %w", err)
	}

	res := &BindResult{
		NamespaceID:  namespaceID,
		BusinessCode: code,
		Source:       "namespace",
		Workdir:      workdir,
		BoundAt:      boundAt,
	}

	if workdir != "" {
		if err := SaveWorkdirClient(workdir, homeConfigFile{
			BusinessCode: code,
			BoundAt:      boundAt,
		}); err != nil {
			return nil, fmt.Errorf("write workdir hub-client: %w", err)
		}
		res.WorkdirWrote = true
	}

	homeCode := HomeBusinessCode()
	if opts.SetHomeFallback || homeCode == "" {
		if err := SaveHomeConfig(homeConfigFile{
			BusinessCode: code,
			BoundAt:      boundAt,
		}); err != nil {
			// Home may be unwritable in some sandboxes — surface but keep ns bind.
			res.Message = fmt.Sprintf("namespace bound to %s; home fallback write failed: %v", code, err)
			return res, nil
		}
		res.HomeWrote = true
	}

	// Confirm resolve sees namespace first.
	resolved, source := ResolveBusinessCode(updated.Metadata, workdir)
	res.BusinessCode = resolved
	res.Source = source
	if res.Message == "" {
		res.Message = fmt.Sprintf("bound namespace %s → %s (source=%s)", namespaceID, resolved, source)
	}
	return res, nil
}

// StatusSnapshot is local Hub bind state for one namespace (no network).
type StatusSnapshot struct {
	NamespaceID     string `json:"namespace_id,omitempty"`
	BusinessCode    string `json:"business_code"`
	Source          string `json:"source"`
	NSStoredCode    string `json:"namespace_stored_code,omitempty"`
	Workdir         string `json:"workdir,omitempty"`
	WorkdirCode     string `json:"workdir_code,omitempty"`
	HomeCode        string `json:"home_code,omitempty"`
	HasToken        bool   `json:"has_token"`
	Bound           bool   `json:"bound"`
	Hint            string `json:"hint,omitempty"`
}

// SnapshotForNamespace reports resolved code + layered sources.
func SnapshotForNamespace(nsMeta map[string]string, workdir string) StatusSnapshot {
	if workdir == "" && nsMeta != nil {
		workdir = strings.TrimSpace(nsMeta["workdir"])
	}
	code, source := ResolveBusinessCode(nsMeta, workdir)
	s := StatusSnapshot{
		BusinessCode: code,
		Source:       source,
		Workdir:      workdir,
		HasToken:     HasHomeToken(),
		HomeCode:     HomeBusinessCode(),
		Bound:        code != "" && source != "unbound",
	}
	if nsMeta != nil {
		s.NSStoredCode = strings.TrimSpace(nsMeta[MetaBusinessCode])
	}
	if workdir != "" {
		s.WorkdirCode = readBusinessCodeFile(WorkdirClientPath(workdir))
	}
	switch {
	case !s.HasToken:
		s.Hint = "No Hub JWT in ~/.agent-hub/config.json. Login via Hub MCP hub_login, then bind."
	case code == "":
		s.Hint = "No team bound. Call hub_bind_team({namespace_id, business_code}) with a 4-char code."
	case source == "namespace":
		s.Hint = "Bound on namespace metadata (product truth for this project)."
	case source == "workdir":
		s.Hint = "Using workdir hub-client.json; prefer hub_bind_team to pin namespace metadata."
	case source == "home":
		s.Hint = "Using home fallback only; bind namespace for multi-project isolation."
	case source == "env":
		s.Hint = "HUB_BUSINESS_CODE env overrides all file/namespace sources."
	default:
		s.Hint = "Incomplete Hub bind."
	}
	return s
}
