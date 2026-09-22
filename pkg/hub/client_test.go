package hub

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestDoJSONCredentialHeaders pins JWT-over-API-key preference and the header
// names the Hub expects.
func TestDoJSONCredentialHeaders(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, nil)

	jwtClient := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	if _, _, err := jwtClient.doJSON(context.Background(), "GET", "/probe", nil); err != nil {
		t.Fatal(err)
	}
	req := h.lastRequest(t)
	if got := req.Header.Get("Authorization"); got != "Bearer jwt-abc" {
		t.Fatalf("Authorization=%q", got)
	}
	if req.Header.Get("X-API-Key") != "" {
		t.Fatal("JWT path must not send X-API-Key")
	}

	keyClient := newTestClient(t, h.URL(), keyEnv("key-xyz"))
	if _, _, err := keyClient.doJSON(context.Background(), "GET", "/probe", nil); err != nil {
		t.Fatal(err)
	}
	req = h.lastRequest(t)
	if req.Header.Get("Authorization") != "" {
		t.Fatal("key path must not send Authorization")
	}
	if got := req.Header.Get("X-API-Key"); got != "key-xyz" {
		t.Fatalf("X-API-Key=%q", got)
	}
	if got := req.Header.Get("X-Business-Code"); got != "z8gw" {
		t.Fatalf("X-Business-Code=%q", got)
	}

	// Both present -> JWT wins.
	both := jwtEnv("jwt-abc")
	both["HUB_API_KEY"] = "key-xyz"
	bothClient := newTestClient(t, h.URL(), both)
	if _, _, err := bothClient.doJSON(context.Background(), "GET", "/probe", nil); err != nil {
		t.Fatal(err)
	}
	req = h.lastRequest(t)
	if req.Header.Get("Authorization") != "Bearer jwt-abc" {
		t.Fatalf("JWT must win when both are set: %+v", req.Header)
	}
	if req.Header.Get("X-API-Key") != "" {
		t.Fatal("must not send the API key alongside a JWT")
	}
}

// TestEnsureMembershipProbesOnlyOnce pins the membership cache: three calls,
// exactly one GET /v1/hub/me/businesses.
func TestEnsureMembershipProbesOnlyOnce(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		_, _ = w.Write([]byte(`{"data":[{"code":"z8gw","role":"owner"}]}`))
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	ctx := context.Background()

	first := c.EnsureMembership(ctx)
	if !first.Member || !first.OK() {
		t.Fatalf("first probe: %+v", first)
	}
	if first.Role != "owner" {
		t.Fatalf("role=%q want owner", first.Role)
	}
	if first.Message != "member" {
		t.Fatalf("message=%q want member", first.Message)
	}
	for i := 0; i < 2; i++ {
		again := c.EnsureMembership(ctx)
		if !again.Member || again.Message != "cached" {
			t.Fatalf("call %d must hit the cache: %+v", i+2, again)
		}
	}

	got := h.count("GET", "/v1/hub/me/businesses")
	if got != 1 {
		t.Fatalf("me/businesses probed %d times, want exactly 1 (paths=%v)", got, h.requests())
	}
}

// TestEnsureMembership401IsNeverCached: a rejected credential must be re-probed
// on the next call, not remembered as a stale verdict.
func TestEnsureMembership401IsNeverCached(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, statusHandler(http.StatusUnauthorized, `{"error":"token expired"}`))
	c := newTestClient(t, h.URL(), jwtEnv("jwt-expired"))
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		got := c.EnsureMembership(ctx)
		if got.Member {
			t.Fatal("401 must not report a member")
		}
		if got.Status != StatusFailed || got.Code != 401 {
			t.Fatalf("call %d: %+v", i+1, got)
		}
		assertNote(t, got.Result, "hub_auth_failed: status 401 unauthorized")
	}
	if n := h.count("GET", "/v1/hub/me/businesses"); n != 2 {
		t.Fatalf("401 must not be cached: probed %d times want 2", n)
	}
	if _, hit := c.cachedMembership(); hit {
		t.Fatal("cache must be empty after 401")
	}
}

func TestEnsureMembership403InvalidatesCache(t *testing.T) {
	isolateHubEnv(t)
	// Request #1 is a successful probe; anything after that is rejected.
	h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, n int) {
		if n == 1 {
			_, _ = w.Write([]byte(`{"data":[{"code":"z8gw"}]}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	ctx := context.Background()

	if got := c.EnsureMembership(ctx); !got.Member {
		t.Fatalf("first probe should succeed: %+v", got)
	}
	if _, hit := c.cachedMembership(); !hit {
		t.Fatal("successful probe should be cached")
	}
	got := c.EnsureMembership(ctx)
	if !got.Member || got.Message != "cached" {
		t.Fatalf("second call must be served from the cache: %+v", got)
	}
	c.InvalidateAuth()
	if _, hit := c.cachedMembership(); hit {
		t.Fatal("InvalidateAuth must clear the cache")
	}
	third := c.EnsureMembership(ctx)
	if third.Member || third.Code != 403 || third.Status != StatusFailed {
		t.Fatalf("post-invalidation probe must re-hit the server: %+v", third)
	}
	if probes := h.count("GET", "/v1/hub/me/businesses"); probes != 2 {
		t.Fatalf("probes=%d want 2 (cache hit must not dial)", probes)
	}
}

// TestSyncTask401DropsCachedMembership is the end-to-end reason InvalidateAuth
// exists: a write-path rejection must not leave a stale "member" verdict cached.
func TestSyncTask401DropsCachedMembership(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, r *http.Request, n int) {
		// Only the very first probe succeeds; the DAG write and the re-probe
		// after it are both rejected with 401.
		if n == 1 && r.Method == "GET" && r.URL.Path == "/v1/hub/me/businesses" {
			_, _ = w.Write([]byte(`{"data":[{"code":"z8gw"}]}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	ctx := context.Background()

	if got := c.EnsureMembership(ctx); !got.Member {
		t.Fatalf("prime: %+v", got)
	}
	res := c.SyncTask(ctx, TaskProjection{TaskID: "T1", Status: "executing"})
	if res.Status != StatusFailed || res.Code != 401 {
		t.Fatalf("write should fail 401: %+v", res)
	}
	if _, hit := c.cachedMembership(); hit {
		t.Fatal("a 401 write must invalidate the cached membership")
	}
	// Next call must actually re-probe: the server now always 401s, so we see
	// a second probe rather than a cached success.
	got := c.EnsureMembership(ctx)
	if got.Member {
		t.Fatal("stale member verdict leaked")
	}
	if probes := h.count("GET", "/v1/hub/me/businesses"); probes != 2 {
		t.Fatalf("probes=%d want 2", probes)
	}
	if writes := h.count("POST", "/v1/hub/dag/z8gw"); writes != 1 {
		t.Fatalf("writes=%d want 1", writes)
	}
}

// TestEnsureMembershipNonMemberIsFailed covers a 200 that omits our team code.
func TestEnsureMembershipNonMemberIsFailed(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		_, _ = w.Write([]byte(`{"data":[{"code":"other"}]}`))
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	got := c.EnsureMembership(context.Background())
	if got.Member || got.Status != StatusFailed {
		t.Fatalf("non-member: %+v", got)
	}
	assertNote(t, got.Result, "hub_auth_failed: not a member of z8gw")
}

// TestEnsureMembershipDisabledAndSkippedAreOffline: the two "we never dialled"
// states must produce zero requests.
func TestEnsureMembershipDisabledAndSkippedAreOffline(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, nil)
	ctx := context.Background()

	disabled := newTestClient(t, h.URL(), func() map[string]string {
		e := jwtEnv("jwt-abc")
		e["HUB_SYNC"] = "0"
		return e
	}())
	got := disabled.EnsureMembership(ctx)
	if got.Status != StatusDisabled || got.Member {
		t.Fatalf("disabled: %+v", got)
	}
	assertNote(t, got.Result, "hub_auth_disabled: HUB_SYNC/HUB_ENABLED off")

	// Enabled=false because the team code is missing.
	skipped := newTestClient(t, h.URL(), map[string]string{
		"HUB_TOKEN": "jwt-abc", "HUB_SYNC": "", "HUB_BUSINESS_CODE": "",
	})
	got = skipped.EnsureMembership(ctx)
	if got.Status != StatusSkipped || got.Member {
		t.Fatalf("skipped: %+v", got)
	}
	assertNote(t, got.Result, "hub_auth_skipped: no login token / business_code")

	// nil config behaves as disabled.
	nilCfg := New(nil)
	got = nilCfg.EnsureMembership(ctx)
	if got.Status != StatusDisabled {
		t.Fatalf("nil config: %+v", got)
	}

	assertNoRequests(t, h)
}

// TestEnsureMembershipKeyProbe covers the API-key path (no JWT).
func TestEnsureMembershipKeyProbe(t *testing.T) {
	cases := []struct {
		name     string
		code     int
		want     Status
		member   bool
		wantNote string
	}{
		{"200 ok", http.StatusOK, StatusOK, true, "hub_auth_ok: key probe ok"},
		{"404 tolerated", http.StatusNotFound, StatusOK, true, "hub_auth_ok: key probe 404 tolerated"},
		{"403 rejected", http.StatusForbidden, StatusFailed, false, "hub_auth_failed: status 403 unauthorized"},
		{"500 failed", http.StatusInternalServerError, StatusFailed, false, "hub_auth_failed: status 500 key probe failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHubEnv(t)
			h := newHubServer(t, statusHandler(tc.code, ""))
			c := newTestClient(t, h.URL(), keyEnv("key-xyz"))
			got := c.EnsureMembership(context.Background())
			if got.Status != tc.want || got.Member != tc.member {
				t.Fatalf("got %+v want status=%s member=%v", got, tc.want, tc.member)
			}
			assertNote(t, got.Result, tc.wantNote)
			if n := h.count("GET", "/v1/hub/dag/z8gw"); n != 1 {
				t.Fatalf("key probe requests=%d want 1", n)
			}
		})
	}
}

func TestEnsureMembershipTransportFailure(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, nil)
	url := h.URL()
	h.srv.Close() // nothing is listening any more

	c := newTestClient(t, url, jwtEnv("jwt-abc"))
	got := c.EnsureMembership(context.Background())
	if got.Status != StatusFailed || got.Member {
		t.Fatalf("unreachable: %+v", got)
	}
	if got.Message == "" {
		t.Fatal("transport failures must carry a message")
	}
	if !strings.Contains(got.Note(), "hub_auth_failed: ") {
		t.Fatalf("note=%q", got.Note())
	}
}

// TestClientTimeoutIsSoft: a hanging Hub must surface as StatusFailed quickly,
// never as a panic or a blocked caller.
func TestClientTimeoutIsSoft(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, sleepHandler(300*time.Millisecond))

	cfg := &Config{BaseURL: h.URL(), Token: "jwt-abc", BusinessCode: "z8gw"}
	c := newWithHTTP(cfg, &http.Client{Timeout: 40 * time.Millisecond})

	start := time.Now()
	got := c.EnsureMembership(context.Background())
	elapsed := time.Since(start)
	if got.Status != StatusFailed {
		t.Fatalf("timeout must fail softly: %+v", got)
	}
	if got.Code != 0 {
		t.Fatalf("a client timeout has no HTTP status: %+v", got)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("timeout took %s — the deadline was not honoured", elapsed)
	}

	// The same budget applies to write paths.
	res := c.SyncTask(context.Background(), TaskProjection{TaskID: "T1"})
	if res.Status != StatusFailed || res.Code != 0 {
		t.Fatalf("task sync timeout: %+v", res)
	}
}

func TestClientEnabledMirrorsConfig(t *testing.T) {
	isolateHubEnv(t)
	c := New(&Config{BaseURL: DefaultBaseURL, Token: "jwt", BusinessCode: "z8gw"})
	if !c.Enabled() {
		t.Fatal("should be enabled")
	}
	if c.Config().BusinessCode != "z8gw" {
		t.Fatal("Config() must expose the resolved config")
	}
	var nilClient *Client
	if nilClient.Enabled() {
		t.Fatal("nil client must be inert")
	}
	if nilClient.Config().BusinessCode != "" {
		t.Fatal("nil client Config() must be a safe empty config")
	}
}
