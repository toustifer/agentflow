package hub

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// hubEnvKeys are every env var the Hub client reads. Tests blank them so a
// developer's real shell config can never leak into a case.
var hubEnvKeys = []string{
	"HUB_BASE_URL", "HUB_TOKEN", "HUB_JWT", "HUB_API_KEY",
	"HUB_BUSINESS_CODE", "HUB_BUSINESS",
	"HUB_SYNC", "HUB_DISABLED", "HUB_ENABLED",
}

// isolateHubEnv gives a test a blank env and a throwaway home directory, so no
// case can read or write the operator's real ~/.agent-hub/config.json.
// Returns the fake home dir. Not usable together with t.Parallel (t.Setenv).
func isolateHubEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, k := range hubEnvKeys {
		t.Setenv(k, "")
	}
	return home
}

func writeHubFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// recordedReq is one request the fake Hub received.
type recordedReq struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
}

// hubServer is an httptest-backed fake Hub that records every request.
//
// No test may talk to the real Hub: the machine JWT expired on 2026-07-29, so a
// genuine request would 401 and prove nothing. Every case here is credential-free.
type hubServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []recordedReq
}

// newHubServer starts a fake Hub. fn receives the request and its 1-based
// sequence number; pass nil for a 200-with-{} default.
func newHubServer(t *testing.T, fn func(w http.ResponseWriter, r *http.Request, n int)) *hubServer {
	t.Helper()
	h := &hubServer{}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		h.mu.Lock()
		h.reqs = append(h.reqs, recordedReq{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Header: r.Header.Clone(),
			Body:   body,
		})
		n := len(h.reqs)
		h.mu.Unlock()

		if fn == nil {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
			return
		}
		fn(w, r, n)
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *hubServer) URL() string { return h.srv.URL }

func (h *hubServer) requests() []recordedReq {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]recordedReq, len(h.reqs))
	copy(out, h.reqs)
	return out
}

// count returns how many requests matched method+path.
func (h *hubServer) count(method, path string) int {
	n := 0
	for _, r := range h.requests() {
		if r.Method == method && r.Path == path {
			n++
		}
	}
	return n
}

func (h *hubServer) lastRequest(t *testing.T) recordedReq {
	t.Helper()
	reqs := h.requests()
	if len(reqs) == 0 {
		t.Fatal("fake hub received no requests")
	}
	return reqs[len(reqs)-1]
}

// lastRequestBodyJSON decodes the final request body into a generic map.
func (h *hubServer) lastRequestBodyJSON(t *testing.T) map[string]any {
	t.Helper()
	req := h.lastRequest(t)
	var m map[string]any
	if err := json.Unmarshal(req.Body, &m); err != nil {
		t.Fatalf("body is not a JSON object: %v (raw=%s)", err, req.Body)
	}
	return m
}

// newTestClient points a Client at the fake Hub under explicit env.
// isolateHubEnv must have run first.
func newTestClient(t *testing.T, baseURL string, env map[string]string) *Client {
	t.Helper()
	t.Setenv("HUB_BASE_URL", baseURL)
	for k, v := range env {
		t.Setenv(k, v)
	}
	return New(Load(""))
}

// jwtEnv is the minimum env for an enabled JWT client.
func jwtEnv(jwt string) map[string]string {
	return map[string]string{"HUB_TOKEN": jwt, "HUB_BUSINESS_CODE": "z8gw"}
}

// keyEnv is the minimum env for an enabled API-key client. HUB_TOKEN is blanked
// explicitly: t.Setenv persists across a test, so a previously built JWT client
// would otherwise leak its token into this one.
func keyEnv(key string) map[string]string {
	return map[string]string{"HUB_TOKEN": "", "HUB_API_KEY": key, "HUB_BUSINESS_CODE": "z8gw"}
}

// sleepHandler delays a response so client-side timeouts can be exercised.
func sleepHandler(d time.Duration) func(http.ResponseWriter, *http.Request, int) {
	return func(w http.ResponseWriter, _ *http.Request, _ int) {
		time.Sleep(d)
		_, _ = w.Write([]byte(`{}`))
	}
}

func statusHandler(code int, body string) func(http.ResponseWriter, *http.Request, int) {
	return func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.WriteHeader(code)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}
}

// assertNote fails when a Result does not render the expected note string.
func assertNote(t *testing.T, r Result, wantNote string) {
	t.Helper()
	if got := r.Note(); got != wantNote {
		t.Fatalf("Note()=%q want %q (result=%+v)", got, wantNote, r)
	}
}

// assertNoRealHubRequest guards that a case produced no outbound request at all.
func assertNoRequests(t *testing.T, h *hubServer) {
	t.Helper()
	if got := len(h.requests()); got != 0 {
		t.Fatalf("expected zero Hub requests, got %d: %+v", got, h.requests())
	}
}

// mustContain fails when the substring is absent.
func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("%q does not contain %q", haystack, needle)
	}
}
