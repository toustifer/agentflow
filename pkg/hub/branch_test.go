package hub

import (
	"context"
	"net/http"
	"os"
	"reflect"
	"sort"
	"testing"
)

// TestReportBranchBodyShape pins the §3.2 wire contract, including the rule that
// the worktree travels as a hostname and never as a filesystem path.
func TestReportBranchBodyShape(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if membershipOK(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))

	res := c.ReportBranch(context.Background(), BranchReport{
		RepoURL:  "https://github.com/org/repo.git",
		Branch:   "feat/hub-federation-rebuild",
		HeadSHA:  "a3a747a",
		BindType: "task",
		BindID:   "task-1-hub-client-package",
		Reporter: "agentflow-dev",
	})
	if !res.OK() {
		t.Fatalf("report should succeed: %+v", res)
	}
	assertNote(t, res, "hub_branch_report_ok")

	req := h.lastRequest(t)
	if req.Method != "POST" || req.Path != "/v1/hub/repos/z8gw/branches/report" {
		t.Fatalf("wrong route: %s %s", req.Method, req.Path)
	}
	body := h.lastRequestBodyJSON(t)

	gotKeys := make([]string, 0, len(body))
	for k := range body {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	wantKeys := []string{"bindings", "branches", "repo_url", "reporter"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("body keys=%v want %v", gotKeys, wantKeys)
	}
	if body["reporter"] != "agentflow-dev" || body["repo_url"] != "https://github.com/org/repo.git" {
		t.Fatalf("scalar fields wrong: %v", body)
	}

	branches, ok := body["branches"].([]any)
	if !ok || len(branches) != 1 {
		t.Fatalf("branches=%v", body["branches"])
	}
	branch := branches[0].(map[string]any)
	if !reflect.DeepEqual(mapKeys(branch), []string{"name", "source", "tip_sha"}) {
		t.Fatalf("branch keys=%v", mapKeys(branch))
	}
	if branch["name"] != "feat/hub-federation-rebuild" || branch["tip_sha"] != "a3a747a" {
		t.Fatalf("branch=%v", branch)
	}
	if branch["source"] != "report" {
		t.Fatalf("source=%q want report", branch["source"])
	}

	bindings, ok := body["bindings"].([]any)
	if !ok || len(bindings) != 1 {
		t.Fatalf("bindings=%v", body["bindings"])
	}
	binding := bindings[0].(map[string]any)
	wantBindingKeys := []string{"bind_id", "bind_type", "branch_name", "head_sha", "status", "worktree_host"}
	if !reflect.DeepEqual(mapKeys(binding), wantBindingKeys) {
		t.Fatalf("binding keys=%v want %v", mapKeys(binding), wantBindingKeys)
	}
	if binding["bind_type"] != "task" || binding["bind_id"] != "task-1-hub-client-package" {
		t.Fatalf("binding=%v", binding)
	}
	if binding["branch_name"] != "feat/hub-federation-rebuild" || binding["head_sha"] != "a3a747a" {
		t.Fatalf("binding=%v", binding)
	}
	if binding["status"] != "active" {
		t.Fatalf("status=%v want active", binding["status"])
	}

	host, _ := os.Hostname()
	if binding["worktree_host"] != host {
		t.Fatalf("worktree_host=%v want hostname %q", binding["worktree_host"], host)
	}
	// A filesystem path must never reach the wire.
	if len(host) > 0 && binding["worktree_host"] == os.Getenv("HOME") {
		t.Fatal("worktree_host must be a hostname, not a path")
	}
}

// TestReportBranchOmitsEmptyOptionals: no repo_url / no bindings when unset.
func TestReportBranchOmitsEmptyOptionals(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if membershipOK(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	if res := c.ReportBranch(context.Background(), BranchReport{Branch: "main"}); !res.OK() {
		t.Fatalf("%+v", res)
	}
	body := h.lastRequestBodyJSON(t)
	if _, present := body["repo_url"]; present {
		t.Fatalf("empty repo_url must be omitted: %v", body)
	}
	if _, present := body["bindings"]; present {
		t.Fatalf("bindings must be omitted without a bind target: %v", body)
	}
}

// TestReportBranchBindingRequiresBothHalves: a partial bind must not be sent.
func TestReportBranchBindingRequiresBothHalves(t *testing.T) {
	for _, br := range []BranchReport{
		{Branch: "main", BindType: "task"},
		{Branch: "main", BindID: "T1"},
	} {
		isolateHubEnv(t)
		h := newHubServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
			if membershipOK(w, r) {
				return
			}
			_, _ = w.Write([]byte(`{}`))
		})
		c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
		if res := c.ReportBranch(context.Background(), br); !res.OK() {
			t.Fatalf("%+v", res)
		}
		if _, present := h.lastRequestBodyJSON(t)["bindings"]; present {
			t.Fatalf("partial bind %+v must be omitted", br)
		}
	}
}

func TestReportBranchStates(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		handler    func(http.ResponseWriter, *http.Request, int)
		in         BranchReport
		wantStatus Status
		wantCode   int
		wantNote   string
	}{
		{
			name:       "ok",
			env:        jwtEnv("jwt-abc"),
			handler:    func(w http.ResponseWriter, r *http.Request, _ int) { membershipOK(w, r) },
			in:         BranchReport{Branch: "main", HeadSHA: "abc123"},
			wantStatus: StatusOK,
			wantNote:   "hub_branch_report_ok",
		},
		{
			name:       "skipped empty branch",
			env:        jwtEnv("jwt-abc"),
			handler:    func(w http.ResponseWriter, r *http.Request, _ int) { membershipOK(w, r) },
			in:         BranchReport{Branch: "  "},
			wantStatus: StatusSkipped,
			wantNote:   "hub_branch_report_skipped: empty branch",
		},
		{
			name:       "disabled by kill switch",
			env:        map[string]string{"HUB_TOKEN": "jwt", "HUB_BUSINESS_CODE": "z8gw", "HUB_DISABLED": "true"},
			in:         BranchReport{Branch: "main"},
			wantStatus: StatusDisabled,
			wantNote:   "hub_branch_report_disabled: HUB_SYNC/HUB_ENABLED off",
		},
		{
			name:       "skipped without credentials",
			env:        map[string]string{},
			in:         BranchReport{Branch: "main"},
			wantStatus: StatusSkipped,
			wantNote:   "hub_branch_report_skipped: no login token / business_code",
		},
		{
			name: "failed 401",
			env:  jwtEnv("jwt-expired"),
			handler: func(w http.ResponseWriter, r *http.Request, _ int) {
				if membershipOK(w, r) {
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
			},
			in:         BranchReport{Branch: "main"},
			wantStatus: StatusFailed,
			wantCode:   401,
			wantNote:   "hub_branch_report_failed: status 401 forbidden",
		},
		{
			name: "failed 403",
			env:  jwtEnv("jwt-nonmember"),
			handler: func(w http.ResponseWriter, r *http.Request, _ int) {
				if membershipOK(w, r) {
					return
				}
				w.WriteHeader(http.StatusForbidden)
			},
			in:         BranchReport{Branch: "main"},
			wantStatus: StatusFailed,
			wantCode:   403,
			wantNote:   "hub_branch_report_failed: status 403 forbidden",
		},
		{
			name: "failed 502",
			env:  jwtEnv("jwt-abc"),
			handler: func(w http.ResponseWriter, r *http.Request, _ int) {
				if membershipOK(w, r) {
					return
				}
				w.WriteHeader(http.StatusBadGateway)
			},
			in:         BranchReport{Branch: "main"},
			wantStatus: StatusFailed,
			wantCode:   502,
			wantNote:   "hub_branch_report_failed: status 502",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHubEnv(t)
			h := newHubServer(t, tc.handler)
			c := newTestClient(t, h.URL(), tc.env)
			res := c.ReportBranch(context.Background(), tc.in)
			if res.Status != tc.wantStatus || res.Code != tc.wantCode {
				t.Fatalf("got %+v want status=%s code=%d", res, tc.wantStatus, tc.wantCode)
			}
			if res.Op != "branch_report" {
				t.Fatalf("op=%q", res.Op)
			}
			assertNote(t, res, tc.wantNote)

			attempted := tc.wantStatus == StatusOK || tc.wantStatus == StatusFailed
			if attempted {
				if got := h.count("POST", "/v1/hub/repos/z8gw/branches/report"); got != 1 {
					t.Fatalf("expected 1 report POST, got %d", got)
				}
			} else {
				// disabled / skipped / empty-branch must not dial the Hub at all
				assertNoRequests(t, h)
			}
		})
	}
}

func TestReportBranchUnreachableHubIsSoft(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, nil)
	url := h.URL()
	h.srv.Close()

	c := newTestClient(t, url, jwtEnv("jwt-abc"))
	res := c.ReportBranch(context.Background(), BranchReport{Branch: "main"})
	if res.Status != StatusFailed || res.Code != 0 || res.Message == "" {
		t.Fatalf("unreachable: %+v", res)
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
