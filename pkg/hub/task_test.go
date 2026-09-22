package hub

import (
	"context"
	"net/http"
	"reflect"
	"sort"
	"testing"
)

// membershipOK answers the JWT membership probe so write-path tests exercise
// only the write itself.
func membershipOK(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == "GET" && r.URL.Path == "/v1/hub/me/businesses" {
		_, _ = w.Write([]byte(`{"data":[{"code":"z8gw","role":"member"}]}`))
		return true
	}
	return false
}

// TestSyncTaskBodyFieldsExact pins the wire contract of §3.3: the five scalar
// fields plus the three array/optional ones, spelled exactly as the Hub expects.
func TestSyncTaskBodyFieldsExact(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if membershipOK(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))

	res := c.SyncTask(context.Background(), TaskProjection{
		TaskID:         "task-1-hub-client-package",
		Title:          "rebuild hub client",
		Status:         "executing",
		AssignedWorker: "agentflow-dev",
		DependsOn:      []string{"task-0"},
		OutputFiles:    []string{"pkg/hub/client.go"},
		Branch:         "feat/hub-federation-rebuild",
		HeadSHA:        "a3a747a",
	})
	if !res.OK() {
		t.Fatalf("sync should succeed: %+v", res)
	}
	assertNote(t, res, "hub_task_sync_ok")

	req := h.lastRequest(t)
	if req.Method != "POST" || req.Path != "/v1/hub/dag/z8gw" {
		t.Fatalf("wrong route: %s %s", req.Method, req.Path)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type=%q", got)
	}
	body := h.lastRequestBodyJSON(t)

	gotKeys := make([]string, 0, len(body))
	for k := range body {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	wantKeys := []string{
		"assigned_worker", "branch", "depends_on", "head_sha",
		"output_files", "status", "task_id", "title",
	}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("body keys=%v want %v", gotKeys, wantKeys)
	}
	wantVals := map[string]any{
		"task_id":         "task-1-hub-client-package",
		"title":           "rebuild hub client",
		"status":          "executing",
		"assigned_worker": "agentflow-dev",
		"branch":          "feat/hub-federation-rebuild",
		"head_sha":        "a3a747a",
	}
	for k, want := range wantVals {
		if body[k] != want {
			t.Fatalf("body[%s]=%v want %v", k, body[k], want)
		}
	}
	if !reflect.DeepEqual(body["depends_on"], []any{"task-0"}) {
		t.Fatalf("depends_on=%v", body["depends_on"])
	}
	if !reflect.DeepEqual(body["output_files"], []any{"pkg/hub/client.go"}) {
		t.Fatalf("output_files=%v", body["output_files"])
	}
}

// TestSyncTaskNilSlicesMarshalAsJSONNull documents the current wire behaviour for
// an empty dependency list: the key is always present.
func TestSyncTaskNilSlicesMarshalAsJSONNull(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if membershipOK(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	c := newTestClient(t, h.URL(), jwtEnv("jwt-abc"))
	if res := c.SyncTask(context.Background(), TaskProjection{TaskID: "T1"}); !res.OK() {
		t.Fatalf("%+v", res)
	}
	body := h.lastRequestBodyJSON(t)
	if v, ok := body["depends_on"]; !ok || v != nil {
		t.Fatalf("depends_on must be present (null), got %v (present=%v)", v, ok)
	}
	if v, ok := body["output_files"]; !ok || v != nil {
		t.Fatalf("output_files must be present (null), got %v (present=%v)", v, ok)
	}
}

// TestSyncTaskStates covers every terminal branch of the soft state machine.
func TestSyncTaskStates(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		handler    func(http.ResponseWriter, *http.Request, int)
		proj       TaskProjection
		wantStatus Status
		wantCode   int
		wantNote   string
		wantWrite  bool
	}{
		{
			name:       "ok",
			env:        jwtEnv("jwt-abc"),
			handler:    func(w http.ResponseWriter, r *http.Request, _ int) { membershipOK(w, r) },
			proj:       TaskProjection{TaskID: "T1", Status: "executing"},
			wantStatus: StatusOK,
			wantNote:   "hub_task_sync_ok",
			wantWrite:  true,
		},
		{
			name:       "disabled by kill switch",
			env:        map[string]string{"HUB_TOKEN": "jwt", "HUB_BUSINESS_CODE": "z8gw", "HUB_DISABLED": "1"},
			handler:    nil,
			proj:       TaskProjection{TaskID: "T1"},
			wantStatus: StatusDisabled,
			wantNote:   "hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off",
		},
		{
			name:       "disabled by HUB_SYNC",
			env:        map[string]string{"HUB_TOKEN": "jwt", "HUB_BUSINESS_CODE": "z8gw", "HUB_SYNC": "0"},
			proj:       TaskProjection{TaskID: "T1"},
			wantStatus: StatusDisabled,
			wantNote:   "hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off",
		},
		{
			name:       "disabled by HUB_ENABLED=false",
			env:        map[string]string{"HUB_TOKEN": "jwt", "HUB_BUSINESS_CODE": "z8gw", "HUB_ENABLED": "false"},
			proj:       TaskProjection{TaskID: "T1"},
			wantStatus: StatusDisabled,
			wantNote:   "hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off",
		},
		{
			name:       "skipped without credentials",
			env:        map[string]string{},
			proj:       TaskProjection{TaskID: "T1"},
			wantStatus: StatusSkipped,
			wantNote:   "hub_task_sync_skipped: no login token / business_code",
		},
		{
			name:       "skipped without business code",
			env:        map[string]string{"HUB_TOKEN": "jwt"},
			proj:       TaskProjection{TaskID: "T1"},
			wantStatus: StatusSkipped,
			wantNote:   "hub_task_sync_skipped: no login token / business_code",
		},
		{
			name:       "skipped on empty task_id",
			env:        jwtEnv("jwt-abc"),
			handler:    func(w http.ResponseWriter, r *http.Request, _ int) { membershipOK(w, r) },
			proj:       TaskProjection{TaskID: "   "},
			wantStatus: StatusSkipped,
			wantNote:   "hub_task_sync_skipped: empty task_id",
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
			proj:       TaskProjection{TaskID: "T1"},
			wantStatus: StatusFailed,
			wantCode:   401,
			wantNote:   "hub_task_sync_failed: status 401 forbidden",
			wantWrite:  true,
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
			proj:       TaskProjection{TaskID: "T1"},
			wantStatus: StatusFailed,
			wantCode:   403,
			wantNote:   "hub_task_sync_failed: status 403 forbidden",
			wantWrite:  true,
		},
		{
			name: "failed 500",
			env:  jwtEnv("jwt-abc"),
			handler: func(w http.ResponseWriter, r *http.Request, _ int) {
				if membershipOK(w, r) {
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
			},
			proj:       TaskProjection{TaskID: "T1"},
			wantStatus: StatusFailed,
			wantCode:   500,
			wantNote:   "hub_task_sync_failed: status 500",
			wantWrite:  true,
		},
		{
			name: "failed 503",
			env:  jwtEnv("jwt-abc"),
			handler: func(w http.ResponseWriter, r *http.Request, _ int) {
				if membershipOK(w, r) {
					return
				}
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			proj:       TaskProjection{TaskID: "T1"},
			wantStatus: StatusFailed,
			wantCode:   503,
			wantNote:   "hub_task_sync_failed: status 503",
			wantWrite:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHubEnv(t)
			h := newHubServer(t, tc.handler)
			c := newTestClient(t, h.URL(), tc.env)
			res := c.SyncTask(context.Background(), tc.proj)

			if res.Status != tc.wantStatus {
				t.Fatalf("status=%s want %s (%+v)", res.Status, tc.wantStatus, res)
			}
			if res.Code != tc.wantCode {
				t.Fatalf("code=%d want %d (%+v)", res.Code, tc.wantCode, res)
			}
			if res.Op != "task_sync" {
				t.Fatalf("op=%q want task_sync", res.Op)
			}
			assertNote(t, res, tc.wantNote)

			writes := h.count("POST", "/v1/hub/dag/z8gw")
			if tc.wantWrite {
				// One membership probe + one write.
				if writes != 1 {
					t.Fatalf("expected exactly 1 write, got %d", writes)
				}
				if probes := h.count("GET", "/v1/hub/me/businesses"); probes != 1 {
					t.Fatalf("expected exactly 1 membership probe, got %d", probes)
				}
			} else {
				// Guard/skip branches must never dial the Hub.
				assertNoRequests(t, h)
			}
		})
	}
}

// TestSyncTaskUnreachableHubIsSoft covers an unreachable host on the write path.
func TestSyncTaskUnreachableHubIsSoft(t *testing.T) {
	isolateHubEnv(t)
	h := newHubServer(t, nil)
	url := h.URL()
	h.srv.Close()

	c := newTestClient(t, url, jwtEnv("jwt-abc"))
	res := c.SyncTask(context.Background(), TaskProjection{TaskID: "T1"})
	if res.Status != StatusFailed || res.Code != 0 {
		t.Fatalf("unreachable: %+v", res)
	}
	if res.Message == "" {
		t.Fatal("transport failure must carry a message")
	}
}
