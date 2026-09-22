package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// defaultTimeout bounds every Hub request. Hub is a mirror: a slow Hub must
// never slow down a local task transition.
const defaultTimeout = 5 * time.Second

// maxResponseBytes caps how much of a Hub response we buffer.
const maxResponseBytes = 1 << 20

// MembershipCacheTTL controls how long a successful EnsureMembership is
// remembered. A variable so tests can shorten it.
var MembershipCacheTTL = 5 * time.Minute

// Client is a soft, optional Hub HTTP client.
//
// Safe to use when the Config is disabled: every method returns
// StatusDisabled / StatusSkipped and performs zero network I/O.
//
// Not a hard dependency of the agentflow engine — construct it only at Hub call
// sites (pkg/server). pkg/engine must not import pkg/hub.
type Client struct {
	cfg  *Config
	http *http.Client

	mu     sync.Mutex
	memOK  bool
	memExp time.Time
}

// New builds a client from config. cfg may be nil (treated as disabled).
func New(cfg *Config) *Client {
	if cfg == nil {
		cfg = &Config{Disabled: true, Source: "nil"}
	}
	return &Client{cfg: cfg, http: &http.Client{Timeout: defaultTimeout}}
}

// NewFromWorkdir is the usual entry: Load(workdir) + New.
func NewFromWorkdir(workdir string) *Client {
	return New(Load(workdir))
}

// NewFromNamespace is the namespace-aware entry: LoadForNamespace + New.
// Prefer this when the namespace metadata (hence the bound team code) is known.
func NewFromNamespace(nsMeta map[string]string, workdir string) *Client {
	return New(LoadForNamespace(nsMeta, workdir))
}

// newWithHTTP is the test seam for timeouts and transports.
func newWithHTTP(cfg *Config, hc *http.Client) *Client {
	if cfg == nil {
		cfg = &Config{Disabled: true, Source: "nil"}
	}
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	return &Client{cfg: cfg, http: hc}
}

// Config returns the resolved config (never nil).
func (c *Client) Config() *Config {
	if c == nil || c.cfg == nil {
		return &Config{Disabled: true, Source: "nil"}
	}
	return c.cfg
}

// Enabled is true when soft Hub I/O is allowed.
func (c *Client) Enabled() bool {
	return c != nil && c.cfg != nil && c.cfg.Enabled()
}

// guard is the single gate for write paths: it decides disabled vs skipped vs go.
func (c *Client) guard(op string) (Result, bool) {
	if c == nil || c.cfg == nil {
		return Result{Status: StatusDisabled, Op: op, Message: "no client"}, false
	}
	if c.cfg.Disabled {
		return Result{Status: StatusDisabled, Op: op, Message: "HUB_SYNC/HUB_ENABLED off"}, false
	}
	if !c.cfg.Enabled() {
		return Result{Status: StatusSkipped, Op: op, Message: "no login token / business_code"}, false
	}
	return Result{}, true
}

// guardJWT gates JWT-only operations (team listing) that need no business_code.
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

// doJSON issues an authenticated JSON request. Transport errors are returned,
// never panicked, so callers can map them onto a soft Result.
//
// Credential preference: JWT (Authorization: Bearer) first, then API key
// (X-API-Key + X-Business-Code).
func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any) (status int, respBody []byte, err error) {
	var bodyReader io.Reader
	if reqBody != nil {
		raw, mErr := json.Marshal(reqBody)
		if mErr != nil {
			return 0, nil, fmt.Errorf("marshal: %w", mErr)
		}
		bodyReader = bytes.NewReader(raw)
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.cfg.HasJWT() {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.Token))
	} else if c.cfg.HasAPIKey() {
		req.Header.Set("X-API-Key", strings.TrimSpace(c.cfg.APIKey))
		req.Header.Set("X-Business-Code", strings.TrimSpace(c.cfg.BusinessCode))
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	return resp.StatusCode, data, nil
}

func (c *Client) cacheMembership(ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memOK = ok
	if ok {
		c.memExp = time.Now().Add(MembershipCacheTTL)
	} else {
		c.memExp = time.Time{}
	}
}

func (c *Client) cachedMembership() (ok bool, hit bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.memExp.IsZero() || time.Now().After(c.memExp) {
		return false, false
	}
	return c.memOK, true
}

// InvalidateAuth drops the cached membership decision.
// Call after any 401/403 so the next attempt re-probes instead of trusting a
// stale "member" verdict.
func (c *Client) InvalidateAuth() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memOK = false
	c.memExp = time.Time{}
}

// safeBiz returns the configured business code without dereferencing nil.
func (c *Client) safeBiz() string {
	if c == nil || c.cfg == nil {
		return ""
	}
	return c.cfg.BusinessCode
}

func truncateMsg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
