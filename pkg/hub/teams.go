package hub

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Business is one Hub team the logged-in user belongs to.
type Business struct {
	ID          int64  `json:"id,omitempty"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status,omitempty"`
	Role        string `json:"role,omitempty"`
}

// StatusSnapshot is local Hub bind state (no network unless Ping=true).
type StatusSnapshot struct {
	Enabled      bool   `json:"enabled"`
	Disabled     bool   `json:"disabled"`
	BaseURL      string `json:"base_url"`
	HasToken     bool   `json:"has_token"`
	HasAPIKey    bool   `json:"has_api_key"`
	BusinessCode string `json:"business_code"`
	Source       string `json:"source"`
	Bound        bool   `json:"bound"` // token + business_code present and not killed
	Hint         string `json:"hint,omitempty"`
}

// Snapshot reads local config only (never fails hard).
func Snapshot(workdir string) StatusSnapshot {
	cfg := Load(workdir)
	s := StatusSnapshot{
		Enabled:      cfg.Enabled(),
		Disabled:     cfg.Disabled,
		BaseURL:      cfg.BaseURL,
		HasToken:     cfg.HasJWT(),
		HasAPIKey:    strings.TrimSpace(cfg.APIKey) != "",
		BusinessCode: cfg.BusinessCode,
		Source:       cfg.Source,
		Bound:        cfg.Enabled(),
	}
	switch {
	case cfg.Disabled:
		s.Hint = "Hub kill-switch on (HUB_SYNC=0 / HUB_DISABLED / HUB_ENABLED=false). Local-only mode."
	case !cfg.HasJWT() && strings.TrimSpace(cfg.APIKey) == "":
		s.Hint = "Not logged in. Call hub_login() (device browser flow), then hub_list_teams + hub_bind_team."
	case strings.TrimSpace(cfg.BusinessCode) == "":
		s.Hint = "Logged in but no team bound. Call hub_list_teams then hub_bind_team({business_code})."
	case cfg.Enabled():
		s.Hint = "Bound. Soft task/branch projection will run when local tasks change."
	default:
		s.Hint = "Incomplete Hub credentials."
	}
	return s
}

// guardJWT allows JWT-only ops (list teams) without business_code.
func (c *Client) guardJWT(op string) (Result, bool) {
	if c == nil || c.cfg == nil {
		return Result{Status: StatusDisabled, Op: op, Message: "no client"}, false
	}
	if c.cfg.Disabled {
		return Result{Status: StatusDisabled, Op: op, Message: "HUB_SYNC/HUB_ENABLED off"}, false
	}
	if !c.cfg.HasJWT() {
		return Result{Status: StatusSkipped, Op: op, Message: "not logged in — call hub_login first"}, false
	}
	return Result{}, true
}

// ListMyTeams GET /v1/hub/me/businesses (JWT). Does not require business_code.
func (c *Client) ListMyTeams(ctx context.Context) ([]Business, Result) {
	const op = "list_teams"
	if r, ok := c.guardJWT(op); !ok {
		return nil, r
	}
	// ensure BaseURL for doJSON
	if c.cfg.BaseURL == "" {
		c.cfg.BaseURL = "https://hub.stifer.xyz"
	}
	status, body, err := c.doJSON(ctx, "GET", "/v1/hub/me/businesses", nil)
	if err != nil {
		return nil, Result{Status: StatusFailed, Op: op, Message: err.Error()}
	}
	if status == 401 || status == 403 {
		c.InvalidateAuth()
		return nil, Result{Status: StatusFailed, Op: op, Code: status, Message: "unauthorized — re-run hub_login"}
	}
	if status >= 300 {
		return nil, Result{Status: StatusFailed, Op: op, Code: status, Message: truncateMsg(string(body))}
	}
	teams := parseBusinessStructs(body)
	return teams, Result{Status: StatusOK, Op: op, Message: itoa(len(teams)) + " teams"}
}

// BindTeam writes business_code to ~/.agent-hub/config.json (and optional workdir).
// If code is empty, unbinds (ClearBusinessCode).
// When code non-empty and JWT present, verifies membership first (soft fail still writes only on OK).
func (c *Client) BindTeam(ctx context.Context, code, workdir string) Result {
	const op = "bind_team"
	if c == nil || c.cfg == nil {
		return Result{Status: StatusDisabled, Op: op, Message: "no client"}
	}
	if c.cfg.Disabled {
		return Result{Status: StatusDisabled, Op: op, Message: "HUB_SYNC/HUB_ENABLED off"}
	}
	code = strings.TrimSpace(code)
	if code == "" {
		if err := ClearBusinessCode(); err != nil {
			return Result{Status: StatusFailed, Op: op, Message: err.Error()}
		}
		c.cfg.BusinessCode = ""
		c.InvalidateAuth()
		return Result{Status: StatusOK, Op: op, Message: "unbound (local-only)"}
	}
	if !c.cfg.HasJWT() && strings.TrimSpace(c.cfg.APIKey) == "" {
		return Result{Status: StatusSkipped, Op: op, Message: "not logged in — call hub_login first"}
	}
	// Verify JWT membership when possible
	if c.cfg.HasJWT() {
		teams, lr := c.ListMyTeams(ctx)
		if lr.Status == StatusFailed {
			return Result{Status: StatusFailed, Op: op, Code: lr.Code, Message: "list teams before bind: " + lr.Message}
		}
		if lr.OK() {
			found := false
			for _, t := range teams {
				if strings.EqualFold(t.Code, code) {
					found = true
					break
				}
			}
			if !found {
				return Result{Status: StatusFailed, Op: op, Message: "not a member of " + code + " — pick a code from hub_list_teams"}
			}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := SaveHomeConfig(homeConfigFile{
		BaseURL:      c.cfg.BaseURL,
		HubURL:       c.cfg.BaseURL,
		BusinessCode: code,
		BoundAt:      now,
	}); err != nil {
		return Result{Status: StatusFailed, Op: op, Message: "save home config: " + err.Error()}
	}
	if workdir != "" {
		_ = SaveWorkdirClient(workdir, homeConfigFile{
			BaseURL:      c.cfg.BaseURL,
			BusinessCode: code,
			Token:        c.cfg.Token,
		})
	}
	c.cfg.BusinessCode = code
	c.InvalidateAuth()
	return Result{Status: StatusOK, Op: op, Message: "bound " + code + " → ~/.agent-hub/config.json"}
}

func parseBusinessStructs(body []byte) []Business {
	codes, roles := parseBusinessList(body)
	// re-parse for names
	var top map[string]json.RawMessage
	_ = json.Unmarshal(body, &top)
	var raw json.RawMessage
	if v, ok := top["data"]; ok {
		raw = v
	} else if v, ok := top["businesses"]; ok {
		raw = v
	} else {
		raw = body
	}
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) != nil {
		var wrap map[string]json.RawMessage
		if json.Unmarshal(raw, &wrap) == nil {
			if v, ok := wrap["items"]; ok {
				_ = json.Unmarshal(v, &arr)
			} else if v, ok := wrap["businesses"]; ok {
				_ = json.Unmarshal(v, &arr)
			}
		}
	}
	out := make([]Business, 0, len(arr))
	if len(arr) == 0 {
		// fallback from codes only
		for i, code := range codes {
			b := Business{Code: code}
			if i < len(roles) {
				b.Role = roles[i]
			}
			out = append(out, b)
		}
		return out
	}
	for _, item := range arr {
		code := stringField(item, "code", "business_code")
		if code == "" {
			continue
		}
		b := Business{
			Code:        code,
			Name:        stringField(item, "name"),
			Description: stringField(item, "description"),
			Status:      stringField(item, "status"),
			Role:        stringField(item, "role"),
		}
		if id, ok := item["id"].(float64); ok {
			b.ID = int64(id)
		}
		out = append(out, b)
	}
	return out
}
