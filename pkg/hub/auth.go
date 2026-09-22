package hub

import (
	"context"
	"encoding/json"
	"strings"
)

// AuthInfo is a soft membership check result. It never blocks the local engine.
type AuthInfo struct {
	Result
	// BusinessCode is the code the check was performed for.
	BusinessCode string
	// Role is filled when /me/businesses reports one.
	Role string
	// Member is true only when the Hub confirmed access.
	Member bool
}

// EnsureMembership verifies that the configured credential can access
// BusinessCode, caching a successful probe for MembershipCacheTTL.
//
// Decoupling contract (this is NOT a hard gate):
//
//	no client / kill-switch → StatusDisabled, Member=false
//	missing credential      → StatusSkipped,  Member=false
//	transport / 401 / 403   → StatusFailed,   Member=false
//	confirmed access        → StatusOK,       Member=true (cached)
//
// A higher layer may choose policy on Member==false; the default federation
// policy is that local tasks always proceed and Hub writes are attempted with
// the server as the real enforcer.
func (c *Client) EnsureMembership(ctx context.Context) AuthInfo {
	const op = "auth"
	if r, ok := c.guard(op); !ok {
		return AuthInfo{Result: r, BusinessCode: c.safeBiz()}
	}
	if ok, hit := c.cachedMembership(); hit {
		if ok {
			return AuthInfo{
				Result:       Result{Status: StatusOK, Op: op, Message: "cached"},
				BusinessCode: c.cfg.BusinessCode,
				Member:       true,
			}
		}
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Message: "cached non-member"},
			BusinessCode: c.cfg.BusinessCode,
		}
	}
	if c.cfg.HasJWT() {
		return c.ensureMembershipJWT(ctx)
	}
	return c.ensureMembershipKeyProbe(ctx)
}

// ensureMembershipJWT probes GET /v1/hub/me/businesses and looks for our code.
func (c *Client) ensureMembershipJWT(ctx context.Context) AuthInfo {
	const op = "auth"
	status, body, err := c.doJSON(ctx, "GET", "/v1/hub/me/businesses", nil)
	if err != nil {
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Message: err.Error()},
			BusinessCode: c.cfg.BusinessCode,
		}
	}
	switch {
	case status == 401 || status == 403:
		c.InvalidateAuth()
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Code: status, Message: "unauthorized"},
			BusinessCode: c.cfg.BusinessCode,
		}
	case status >= 300:
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Code: status, Message: "list businesses failed"},
			BusinessCode: c.cfg.BusinessCode,
		}
	}

	codes, roles := parseBusinessList(body)
	want := strings.TrimSpace(c.cfg.BusinessCode)
	role, member := "", false
	for i, code := range codes {
		if strings.EqualFold(strings.TrimSpace(code), want) {
			member = true
			if i < len(roles) {
				role = roles[i]
			}
			break
		}
	}
	c.cacheMembership(member)
	if !member {
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Message: "not a member of " + want},
			BusinessCode: want,
		}
	}
	return AuthInfo{
		Result:       Result{Status: StatusOK, Op: op, Message: "member"},
		BusinessCode: want,
		Role:         role,
		Member:       true,
	}
}

// ensureMembershipKeyProbe is the API-key path: GET /v1/hub/dag/{code} is the
// cheapest route that goes through RequireMembership for a machine credential.
//
// 404 is tolerated (business present but empty DAG on some builds); the server
// remains the enforcer for the actual write.
func (c *Client) ensureMembershipKeyProbe(ctx context.Context) AuthInfo {
	const op = "auth"
	status, _, err := c.doJSON(ctx, "GET", "/v1/hub/dag/"+c.cfg.BusinessCode, nil)
	if err != nil {
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Message: err.Error()},
			BusinessCode: c.cfg.BusinessCode,
		}
	}
	switch {
	case status == 401 || status == 403:
		c.InvalidateAuth()
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Code: status, Message: "unauthorized"},
			BusinessCode: c.cfg.BusinessCode,
		}
	case status >= 200 && status < 300:
		c.cacheMembership(true)
		return AuthInfo{
			Result:       Result{Status: StatusOK, Op: op, Message: "key probe ok"},
			BusinessCode: c.cfg.BusinessCode,
			Member:       true,
		}
	case status == 404:
		c.cacheMembership(true)
		return AuthInfo{
			Result:       Result{Status: StatusOK, Op: op, Message: "key probe 404 tolerated"},
			BusinessCode: c.cfg.BusinessCode,
			Member:       true,
		}
	default:
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Code: status, Message: "key probe failed"},
			BusinessCode: c.cfg.BusinessCode,
		}
	}
}

// parseBusinessList accepts the response shapes the Hub has shipped:
// a bare array, {data:[...]}, {businesses:[...]}, {data:{items:[...]}}.
func parseBusinessList(body []byte) (codes []string, roles []string) {
	for _, item := range parseBusinessItems(body) {
		code := stringField(item, "code", "business_code")
		if code == "" {
			continue
		}
		codes = append(codes, code)
		roles = append(roles, stringField(item, "role"))
	}
	return codes, roles
}

// parseBusinessItems unwraps the envelope and returns the row maps.
func parseBusinessItems(body []byte) []map[string]any {
	var top map[string]json.RawMessage
	raw := json.RawMessage(body)
	if json.Unmarshal(body, &top) == nil {
		if v, ok := top["data"]; ok {
			raw = v
		} else if v, ok := top["businesses"]; ok {
			raw = v
		}
	}
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) == nil {
		return arr
	}
	// data may itself be an envelope: {items:[...]} / {businesses:[...]}
	var wrap map[string]json.RawMessage
	if json.Unmarshal(raw, &wrap) == nil {
		if v, ok := wrap["items"]; ok {
			_ = json.Unmarshal(v, &arr)
		} else if v, ok := wrap["businesses"]; ok {
			_ = json.Unmarshal(v, &arr)
		}
	}
	return arr
}

func stringField(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}
