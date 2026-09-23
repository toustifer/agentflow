package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
	"github.com/toustifer/agentflow/pkg/hub"
)

// ---------------------------------------------------------------------------
// Test rig for the Hub credential tools (hub_login / hub_list_teams).
//
// Every case here talks to an httptest fake Hub. The production Hub is never
// contacted: the machine JWT expired on 2026-07-29, so a real request would 401
// and prove nothing. isolateHubEnv (hub_projection_test.go) redirects HOME /
// USERPROFILE to a temp dir, so FinishDeviceLogin's real write to
// ~/.agent-hub/config.json lands in the sandbox and can never overwrite the
// operator's credential file.
// ---------------------------------------------------------------------------

type deviceHubCall struct {
	method string
	path   string
	query  string
	header http.Header
}

// deviceHub is a minimal fake Hub covering exactly the two device-flow routes
// plus /v1/hub/me/businesses.
type deviceHub struct {
	server *httptest.Server

	mu    sync.Mutex
	calls []deviceHubCall

	// deviceBody / tokenStatus / tokenBody drive POST /v1/hub/auth/device and
	// GET /v1/hub/auth/device/token. teamsStatus / teamsBody drive the listing.
	deviceStatus int
	deviceBody   string
	tokenStatus  int
	tokenBody    string
	teamsStatus  int
	teamsBody    string
}

func newDeviceHub(t *testing.T) *deviceHub {
	t.Helper()
	h := &deviceHub{
		deviceBody:  `{"code":"WDJB-MJHT","verification_url":"https://example.invalid/auth/device?code=WDJB-MJHT","expires_in":900}`,
		tokenStatus: http.StatusOK,
		tokenBody:   `{"data":{"status":"approved","token":"jwt-from-device-flow"}}`,
		teamsBody:   `{"data":[{"id":1,"code":"z8gw","name":"Zhiji","role":"owner"}]}`,
	}
	h.server = httptest.NewServer(http.HandlerFunc(h.handle))
	t.Cleanup(h.server.Close)
	return h
}

func (h *deviceHub) handle(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.calls = append(h.calls, deviceHubCall{
		method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone(),
	})
	deviceStatus, deviceBody := h.deviceStatus, h.deviceBody
	tokenStatus, tokenBody := h.tokenStatus, h.tokenBody
	teamsStatus, teamsBody := h.teamsStatus, h.teamsBody
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/hub/auth/device":
		if deviceStatus != 0 {
			w.WriteHeader(deviceStatus)
		}
		_, _ = w.Write([]byte(deviceBody))
	case r.Method == http.MethodGet && r.URL.Path == "/v1/hub/auth/device/token":
		if tokenStatus != 0 {
			w.WriteHeader(tokenStatus)
		}
		_, _ = w.Write([]byte(tokenBody))
	case r.Method == http.MethodGet && r.URL.Path == "/v1/hub/me/businesses":
		if teamsStatus != 0 {
			w.WriteHeader(teamsStatus)
		}
		_, _ = w.Write([]byte(teamsBody))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (h *deviceHub) URL() string { return h.server.URL }

func (h *deviceHub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.calls)
}

func (h *deviceHub) snapshot() []deviceHubCall {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]deviceHubCall, len(h.calls))
	copy(out, h.calls)
	return out
}

func (h *deviceHub) countRoute(method, path string) int {
	n := 0
	for _, c := range h.snapshot() {
		if c.method == method && c.path == path {
			n++
		}
	}
	return n
}

func (h *deviceHub) last() deviceHubCall {
	calls := h.snapshot()
	return calls[len(calls)-1]
}

// pointHubAt makes the isolated env use h as the Hub base URL.
func pointHubAt(t *testing.T, h *deviceHub) {
	t.Helper()
	t.Setenv("HUB_BASE_URL", h.URL())
}

// ---------------------------------------------------------------------------
// AC 1 — registration + model-facing descriptions
// ---------------------------------------------------------------------------

func TestHubLoginToolsRegisteredWithTwoStepDescriptions(t *testing.T) {
	srv := buildIsolatedServer(t)

	byName := map[string]ToolSpec{}
	names := make([]string, 0, len(srv.Tools()))
	for _, tool := range srv.Tools() {
		names = append(names, tool.Name)
		byName[tool.Name] = tool
	}
	require.Contains(t, names, "hub_login")
	require.Contains(t, names, "hub_list_teams")
	// The pre-existing Hub tools must not have been disturbed.
	require.Contains(t, names, "hub_status")
	require.Contains(t, names, "hub_bind_team")

	login := byName["hub_login"]
	require.NotEmpty(t, login.Description, "hub_login must carry a model-facing description")
	for _, want := range []string{"code", "verification_url", "pending_approval", "~/.agent-hub/config.json"} {
		require.Contains(t, login.Description, want)
	}
	// The description must teach the two-step calling convention.
	require.Contains(t, login.Description, "Step 1")
	require.Contains(t, login.Description, "Step 2")

	list := byName["hub_list_teams"]
	require.NotEmpty(t, list.Description)
	require.Contains(t, list.Description, "hub_login")
	require.Contains(t, list.Description, "skipped")

	// hub_login takes no required argument: empty input starts the flow.
	loginProps, ok := login.InputSchema["properties"].(map[string]any)
	require.True(t, ok, "hub_login needs an input schema: %v", login.InputSchema)
	_, hasCode := loginProps["code"]
	require.True(t, hasCode, "hub_login schema must advertise `code`")
	_, required := login.InputSchema["required"]
	require.False(t, required, "hub_login must require nothing: %v", login.InputSchema)

	// The description must survive JSON serialization into tools/list.
	raw, err := json.Marshal(login)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"description":"Log in to agent-hub`)
}

// ---------------------------------------------------------------------------
// AC 2 — two-phase semantics; "pending" is never a failure
// ---------------------------------------------------------------------------

func TestHubLoginStartReturnsCodeAndVerificationURL(t *testing.T) {
	isolateHubEnv(t)
	h := newDeviceHub(t)
	pointHubAt(t, h)

	srv := buildIsolatedServer(t)
	result, err := srv.Handle(context.Background(), "hub_login", map[string]any{})
	require.NoError(t, err)

	require.Equal(t, "pending_approval", result["status"])
	require.Equal(t, "start", result["step"])
	require.Equal(t, false, result["approved"])
	require.Equal(t, "WDJB-MJHT", result["code"])
	require.Equal(t,
		"https://example.invalid/auth/device?code=WDJB-MJHT",
		result["verification_url"],
	)
	require.Equal(t, 900, result["expires_in"])
	require.Contains(t, result["next"].(string), "Approve")
	require.Contains(t, result["next"].(string), "hub_login")
	// `failed` must appear nowhere: the flow started fine.
	require.NotContains(t, result["note"].(string), "failed")

	require.Equal(t, 1, h.countRoute(http.MethodPost, "/v1/hub/auth/device"))
	require.Equal(t, 0, h.countRoute(http.MethodGet, "/v1/hub/auth/device/token"))
	require.Equal(t, 1, h.count(), "only the device-start route may be dialled")
}

func TestHubLoginFinishPendingIsRetryableNotAFailure(t *testing.T) {
	cases := []struct {
		name  string
		setup func(h *deviceHub)
	}{
		{"202 accepted", func(h *deviceHub) { h.tokenStatus = http.StatusAccepted; h.tokenBody = "" }},
		{"200 pending status", func(h *deviceHub) { h.tokenBody = `{"data":{"status":"pending"}}` }},
		{"200 without token", func(h *deviceHub) { h.tokenBody = `{"data":{}}` }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHubEnv(t)
			h := newDeviceHub(t)
			tc.setup(h)
			pointHubAt(t, h)

			srv := buildIsolatedServer(t)
			// Poll twice: a pending device must stay pollable.
			for i := 0; i < 2; i++ {
				result, err := srv.Handle(context.Background(), "hub_login", map[string]any{"code": "WDJB-MJHT"})
				require.NoError(t, err, "pending approval must never become a tool error")

				require.Equal(t, "pending_approval", result["status"], "poll %d", i)
				require.Equal(t, "finish", result["step"])
				require.Equal(t, false, result["approved"])
				require.NotEqual(t, true, result["logged_in"], "a pending poll must not claim a login")
				require.NotEqual(t, "failed", result["status"])
				require.Contains(t, result["note"].(string), "pending approval")
				require.Contains(t, result["hint"].(string), "Approve")
				require.Contains(t, result["hint"].(string), "WDJB-MJHT")
				// The returned ticket must be the same one the caller passed in.
				require.Equal(t, "WDJB-MJHT", result["code"])
			}
			require.Equal(t, 2, h.countRoute(http.MethodGet, "/v1/hub/auth/device/token"))
		})
	}
}

func TestHubLoginFinishRejectedCodeIsFailed(t *testing.T) {
	isolateHubEnv(t)
	h := newDeviceHub(t)
	h.tokenStatus = http.StatusBadRequest
	h.tokenBody = `{"error":"unknown code"}`
	pointHubAt(t, h)

	srv := buildIsolatedServer(t)
	result, err := srv.Handle(context.Background(), "hub_login", map[string]any{"code": "BAD-CODE"})
	require.NoError(t, err)
	require.Equal(t, "failed", result["status"])
	require.Contains(t, result["note"].(string), "hub_login_finish_failed")
	require.Contains(t, result["hint"].(string), "expired")
	require.NotContains(t, result, "logged_in")
}

func TestHubLoginKillSwitchDialsNothing(t *testing.T) {
	isolateHubEnv(t)
	h := newDeviceHub(t)
	pointHubAt(t, h)
	t.Setenv("HUB_SYNC", "0")

	srv := buildIsolatedServer(t)
	for _, input := range []map[string]any{{}, {"code": "WDJB-MJHT"}} {
		result, err := srv.Handle(context.Background(), "hub_login", input)
		require.NoError(t, err)
		require.Equal(t, "disabled", result["status"])
	}
	require.Equal(t, 0, h.count(), "the kill switch must produce zero outbound requests")
}

// ---------------------------------------------------------------------------
// AC 3 — JWT lands in home config; home stays JWT-only (no invented team code)
// ---------------------------------------------------------------------------

func TestHubLoginFinishSuccessKeepsHomeJWTOOnly(t *testing.T) {
	home := isolateHubEnv(t)
	h := newDeviceHub(t)
	pointHubAt(t, h)

	srv := buildIsolatedServer(t)
	result, err := srv.Handle(context.Background(), "hub_login", map[string]any{"code": "WDJB-MJHT"})
	require.NoError(t, err)

	require.Equal(t, "ok", result["status"])
	require.Equal(t, "finish", result["step"])
	require.Equal(t, true, result["approved"])
	require.Equal(t, true, result["logged_in"])
	require.Equal(t, true, result["token_saved"])
	require.Equal(t, true, result["token_present"])
	require.Contains(t, result["note"].(string), "hub_login_finish_ok")

	// AC 3: home stays JWT-only — no invented team code anywhere in the payload.
	require.Equal(t, "", result["business_code"])
	require.Equal(t, true, result["home_config_jwt_only"])
	require.Equal(t, hub.HomeConfigPath(), result["home_config"])

	// The JWT itself must never be echoed back through MCP (SYNC_CONTRACT §3.3).
	blob, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "jwt-from-device-flow")

	// The file really is in the *isolated* home, and really holds the token.
	wantPath := filepath.Join(home, ".agent-hub", "config.json")
	require.Equal(t, wantPath, hub.HomeConfigPath())
	raw, err := os.ReadFile(wantPath)
	require.NoError(t, err, "device login must persist the JWT to the isolated home config")
	require.Contains(t, string(raw), "jwt-from-device-flow")

	// The persisted file must not carry a business_code key at all.
	var file map[string]any
	require.NoError(t, json.Unmarshal(raw, &file))
	_, hasTeam := file["business_code"]
	require.False(t, hasTeam, "device login must not invent a team code: %v", file)

	// And the client-side view agrees: JWT present, team code empty, legacy empty.
	cfg := hub.Load("")
	require.Equal(t, "jwt-from-device-flow", cfg.Token)
	require.Equal(t, "", cfg.BusinessCode)
	require.Equal(t, "", cfg.BusinessCodeSource)
	require.Equal(t, "", hub.HomeBusinessCodeLegacy())
	require.True(t, cfg.HasJWT())
	require.False(t, cfg.HasAPIKey())

	require.Equal(t, 1, h.countRoute(http.MethodGet, "/v1/hub/auth/device/token"))
	require.Equal(t, "code=WDJB-MJHT", h.last().query)
}

// ---------------------------------------------------------------------------
// AC 4 — hub_list_teams: API-key-only is skipped (with a hub_login hint)
// ---------------------------------------------------------------------------

func TestHubListTeamsReturnsDiscoveredTeamCodes(t *testing.T) {
	isolateHubEnv(t)
	h := newDeviceHub(t)
	pointHubAt(t, h)
	t.Setenv("HUB_TOKEN", "jwt-listed")

	srv := buildIsolatedServer(t)
	result, err := srv.Handle(context.Background(), "hub_list_teams", map[string]any{})
	require.NoError(t, err)

	require.Equal(t, "ok", result["status"])
	require.Equal(t, 1, result["count"])
	teams, ok := result["teams"].([]hub.Business)
	require.True(t, ok, "teams=%v", result["teams"])
	require.Len(t, teams, 1)
	require.Equal(t, "z8gw", teams[0].Code)
	require.Equal(t, "Zhiji", teams[0].Name)
	require.Equal(t, "owner", teams[0].Role)
	require.Equal(t, true, result["has_jwt"])
	require.Contains(t, result["next"].(string), "hub_bind_team")

	require.Equal(t, 1, h.countRoute(http.MethodGet, "/v1/hub/me/businesses"))
	require.Equal(t, "Bearer jwt-listed", h.last().header.Get("Authorization"))
}

func TestHubListTeamsAPIKeyOnlyIsSkippedWithLoginHint(t *testing.T) {
	isolateHubEnv(t)
	h := newDeviceHub(t)
	pointHubAt(t, h)
	// API key + team code, but no JWT: exactly the case guardJWT must skip.
	t.Setenv("HUB_TOKEN", "")
	t.Setenv("HUB_JWT", "")
	t.Setenv("HUB_API_KEY", "machine-key")
	t.Setenv("HUB_BUSINESS_CODE", "z8gw")

	srv := buildIsolatedServer(t)
	result, err := srv.Handle(context.Background(), "hub_list_teams", map[string]any{})
	require.NoError(t, err)

	require.Equal(t, "skipped", result["status"], "API-key-only must be skipped, never failed")
	require.NotEqual(t, "failed", result["status"])
	require.Equal(t, "hub_list_teams_skipped: not logged in — call hub_login first", result["note"])
	require.Equal(t, false, result["has_jwt"])
	require.Equal(t, true, result["has_api_key"])
	require.Equal(t, 0, result["count"])

	hint, ok := result["hint"].(string)
	require.True(t, ok, "skipped must carry a hint: %v", result)
	require.Contains(t, hint, "hub_login")
	require.Contains(t, hint, "API key alone")

	next, ok := result["next"].([]string)
	require.True(t, ok, "skipped must carry an ordered next: %v", result)
	require.Contains(t, next[0], "hub_login({})")
	require.Contains(t, next[len(next)-1], "hub_list_teams({})")

	// The whole point of the guard: nothing was dialled at all.
	require.Equal(t, 0, h.count(), "API-key-only listing must not dial the Hub")
}

func TestHubListTeamsWithoutAnyCredentialIsSkippedOffline(t *testing.T) {
	isolateHubEnv(t)
	h := newDeviceHub(t)
	pointHubAt(t, h)

	srv := buildIsolatedServer(t)
	result, err := srv.Handle(context.Background(), "hub_list_teams", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, "skipped", result["status"])
	require.Contains(t, result["hint"].(string), "hub_login")
	require.Equal(t, 0, h.count())
}

func TestHubListTeamsUnauthorizedHintsRelogin(t *testing.T) {
	isolateHubEnv(t)
	h := newDeviceHub(t)
	h.teamsStatus = http.StatusUnauthorized
	h.teamsBody = `{"error":"jwt expired"}`
	pointHubAt(t, h)
	t.Setenv("HUB_TOKEN", "jwt-expired")

	srv := buildIsolatedServer(t)
	result, err := srv.Handle(context.Background(), "hub_list_teams", map[string]any{})
	require.NoError(t, err, "a Hub 401 is a soft-fail, not a tool error")

	require.Equal(t, "failed", result["status"])
	require.Equal(t, 401, result["http_status"])
	require.Contains(t, result["hint"].(string), "hub_login")
	require.Contains(t, result["note"].(string), "hub_list_teams_failed: status 401")
	require.Equal(t, 1, h.countRoute(http.MethodGet, "/v1/hub/me/businesses"))
}

func TestHubListTeamsKillSwitchDialsNothing(t *testing.T) {
	isolateHubEnv(t)
	h := newDeviceHub(t)
	pointHubAt(t, h)
	t.Setenv("HUB_TOKEN", "jwt-listed")
	t.Setenv("HUB_DISABLED", "1")

	srv := buildIsolatedServer(t)
	result, err := srv.Handle(context.Background(), "hub_list_teams", map[string]any{})
	require.NoError(t, err)
	require.Equal(t, "disabled", result["status"])
	require.Equal(t, 0, h.count())
}

// The optional namespace_id must be accepted (and must not be mistaken for a
// required business_code): with no credentials it is still a soft skip.
func TestHubListTeamsAcceptsNamespaceTarget(t *testing.T) {
	isolateHubEnv(t)
	h := newDeviceHub(t)
	pointHubAt(t, h)

	// newTestServer (not buildIsolatedServer) because this case really resolves
	// the namespace row: ns-1 exists there and carries a workdir.
	srv := newTestServer(t)
	result, err := srv.Handle(context.Background(), "hub_list_teams", map[string]any{"namespace_id": "ns-1"})
	require.NoError(t, err)
	require.Equal(t, "skipped", result["status"])
	require.Equal(t, 0, h.count())

	// An unknown namespace is a real lookup failure and must be reported.
	_, err = srv.Handle(context.Background(), "hub_list_teams", map[string]any{"namespace_id": "ns-does-not-exist"})
	require.ErrorIs(t, err, engine.ErrNamespaceNotFound)
	require.Equal(t, 0, h.count())
}
