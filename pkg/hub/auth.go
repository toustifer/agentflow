package hub

import (
	"context"
	"encoding/json"
	"strings"
)

// AuthInfo is a soft membership check result (never blocks local engine).
type AuthInfo struct {
	Result
	BusinessCode string
	Role         string // when known from /me/businesses
	Member       bool
}

// EnsureMembership verifies the configured token/key can access BusinessCode.
//
// Decoupling:
//   - No config / kill-switch → StatusDisabled|Skipped, Member=false (caller continues local-only)
//   - Network / 401 / 403 → StatusFailed, Member=false (still soft for task lifecycle)
//   - Success → StatusOK, Member=true (cached briefly)
//
// This is NOT a hard gate unless a higher layer chooses to treat Member==false as policy.
// Default product policy for federation: local tasks always proceed; Hub writes skip if !Member.
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
			Member:       false,
		}
	}

	// Prefer JWT path: GET /v1/hub/me/businesses and look for business_code.
	// Machine key: probe GET /v1/hub/dag/:code (membership-or-key via RequireMembership).
	if c.cfg.HasJWT() {
		return c.ensureMembershipJWT(ctx)
	}
	return c.ensureMembershipKeyProbe(ctx)
}

func (c *Client) ensureMembershipJWT(ctx context.Context) AuthInfo {
	const op = "auth"
	status, body, err := c.doJSON(ctx, "GET", "/v1/hub/me/businesses", nil)
	if err != nil {
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Message: err.Error()},
			BusinessCode: c.cfg.BusinessCode,
		}
	}
	if status == 401 || status == 403 {
		c.cacheMembership(false)
		c.InvalidateAuth()
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Code: status, Message: "unauthorized"},
			BusinessCode: c.cfg.BusinessCode,
			Member:       false,
		}
	}
	if status >= 300 {
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Code: status, Message: "list businesses failed"},
			BusinessCode: c.cfg.BusinessCode,
		}
	}
	// Response shapes vary; accept data as array or {businesses:[]}/{data:[]}.
	codes, roles := parseBusinessList(body)
	want := strings.TrimSpace(c.cfg.BusinessCode)
	role, member := "", false
	for i, code := range codes {
		if strings.EqualFold(code, want) {
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
			Member:       false,
		}
	}
	return AuthInfo{
		Result:       Result{Status: StatusOK, Op: op, Message: "member"},
		BusinessCode: want,
		Role:         role,
		Member:       true,
	}
}

func (c *Client) ensureMembershipKeyProbe(ctx context.Context) AuthInfo {
	const op = "auth"
	// Lightweight membership/key probe: GetDAG requires RequireMembership or matching key.
	path := "/v1/hub/dag/" + c.cfg.BusinessCode
	status, _, err := c.doJSON(ctx, "GET", path, nil)
	if err != nil {
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Message: err.Error()},
			BusinessCode: c.cfg.BusinessCode,
		}
	}
	if status == 401 || status == 403 {
		c.cacheMembership(false)
		return AuthInfo{
			Result:       Result{Status: StatusFailed, Op: op, Code: status, Message: "unauthorized"},
			BusinessCode: c.cfg.BusinessCode,
			Member:       false,
		}
	}
	if status == 404 {
		// business missing vs empty dag — treat as auth ok if not 401/403
		// 404 on business often returned as 404 message; still not proof of membership.
		// Accept 200 only as strong signal; 404 with body "business not found" = fail.
	}
	if status >= 200 && status < 300 {
		c.cacheMembership(true)
		return AuthInfo{
			Result:       Result{Status: StatusOK, Op: op, Message: "key probe ok"},
			BusinessCode: c.cfg.BusinessCode,
			Member:       true,
		}
	}
	// 404 may mean empty route ok on some builds; be conservative:
	if status == 404 {
		// Allow key path to proceed for write attempts; membership enforced server-side.
		c.cacheMembership(true)
		return AuthInfo{
			Result:       Result{Status: StatusOK, Op: op, Message: "key probe 404 tolerated"},
			BusinessCode: c.cfg.BusinessCode,
			Member:       true,
		}
	}
	return AuthInfo{
		Result:       Result{Status: StatusFailed, Op: op, Code: status, Message: "key probe failed"},
		BusinessCode: c.cfg.BusinessCode,
		Member:       false,
	}
}

func (c *Client) safeBiz() string {
	if c == nil || c.cfg == nil {
		return ""
	}
	return c.cfg.BusinessCode
}

func parseBusinessList(body []byte) (codes []string, roles []string) {
	var top map[string]json.RawMessage
	if json.Unmarshal(body, &top) != nil {
		return nil, nil
	}
	// Try data, businesses, or root array
	var raw json.RawMessage
	if v, ok := top["data"]; ok {
		raw = v
	} else if v, ok := top["businesses"]; ok {
		raw = v
	} else {
		// maybe body is array
		raw = body
	}
	var arr []map[string]any
	if json.Unmarshal(raw, &arr) != nil {
		// data might be {items:[]}
		var wrap map[string]json.RawMessage
		if json.Unmarshal(raw, &wrap) == nil {
			if v, ok := wrap["items"]; ok {
				_ = json.Unmarshal(v, &arr)
			} else if v, ok := wrap["businesses"]; ok {
				_ = json.Unmarshal(v, &arr)
			}
		}
	}
	for _, item := range arr {
		code := stringField(item, "code", "business_code")
		if code == "" {
			continue
		}
		codes = append(codes, code)
		roles = append(roles, stringField(item, "role"))
	}
	return codes, roles
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
