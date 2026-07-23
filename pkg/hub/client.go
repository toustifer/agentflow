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

const defaultTimeout = 5 * time.Second

// Client is a soft Hub HTTP client. Safe to use when Config is disabled:
// methods return StatusDisabled / StatusSkipped and never require network.
//
// Not a hard dependency of the agentflow engine: construct only at Hub call sites.
type Client struct {
	cfg    *Config
	http   *http.Client
	mu     sync.Mutex
	memOK  bool
	memExp time.Time
}

// New builds a client from config. cfg may be nil (treated as disabled).
func New(cfg *Config) *Client {
	if cfg == nil {
		cfg = &Config{Disabled: true, Source: "nil"}
	}
	return &Client{
		cfg: cfg,
		http: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// NewFromWorkdir is the usual entry: Load(workdir) + New.
func NewFromWorkdir(workdir string) *Client {
	return New(Load(workdir))
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

// doJSON issues an authenticated JSON request. Soft on transport errors.
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
	// JWT preferred; machine key fallback.
	if c.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	} else if c.cfg.APIKey != "" {
		req.Header.Set("X-API-Key", c.cfg.APIKey)
		req.Header.Set("X-Business-Code", c.cfg.BusinessCode)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, data, nil
}

// MembershipCacheTTL controls how long a successful EnsureMembership is remembered.
var MembershipCacheTTL = 5 * time.Minute

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

// InvalidateAuth drops membership cache (e.g. after 401/403).
func (c *Client) InvalidateAuth() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memOK = false
	c.memExp = time.Time{}
}
