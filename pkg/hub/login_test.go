package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveBaseURL uses the exported constant so no test file needs to spell
// the production hostname.
func TestResolveBaseURL(t *testing.T) {
	t.Parallel()
	if got := resolveBaseURL(""); got != DefaultBaseURL {
		t.Fatalf("empty base must fall back to the default: %q", got)
	}
	if got := resolveBaseURL("https://example.invalid/"); got != "https://example.invalid" {
		t.Fatalf("trailing slash must be trimmed: %q", got)
	}
	if got := resolveBaseURL("   "); got != DefaultBaseURL {
		t.Fatalf("blank base must fall back to the default: %q", got)
	}
}

// TestStartDeviceLoginEnvelopeShapes accepts {data:{...}} and a flat body.
func TestStartDeviceLoginEnvelopeShapes(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"data envelope", `{"data":{"code":"WDJB-MJHT","verification_url":"https://example.invalid/auth/device?code=WDJB-MJHT","expires_in":900}}`},
		{"flat body", `{"code":"WDJB-MJHT","verification_url":"https://example.invalid/auth/device?code=WDJB-MJHT","expires_in":900}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHubEnv(t)
			h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
				_, _ = w.Write([]byte(tc.body))
			})
			start, res := StartDeviceLogin(context.Background(), h.URL())
			if !res.OK() {
				t.Fatalf("res=%+v", res)
			}
			if start.Code != "WDJB-MJHT" || start.ExpiresIn != 900 {
				t.Fatalf("start=%+v", start)
			}
			wantURL := "https://example.invalid/auth/device?code=WDJB-MJHT"
			if start.VerificationURL != wantURL {
				t.Fatalf("verification_url=%q want %q", start.VerificationURL, wantURL)
			}
			req := h.lastRequest(t)
			if req.Method != "POST" || req.Path != "/v1/hub/auth/device" {
				t.Fatalf("route=%s %s", req.Method, req.Path)
			}
			assertNote(t, res, "hub_login_start_ok: WDJB-MJHT")
		})
	}
}

// TestStartDeviceLoginSynthesizesVerificationURL: some builds omit it.
func TestStartDeviceLoginSynthesizesVerificationURL(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		_, _ = w.Write([]byte(`{"code":"ABCD-EFGH"}`))
	})
	start, res := StartDeviceLogin(context.Background(), h.URL())
	if !res.OK() {
		t.Fatalf("res=%+v", res)
	}
	want := h.URL() + "/auth/device?code=ABCD-EFGH"
	if start.VerificationURL != want {
		t.Fatalf("verification_url=%q want %q", start.VerificationURL, want)
	}
}

func TestStartDeviceLoginFailures(t *testing.T) {
	t.Run("server error", func(t *testing.T) {
		isolateHubEnv(t)
		h := newHubServer(t, statusHandler(http.StatusInternalServerError, `boom`))
		_, res := StartDeviceLogin(context.Background(), h.URL())
		if res.Status != StatusFailed || res.Code != 500 {
			t.Fatalf("res=%+v", res)
		}
		mustContain(t, res.Message, "boom")
	})
	t.Run("malformed body", func(t *testing.T) {
		isolateHubEnv(t)
		h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
			_, _ = w.Write([]byte(`{"data":{"nope":true}}`))
		})
		_, res := StartDeviceLogin(context.Background(), h.URL())
		if res.Status != StatusFailed || res.Message != "bad device response" {
			t.Fatalf("res=%+v", res)
		}
	})
	t.Run("unreachable", func(t *testing.T) {
		isolateHubEnv(t)
		h := newHubServer(t, nil)
		url := h.URL()
		h.srv.Close()
		_, res := StartDeviceLogin(context.Background(), url)
		if res.Status != StatusFailed || res.Message == "" {
			t.Fatalf("res=%+v", res)
		}
	})
	t.Run("kill switch", func(t *testing.T) {
		isolateHubEnv(t)
		h := newHubServer(t, nil)
		t.Setenv("HUB_SYNC", "0")
		_, res := StartDeviceLogin(context.Background(), h.URL())
		if res.Status != StatusDisabled {
			t.Fatalf("res=%+v", res)
		}
		assertNote(t, res, "hub_login_start_disabled: HUB_SYNC/HUB_ENABLED off")
		assertNoRequests(t, h)
	})
}

// TestFinishDeviceLoginPersistsJWT walks the approved path and proves the token
// lands in the isolated home config, not in the operator's real one.
func TestFinishDeviceLoginPersistsJWT(t *testing.T) {
	home := isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		_, _ = w.Write([]byte(`{"data":{"status":"approved","token":"jwt-from-hub"}}`))
	})

	token, res := FinishDeviceLogin(context.Background(), h.URL(), "WDJB-MJHT")
	if !res.OK() {
		t.Fatalf("res=%+v", res)
	}
	if token != "jwt-from-hub" {
		t.Fatalf("token=%q", token)
	}
	req := h.lastRequest(t)
	if req.Method != "GET" || req.Path != "/v1/hub/auth/device/token" {
		t.Fatalf("route=%s %s", req.Method, req.Path)
	}
	if req.Query != "code=WDJB-MJHT" {
		t.Fatalf("query=%q", req.Query)
	}

	// The JWT is now readable from the (isolated) home config, JWT-only.
	cfg := Load("")
	if cfg.Token != "jwt-from-hub" {
		t.Fatalf("token not persisted: %+v", cfg)
	}
	if cfg.BusinessCode != "" {
		t.Fatal("device login must not invent a team code")
	}
	if HomeBusinessCodeLegacy() != "" {
		t.Fatal("home must stay team-free")
	}
	// The token alone is not an enabled config, but its origin is reported.
	if cfg.Enabled() {
		t.Fatalf("a token without a team code must not be enabled: %+v", cfg)
	}
	if cfg.Source != HomeConfigPath() {
		t.Fatalf("source=%q want the home config path", cfg.Source)
	}
	if cfg.BusinessCodeSource != "" {
		t.Fatalf("device login must not invent a code source: %q", cfg.BusinessCodeSource)
	}

	// Read the file itself: proves the token was written to the sandbox home.
	wantPath := filepath.Join(home, ".agent-hub", "config.json")
	if HomeConfigPath() != wantPath {
		t.Fatalf("HomeConfigPath()=%q want %q", HomeConfigPath(), wantPath)
	}
	raw, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("token file missing: %v", err)
	}
	var file struct {
		Token   string `json:"token"`
		BaseURL string `json:"base_url"`
		LoginAt string `json:"login_at"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("bad config json: %v", err)
	}
	if file.Token != "jwt-from-hub" {
		t.Fatalf("persisted token=%q", file.Token)
	}
	if file.BaseURL != h.URL() {
		t.Fatalf("persisted base_url=%q want %q", file.BaseURL, h.URL())
	}
	if file.LoginAt == "" {
		t.Fatal("login_at should be stamped")
	}
}

// TestFinishDeviceLoginPendingIsSkipped: a not-yet-approved poll is a soft skip,
// never an error, so the caller can poll again.
func TestFinishDeviceLoginPendingIsSkipped(t *testing.T) {
	cases := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request, int)
	}{
		{"202 accepted", statusHandler(http.StatusAccepted, "")},
		{"200 pending status", func(w http.ResponseWriter, _ *http.Request, _ int) {
			_, _ = w.Write([]byte(`{"data":{"status":"pending"}}`))
		}},
		{"200 without token", func(w http.ResponseWriter, _ *http.Request, _ int) {
			_, _ = w.Write([]byte(`{"data":{}}`))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHubEnv(t)
			h := newHubServer(t, tc.handler)
			token, res := FinishDeviceLogin(context.Background(), h.URL(), "WDJB-MJHT")
			if token != "" {
				t.Fatalf("pending must not yield a token: %q", token)
			}
			if res.Status != StatusSkipped {
				t.Fatalf("res=%+v", res)
			}
			assertNote(t, res, "hub_login_finish_skipped: pending approval — open verification URL and click Approve")
		})
	}
}

func TestFinishDeviceLoginFailures(t *testing.T) {
	t.Run("empty code", func(t *testing.T) {
		isolateHubEnv(t)
		h := newHubServer(t, nil)
		_, res := FinishDeviceLogin(context.Background(), h.URL(), "  ")
		if res.Status != StatusFailed || res.Message != "code required" {
			t.Fatalf("res=%+v", res)
		}
		assertNoRequests(t, h)
	})
	t.Run("rejected code", func(t *testing.T) {
		isolateHubEnv(t)
		h := newHubServer(t, statusHandler(http.StatusBadRequest, `{"error":"unknown code"}`))
		_, res := FinishDeviceLogin(context.Background(), h.URL(), "BAD-CODE")
		if res.Status != StatusFailed || res.Code != 400 {
			t.Fatalf("res=%+v", res)
		}
		mustContain(t, res.Message, "unknown code")
	})
	t.Run("unreachable", func(t *testing.T) {
		isolateHubEnv(t)
		h := newHubServer(t, nil)
		url := h.URL()
		h.srv.Close()
		_, res := FinishDeviceLogin(context.Background(), url, "WDJB-MJHT")
		if res.Status != StatusFailed || res.Message == "" {
			t.Fatalf("res=%+v", res)
		}
	})
	t.Run("kill switch", func(t *testing.T) {
		isolateHubEnv(t)
		h := newHubServer(t, nil)
		t.Setenv("HUB_ENABLED", "false")
		_, res := FinishDeviceLogin(context.Background(), h.URL(), "WDJB-MJHT")
		if res.Status != StatusDisabled {
			t.Fatalf("res=%+v", res)
		}
		assertNoRequests(t, h)
	})
}

// TestFinishDeviceLoginEscapesCode keeps odd codes URL-safe.
func TestFinishDeviceLoginEscapesCode(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		_, _ = w.Write([]byte(`{"data":{"token":"jwt"}}`))
	})
	if _, res := FinishDeviceLogin(context.Background(), h.URL(), "A B&C"); !res.OK() {
		t.Fatalf("res=%+v", res)
	}
	if got := h.lastRequest(t).Query; got == "code=A B&C" {
		t.Fatalf("code must be URL-escaped, got raw %q", got)
	}
}
