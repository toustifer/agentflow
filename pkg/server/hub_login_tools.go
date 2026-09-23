package server

import (
	"context"
	"strings"

	"github.com/toustifer/agentflow/pkg/hub"
)

// This file exposes the two Hub *credential* entry points that do not name a
// team: the browser device-code login (pkg/hub/login.go) and "which teams do I
// belong to" (pkg/hub/teams.go).
//
// Both are read-only with respect to agentflow state: nothing here writes
// SQLite and nothing here invents a team code. hub_login only persists a JWT to
// ~/.agent-hub/config.json, which stays JWT-only by design — a machine-wide team
// bind would make two namespaces fight over one Hub team
// (docs/SYNC_CONTRACT.md §2.4).
//
// Soft-fail applies here too (docs/SYNC_CONTRACT.md §1): a Hub outage or a bad
// code yields a payload with status "failed", never a JSON-RPC error, so an
// operator reading MCP output can tell "not configured" from "tried and failed".

// Login payload statuses. They are deliberately distinct from hub.Status so the
// two-phase device flow can report "the human has not clicked Approve yet"
// without borrowing the word "failed".
const (
	hubLoginStatusPendingApproval = "pending_approval"
	hubLoginStatusOK              = "ok"
	hubLoginStatusFailed          = "failed"
)

// Login payload keys shared by both phases, so a polling caller can tell which
// step produced the row and what to do next.
const (
	hubLoginStepStart  = "start"
	hubLoginStepFinish = "finish"
)

// handleHubLogin runs one step of the browser device-code flow.
//
//	hub_login({})            → code + verification_url (step=start)
//	hub_login({code})        → poll once (step=finish)
//
// Without a code it starts the flow. With a code it polls the Hub exactly once:
// an unapproved device is reported as status "pending_approval" (never
// "failed"), so the model can simply call hub_login again with the same code.
//
// On approval the JWT is written to ~/.agent-hub/config.json. The token is never
// echoed back in the payload (docs/SYNC_CONTRACT.md §3.3) and no business_code
// is written: home stays JWT-only.
func (s *Server) handleHubLogin(ctx context.Context, input map[string]any) (map[string]any, error) {
	code, _ := optionalString(input, "code")
	code = strings.TrimSpace(code)

	nsMeta, workdir, err := s.hubResolveTarget(ctx, input)
	if err != nil {
		return nil, err
	}
	// The device flow needs no credential, only a base URL. Resolving it through
	// the same layers as every other Hub call (env > workdir > home) means a
	// self-hosted Hub configured by HUB_BASE_URL or a workdir config is honoured
	// without a new input field.
	baseURL := hub.LoadForNamespace(nsMeta, workdir).BaseURL

	if code == "" {
		return hubLoginStartPayload(ctx, baseURL), nil
	}
	return hubLoginFinishPayload(ctx, baseURL, code), nil
}

// hubLoginStartPayload starts the device flow and reports where the human must go.
func hubLoginStartPayload(ctx context.Context, baseURL string) map[string]any {
	start, res := hub.StartDeviceLogin(ctx, baseURL)

	out := map[string]any{
		"status":      string(res.Status),
		"op":          res.Op,
		"note":        res.Note(),
		"step":        hubLoginStepStart,
		"approved":    false,
		"base_url":    baseURL,
		"home_config": hub.HomeConfigPath(),
	}
	switch res.Status {
	case hub.StatusOK:
		out["status"] = hubLoginStatusPendingApproval
		out["code"] = start.Code
		out["verification_url"] = start.VerificationURL
		out["expires_in"] = start.ExpiresIn
		out["next"] = "Open verification_url in a browser, click Approve, then call " +
			"hub_login({code: \"" + start.Code + "\"}) to finish. The code stays valid until it expires."
	case hub.StatusDisabled:
		out["hint"] = "Hub I/O is off (HUB_SYNC / HUB_DISABLED / HUB_ENABLED). Unset the kill switch, then call hub_login again."
	default:
		out["status"] = hubLoginStatusFailed
		out["error"] = res.Message
		out["hint"] = "Could not reach " + baseURL + " — check HUB_BASE_URL / network, then call hub_login again."
	}
	return out
}

// hubLoginFinishPayload polls the device token endpoint once and reports the
// outcome. "pending" is a soft, retryable state — not a failure.
func hubLoginFinishPayload(ctx context.Context, baseURL, code string) map[string]any {
	token, res := hub.FinishDeviceLogin(ctx, baseURL, code)

	out := map[string]any{
		"status":      string(res.Status),
		"op":          res.Op,
		"note":        res.Note(),
		"step":        hubLoginStepFinish,
		"code":        code,
		"approved":    false,
		"base_url":    baseURL,
		"home_config": hub.HomeConfigPath(),
	}
	switch res.Status {
	case hub.StatusOK:
		out["status"] = hubLoginStatusOK
		out["approved"] = true
		out["logged_in"] = true
		// The token itself is never returned: it lives in home config only.
		out["token_saved"] = true
		out["token_present"] = token != ""
		// Home is JWT-only, so there is no team code to report — and none was
		// invented. Pinned by TestHubLoginFinishSuccessKeepsHomeJWTOOnly.
		out["business_code"] = ""
		out["home_config_jwt_only"] = true
		out["next"] = "JWT saved. Call hub_list_teams to discover your team codes, " +
			"then hub_bind_team({namespace_id, business_code}) per namespace."
	case hub.StatusSkipped:
		// 202 / {"status":"pending"}: the human has not clicked Approve yet.
		out["status"] = hubLoginStatusPendingApproval
		out["pending"] = true
		out["hint"] = "Not approved yet — open the verification_url from the start step and click Approve, " +
			"then call hub_login({code: \"" + code + "\"}) again."
		out["next"] = "hub_login({code: \"" + code + "\"})"
	case hub.StatusDisabled:
		out["hint"] = "Hub I/O is off (HUB_SYNC / HUB_DISABLED / HUB_ENABLED). Unset the kill switch, then call hub_login again."
	default:
		if res.Code == 400 || res.Code == 404 || res.Code == 410 {
			out["hint"] = "The code was rejected or has expired — restart with hub_login({}) to get a fresh code."
		} else {
			out["hint"] = "Could not reach " + baseURL + " — check HUB_BASE_URL / network, then call hub_login({code}) again."
		}
		out["status"] = hubLoginStatusFailed
		out["error"] = res.Message
	}
	return out
}

// handleHubListTeams lists the Hub teams the logged-in user belongs to
// (GET /v1/hub/me/businesses). It is the discovery half of hub_bind_team: it
// answers "which business_code do I pass?".
//
// It needs a JWT and no team code. An API key alone is not enough: the client's
// JWT guard returns StatusSkipped before any request is dialled, and this
// handler reports that as status "skipped" with a hint pointing at hub_login —
// never as "failed".
func (s *Server) handleHubListTeams(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsMeta, workdir, err := s.hubResolveTarget(ctx, input)
	if err != nil {
		return nil, err
	}
	client := hub.NewFromNamespace(nsMeta, workdir)
	cfg := client.Config()

	teams, res := client.ListMyTeams(ctx)
	if teams == nil {
		teams = []hub.Business{}
	}

	out := map[string]any{
		"status":      string(res.Status),
		"op":          res.Op,
		"note":        res.Note(),
		"count":       len(teams),
		"teams":       teams,
		"has_jwt":     cfg.HasJWT(),
		"has_api_key": cfg.HasAPIKey(),
		"source":      cfg.Source,
		"workdir":     cfg.Workdir,
	}
	switch res.Status {
	case hub.StatusOK:
		out["next"] = "hub_bind_team({namespace_id, business_code}) with one of the codes above."
	case hub.StatusSkipped:
		out["hint"] = "hub_list_teams needs a Hub JWT and an API key alone is not enough — " +
			"call hub_login first (start, then finish with the code from the verification_url)."
		out["next"] = []string{
			"hub_login({}) — returns code + verification_url",
			"open verification_url and click Approve",
			"hub_login({code}) — repeat until status is 'ok'",
			"hub_list_teams({})",
		}
	case hub.StatusDisabled:
		out["hint"] = "Hub I/O is off (HUB_SYNC / HUB_DISABLED / HUB_ENABLED). Unset the kill switch, then retry hub_list_teams."
	default:
		if res.Code == 401 || res.Code == 403 {
			out["hint"] = "The stored JWT was rejected (expired?) — run hub_login again to refresh it."
			out["next"] = []string{"hub_login({})", "hub_login({code}) after approving in the browser", "hub_list_teams({})"}
		} else {
			out["hint"] = "The Hub rejected the request — retry, or check HUB_BASE_URL / network."
		}
		out["error"] = res.Message
		out["http_status"] = res.Code
	}
	return out, nil
}

// hubResolveTarget resolves the namespace metadata + workdir a Hub credential
// lookup should use.
//
// Both are optional. With neither, only the machine-wide home layer
// (~/.agent-hub/config.json) applies — which is exactly what hub_login and
// hub_list_teams want by default. A namespace_id contributes its metadata and,
// when workdir input is absent, the workdir recorded on the namespace.
func (s *Server) hubResolveTarget(ctx context.Context, input map[string]any) (nsMeta map[string]string, workdir string, err error) {
	nsID, _ := optionalString(input, "namespace_id")
	nsID = strings.TrimSpace(nsID)
	workdir, _ = optionalString(input, "workdir")
	workdir = strings.TrimSpace(workdir)

	if nsID == "" || s == nil || s.engine == nil {
		return nil, workdir, nil
	}
	ns, err := s.engine.GetNamespace(ctx, nsID)
	if err != nil {
		return nil, "", err
	}
	if ns != nil {
		nsMeta = ns.Metadata
		if workdir == "" && nsMeta != nil {
			workdir = strings.TrimSpace(nsMeta["workdir"])
		}
	}
	return nsMeta, workdir, nil
}
