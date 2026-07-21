package server

import (
	"context"
	"strings"

	"github.com/toustifer/agentflow/pkg/hub"
)

// handleHubStatus returns local Hub bind state (no network).
// Optional workdir improves file discovery for hub-client.json.
func (s *Server) handleHubStatus(_ context.Context, input map[string]any) (map[string]any, error) {
	workdir := firstNonEmpty(strArg(input, "workdir"), s.workdirFromNamespace(input))
	snap := hub.Snapshot(workdir)
	return map[string]any{
		"enabled":       snap.Enabled,
		"disabled":      snap.Disabled,
		"base_url":      snap.BaseURL,
		"has_token":     snap.HasToken,
		"has_api_key":   snap.HasAPIKey,
		"business_code": snap.BusinessCode,
		"source":        snap.Source,
		"bound":         snap.Bound,
		"hint":          snap.Hint,
		// AI onboarding path
		"next": nextHubSteps(snap),
	}, nil
}

func nextHubSteps(snap hub.StatusSnapshot) []string {
	if snap.Disabled {
		return []string{"Hub killed by env; unset HUB_SYNC/HUB_DISABLED or set HUB_ENABLED=1 for federation"}
	}
	var steps []string
	if !snap.HasToken && !snap.HasAPIKey {
		steps = append(steps, "hub_login() → open verification_url → hub_login({code})")
	}
	if snap.HasToken && snap.BusinessCode == "" {
		steps = append(steps, "hub_list_teams()", "hub_bind_team({business_code})")
	}
	if snap.Bound {
		steps = append(steps, "ready — soft task/branch projection active on create/transition")
	}
	return steps
}

// handleHubLogin device auth: no code → start; with code → finish + persist JWT.
func (s *Server) handleHubLogin(ctx context.Context, input map[string]any) (map[string]any, error) {
	workdir := firstNonEmpty(strArg(input, "workdir"), s.workdirFromNamespace(input))
	base := firstNonEmpty(strArg(input, "base_url"), hub.Load(workdir).BaseURL)
	code := strings.TrimSpace(strArg(input, "code"))

	if code == "" {
		start, r := hub.StartDeviceLogin(ctx, base)
		out := map[string]any{
			"step":   1,
			"status": string(r.Status),
			"note":   r.Note(),
		}
		if r.OK() {
			out["code"] = start.Code
			out["verification_url"] = start.VerificationURL
			out["expires_in"] = start.ExpiresIn
			out["instruction"] = "Open verification_url in a browser while logged into Hub, click Approve, then call hub_login({code: \"" + start.Code + "\"})."
		}
		return out, nil
	}

	token, r := hub.FinishDeviceLogin(ctx, base, code)
	out := map[string]any{
		"step":   2,
		"status": string(r.Status),
		"note":   r.Note(),
	}
	if r.OK() {
		out["logged_in"] = true
		out["token_saved"] = true
		// never return full token to model logs if avoidable — omit token value
		_ = token
		out["next"] = []string{"hub_list_teams()", "hub_bind_team({business_code})"}
		out["hint"] = "JWT saved to ~/.agent-hub/config.json. List teams and bind one."
	}
	return out, nil
}

// handleHubListTeams lists businesses for the JWT user.
func (s *Server) handleHubListTeams(ctx context.Context, input map[string]any) (map[string]any, error) {
	workdir := firstNonEmpty(strArg(input, "workdir"), s.workdirFromNamespace(input))
	c := hub.NewFromWorkdir(workdir)
	teams, r := c.ListMyTeams(ctx)
	items := make([]map[string]any, 0, len(teams))
	for _, t := range teams {
		items = append(items, map[string]any{
			"code":        t.Code,
			"name":        t.Name,
			"role":        t.Role,
			"status":      t.Status,
			"description": t.Description,
		})
	}
	out := map[string]any{
		"status": string(r.Status),
		"note":   r.Note(),
		"teams":  items,
		"count":  len(items),
	}
	if r.OK() && len(items) > 0 {
		out["hint"] = "Call hub_bind_team({business_code: \"<code>\"}) to bind the active team for soft projection."
	}
	if r.OK() && len(items) == 0 {
		out["hint"] = "No teams yet. Create one on https://hub.stifer.xyz (Dashboard) then re-list, or join via invite."
	}
	return out, nil
}

// handleHubBindTeam writes business_code into home (and optional workdir) config.
func (s *Server) handleHubBindTeam(ctx context.Context, input map[string]any) (map[string]any, error) {
	workdir := firstNonEmpty(strArg(input, "workdir"), s.workdirFromNamespace(input))
	code := strings.TrimSpace(strArg(input, "business_code"))
	// allow explicit unbind
	if _, ok := input["business_code"]; ok && code == "" {
		// explicit empty → unbind
	} else if code == "" {
		return map[string]any{
			"status": "failed",
			"note":   "hub_bind_team_failed: business_code required (or empty string to unbind with explicit key)",
			"hint":   "Pass business_code from hub_list_teams, or business_code:\"\" to unbind.",
		}, nil
	}
	c := hub.NewFromWorkdir(workdir)
	r := c.BindTeam(ctx, code, workdir)
	snap := hub.Snapshot(workdir)
	return map[string]any{
		"status":        string(r.Status),
		"note":          r.Note(),
		"business_code": snap.BusinessCode,
		"bound":         snap.Bound,
		"hint":          snap.Hint,
	}, nil
}

func strArg(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	v, ok := input[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func (s *Server) workdirFromNamespace(input map[string]any) string {
	if s == nil {
		return ""
	}
	ns := strArg(input, "namespace_id")
	if ns == "" {
		return ""
	}
	return s.namespaceWorkdir(context.Background(), ns)
}
