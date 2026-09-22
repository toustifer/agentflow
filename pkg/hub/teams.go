package hub

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
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

// ListMyTeams GETs /v1/hub/me/businesses with the JWT.
//
// Unlike the write paths this needs no business_code: its whole purpose is to
// discover which codes the user may pick.
func (c *Client) ListMyTeams(ctx context.Context) ([]Business, Result) {
	const op = "list_teams"
	if r, ok := c.guardJWT(op); !ok {
		return nil, r
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
	return teams, Result{Status: StatusOK, Op: op, Message: strconv.Itoa(len(teams)) + " teams"}
}

// parseBusinessStructs renders the /me/businesses payload as Business rows.
// Unknown extra fields are ignored; rows without a code are dropped.
func parseBusinessStructs(body []byte) []Business {
	items := parseBusinessItems(body)
	if len(items) == 0 {
		// Fall back to the codes-only parse so an unexpected envelope still
		// yields usable codes instead of an empty list.
		codes, roles := parseBusinessList(body)
		out := make([]Business, 0, len(codes))
		for i, code := range codes {
			b := Business{Code: code}
			if i < len(roles) {
				b.Role = roles[i]
			}
			out = append(out, b)
		}
		return out
	}
	out := make([]Business, 0, len(items))
	for _, item := range items {
		code := stringField(item, "code", "business_code")
		if strings.TrimSpace(code) == "" {
			continue
		}
		b := Business{
			Code:        code,
			Name:        stringField(item, "name"),
			Description: stringField(item, "description"),
			Status:      stringField(item, "status"),
			Role:        stringField(item, "role"),
		}
		switch id := item["id"].(type) {
		case float64:
			b.ID = int64(id)
		case string:
			if n, err := strconv.ParseInt(id, 10, 64); err == nil {
				b.ID = n
			}
		case json.Number:
			if n, err := id.Int64(); err == nil {
				b.ID = n
			}
		}
		out = append(out, b)
	}
	return out
}
