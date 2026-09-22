package hub

import (
	"context"
	"net/http"
	"testing"
)

// TestListMyTeamsAcceptsEnvelopeShapes: the Hub has shipped several response
// envelopes; all of them must yield the same team list.
func TestListMyTeamsAcceptsEnvelopeShapes(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantName string
		wantRole string
		wantID   int64
	}{
		{"data array", `{"data":[{"id":1,"code":"z8gw","name":"Zhiji","role":"owner"}]}`, "Zhiji", "owner", 1},
		{"businesses array", `{"businesses":[{"id":1,"code":"z8gw","name":"Zhiji","role":"owner"}]}`, "Zhiji", "owner", 1},
		{"bare array", `[{"id":1,"code":"z8gw","name":"Zhiji","role":"owner"}]`, "Zhiji", "owner", 1},
		{"data.items", `{"data":{"items":[{"id":1,"code":"z8gw","name":"Zhiji","role":"owner"}]}}`, "Zhiji", "owner", 1},
		{"data.businesses", `{"data":{"businesses":[{"id":1,"code":"z8gw","name":"Zhiji","role":"owner"}]}}`, "Zhiji", "owner", 1},
		{"business_code alias", `{"data":[{"id":1,"business_code":"z8gw"}]}`, "", "", 1},
		{"string id", `{"data":[{"id":"42","code":"z8gw"}]}`, "", "", 42},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHubEnv(t)
			h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
				_, _ = w.Write([]byte(tc.body))
			})
			c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))

			teams, res := c.ListMyTeams(context.Background())
			if !res.OK() {
				t.Fatalf("res=%+v", res)
			}
			if len(teams) != 1 {
				t.Fatalf("teams=%+v want exactly 1", teams)
			}
			if teams[0].Code != "z8gw" || teams[0].Name != tc.wantName || teams[0].Role != tc.wantRole {
				t.Fatalf("team=%+v want code=z8gw name=%q role=%q", teams[0], tc.wantName, tc.wantRole)
			}
			if teams[0].ID != tc.wantID {
				t.Fatalf("team id=%d want %d", teams[0].ID, tc.wantID)
			}
			if got := h.count("GET", "/v1/hub/me/businesses"); got != 1 {
				t.Fatalf("requests=%d", got)
			}
			if got := h.lastRequest(t).Header.Get("Authorization"); got != "Bearer jwt-abc" {
				t.Fatalf("Authorization=%q", got)
			}
		})
	}
}

func TestListMyTeamsSkipsRowsWithoutACode(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		_, _ = w.Write([]byte(`{"data":[{"name":"no code"},{"code":"z8gw"},{"business_code":""}]}`))
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	teams, res := c.ListMyTeams(context.Background())
	if !res.OK() || len(teams) != 1 || teams[0].Code != "z8gw" {
		t.Fatalf("teams=%+v res=%+v", teams, res)
	}
}

func TestListMyTeamsEmptyListIsOK(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	teams, res := c.ListMyTeams(context.Background())
	if !res.OK() || len(teams) != 0 {
		t.Fatalf("teams=%+v res=%+v", teams, res)
	}
	assertNote(t, res, "hub_list_teams_ok: 0 teams")
}

// TestListMyTeamsGuardBranches: listing needs a JWT but no business_code, and
// the guard branches must stay offline.
func TestListMyTeamsGuardBranches(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, nil)
	ctx := context.Background()

	// JWT present but no business_code -> still allowed (that is the point).
	noCode := newTestClient(t, h.URL(), map[string]string{"HUB_TOKEN": "jwt-abc"})
	teams, res := noCode.ListMyTeams(ctx)
	if !res.OK() || teams == nil {
		t.Fatalf("JWT-only listing must work: %+v", res)
	}

	// API key only -> skipped, jwt is required for /me/businesses.
	keyOnly := newTestClient(t, h.URL(), map[string]string{
		"HUB_TOKEN": "", "HUB_API_KEY": "k", "HUB_BUSINESS_CODE": "z8gw",
	})
	_, res = keyOnly.ListMyTeams(ctx)
	if res.Status != StatusSkipped {
		t.Fatalf("key-only listing must skip: %+v", res)
	}
	assertNote(t, res, "hub_list_teams_skipped: not logged in — call hub_login first")

	// Kill switch -> disabled.
	killed := newTestClient(t, h.URL(), map[string]string{"HUB_TOKEN": "jwt", "HUB_SYNC": "0"})
	_, res = killed.ListMyTeams(ctx)
	if res.Status != StatusDisabled {
		t.Fatalf("killed listing must be disabled: %+v", res)
	}

	if n := h.count("GET", "/v1/hub/me/businesses"); n != 1 {
		t.Fatalf("only the JWT call should have dialled, got %d requests", n)
	}
}

func TestListMyTeamsUnauthorizedInvalidates(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, statusHandler(http.StatusUnauthorized, `{"error":"expired"}`))
	c := newTestClient(t, h.URL(), jwtEnv("jwt-expired"))

	teams, res := c.ListMyTeams(context.Background())
	if teams != nil || res.Status != StatusFailed || res.Code != 401 {
		t.Fatalf("teams=%v res=%+v", teams, res)
	}
	mustContain(t, res.Note(), "hub_list_teams_failed: status 401")
	if _, hit := c.cachedMembership(); hit {
		t.Fatal("401 must not leave a cached membership")
	}
}

func TestListMyTeamsServerError(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, statusHandler(http.StatusBadGateway, `upstream down`))
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	_, res := c.ListMyTeams(context.Background())
	if res.Status != StatusFailed || res.Code != 502 {
		t.Fatalf("res=%+v", res)
	}
	mustContain(t, res.Message, "upstream down")
}
