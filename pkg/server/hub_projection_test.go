package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/toustifer/agentflow/pkg/engine"
	"github.com/toustifer/agentflow/pkg/hub"
)

// ---------------------------------------------------------------------------
// Test rig: an httptest Hub that records every request it receives.
//
// No case here ever reaches the production Hub. The base URL is either this
// recorder or a just-closed port, and ~/.agent-hub/config.json is isolated to a
// temp dir so a developer's real JWT cannot influence an outcome.
// ---------------------------------------------------------------------------

type hubCall struct {
	method string
	path   string
	body   []byte
}

type fakeHub struct {
	server *httptest.Server

	mu    sync.Mutex
	calls []hubCall

	status   int           // 0 => 200 on every route
	writeLag time.Duration // delays write routes only
}

// newFakeHub starts a recording Hub. status != 0 makes every route fail with
// that status; writeLag delays only the write routes, so a hang can be measured
// without also stalling the membership probe.
func newFakeHub(t *testing.T, status int, writeLag time.Duration) *fakeHub {
	t.Helper()
	f := &fakeHub{status: status, writeLag: writeLag}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeHub) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, hubCall{method: r.Method, path: r.URL.Path, body: body})
	status, lag := f.status, f.writeLag
	f.mu.Unlock()

	if r.Method == http.MethodGet && r.URL.Path == "/v1/hub/me/businesses" {
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"code":"z8gw","role":"owner"}]`)
		return
	}
	if lag > 0 {
		time.Sleep(lag)
	}
	if status != 0 {
		w.WriteHeader(status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"ok":true}`)
}

// setFault injects a fault mid-test, so a case can prepare a healthy task and
// then measure exactly one trigger under failure.
func (f *fakeHub) setFault(status int, writeLag time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = status
	f.writeLag = writeLag
}

func (f *fakeHub) URL() string { return f.server.URL }

func (f *fakeHub) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeHub) snapshot() []hubCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]hubCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// postsTo returns the raw bodies of every POST whose path ends with suffix.
func (f *fakeHub) postsTo(suffix string) [][]byte {
	var out [][]byte
	for _, c := range f.snapshot() {
		if c.method == http.MethodPost && strings.HasSuffix(c.path, suffix) {
			out = append(out, c.body)
		}
	}
	return out
}

func (f *fakeHub) paths() []string {
	out := make([]string, 0)
	for _, c := range f.snapshot() {
		out = append(out, c.method+" "+c.path)
	}
	return out
}

// wireTask mirrors pkg/hub's taskProjectionBody so a test asserts the actual
// field names on the wire rather than an internal struct.
type wireTask struct {
	TaskID         string   `json:"task_id"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	AssignedWorker string   `json:"assigned_worker"`
	DependsOn      []string `json:"depends_on"`
	OutputFiles    []string `json:"output_files"`
	Branch         string   `json:"branch"`
	HeadSHA        string   `json:"head_sha"`
}

type wireBranchReport struct {
	Reporter string `json:"reporter"`
	RepoURL  string `json:"repo_url"`
	Branches []struct {
		Name   string `json:"name"`
		TipSHA string `json:"tip_sha"`
		Source string `json:"source"`
	} `json:"branches"`
	Bindings []struct {
		BindType     string `json:"bind_type"`
		BindID       string `json:"bind_id"`
		BranchName   string `json:"branch_name"`
		HeadSHA      string `json:"head_sha"`
		WorktreeHost string `json:"worktree_host"`
		Status       string `json:"status"`
	} `json:"bindings"`
}

func decodeWireTask(t *testing.T, raw []byte) wireTask {
	t.Helper()
	var v wireTask
	require.NoError(t, json.Unmarshal(raw, &v), "body=%s", string(raw))
	return v
}

// isolateHubEnv makes Hub configuration fully deterministic for one test:
// every HUB_* knob is blanked and the user home is redirected to a temp dir, so
// ~/.agent-hub/config.json (JWT-only, and present on this dev machine) neither
// supplies a credential nor a team code.
//
// t.Setenv forbids t.Parallel, which is why none of the projection tests call it.
func isolateHubEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	for _, k := range []string{
		"HUB_BASE_URL", "HUB_TOKEN", "HUB_JWT", "HUB_API_KEY",
		"HUB_BUSINESS_CODE", "HUB_BUSINESS", "HUB_SYNC", "HUB_ENABLED", "HUB_DISABLED",
	} {
		t.Setenv(k, "")
	}
	return home
}

// enableHubForTest points the projection at the recorder with credentials that
// are valid enough to be attempted (JWT + team code).
func enableHubForTest(t *testing.T, f *fakeHub) {
	t.Helper()
	t.Setenv("HUB_BASE_URL", f.URL())
	t.Setenv("HUB_TOKEN", "jwt-projection-test")
	t.Setenv("HUB_BUSINESS_CODE", "z8gw")
}

func hubNoteOf(t *testing.T, payload map[string]any) string {
	t.Helper()
	note, ok := payload[hubNoteKey].(string)
	require.True(t, ok, "%s missing from payload: %v", hubNoteKey, payload)
	return note
}

// startMetadata mirrors what a Leader sends with a real start: the launch ticket
// task_prepare_start issued plus the runtime binding the worker reported.
func startMetadata(t *testing.T, prepared map[string]any) map[string]any {
	t.Helper()
	ticket, _ := prepared["launch_ticket"].(string)
	require.NotEmpty(t, ticket)
	return map[string]any{
		"actor":               "leader",
		"launch.ticket":       ticket,
		"worker_agent_id":     "agent-projection-test",
		"runtime.provider":    "claude_code",
		"runtime.status":      "started",
		"runtime.launched_at": time.Now().UTC().Format(time.RFC3339),
	}
}

// closedPortURL returns a base URL nothing listens on, for the connection
// refused fault case.
func closedPortURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return "http://" + addr
}

// ---------------------------------------------------------------------------
// AC-1 — the four lifecycle triggers project softly, note backfilled
// ---------------------------------------------------------------------------

func TestLifecycleProjectionCoversAllFourTriggers(t *testing.T) {
	isolateHubEnv(t)
	f := newFakeHub(t, 0, 0)
	srv := newTestServer(t)
	enableHubForTest(t, f)
	ctx := context.Background()

	// (1) task_create ---------------------------------------------------------
	created, err := srv.Handle(ctx, "task_create", map[string]any{
		"namespace_id":    "ns-1",
		"task_id":         "T-hub-create",
		"title":           "projected on create",
		"assigned_worker": "worker-b",
		"depends_on":      []any{"T-hub-dep"},
		"output_files":    []any{"out.txt"},
	})
	require.NoError(t, err)
	require.Equal(t, "hub_task_sync_ok", hubNoteOf(t, created))
	require.Equal(t, "assigned", created["state"])

	syncs := f.postsTo("/v1/hub/dag/z8gw")
	require.Len(t, syncs, 1)
	body := decodeWireTask(t, syncs[0])
	require.Equal(t, "T-hub-create", body.TaskID)
	require.Equal(t, "projected on create", body.Title)
	require.Equal(t, "assigned", body.Status)
	require.Equal(t, "worker-b", body.AssignedWorker)
	require.Equal(t, []string{"T-hub-dep"}, body.DependsOn)
	require.Equal(t, []string{"out.txt"}, body.OutputFiles)
	require.Empty(t, body.Branch, "a task with no worktree carries no branch")
	require.Empty(t, body.HeadSHA)

	// (2) task_prepare_start --------------------------------------------------
	createDagTaskForStart(t, srv, "T-hub-start")
	prepared, err := srv.Handle(ctx, "task_prepare_start", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-hub-start",
	})
	require.NoError(t, err)
	require.Equal(t, "hub_task_sync_ok", hubNoteOf(t, prepared))
	require.Equal(t, "hub_branch_report_ok", prepared[hubBranchNoteKey])

	syncs = f.postsTo("/v1/hub/dag/z8gw")
	require.Len(t, syncs, 2)
	preparedBody := decodeWireTask(t, syncs[1])
	require.Equal(t, "T-hub-start", preparedBody.TaskID)
	require.Equal(t, "assigned", preparedBody.Status)
	require.Equal(t, "feat/test", preparedBody.Branch)
	require.Len(t, preparedBody.HeadSHA, 40, "worktree head sha must be carried")
	require.Equal(t, strings.ToLower(preparedBody.HeadSHA), preparedBody.HeadSHA)

	// Branch report carries the task binding, the hostname — and no local path.
	branchPosts := f.postsTo("/v1/hub/repos/z8gw/branches/report")
	require.Len(t, branchPosts, 1)
	var report wireBranchReport
	require.NoError(t, json.Unmarshal(branchPosts[0], &report))
	require.Equal(t, "agentflow", report.Reporter)
	require.Len(t, report.Branches, 1)
	require.Equal(t, "feat/test", report.Branches[0].Name)
	require.Equal(t, preparedBody.HeadSHA, report.Branches[0].TipSHA)
	require.Equal(t, "report", report.Branches[0].Source)
	require.Len(t, report.Bindings, 1)
	require.Equal(t, "task", report.Bindings[0].BindType)
	require.Equal(t, "T-hub-start", report.Bindings[0].BindID)
	require.Equal(t, "feat/test", report.Bindings[0].BranchName)
	require.Equal(t, report.Branches[0].TipSHA, report.Bindings[0].HeadSHA)
	require.Equal(t, "active", report.Bindings[0].Status)
	host, err := os.Hostname()
	require.NoError(t, err)
	require.Equal(t, host, report.Bindings[0].WorktreeHost)

	// Red line: never a local absolute path on the wire.
	meta, ok := prepared["metadata"].(map[string]any)
	require.True(t, ok)
	worktreePath, _ := meta["git.worktree_path"].(string)
	require.NotEmpty(t, worktreePath)
	raw := string(branchPosts[0])
	require.NotContains(t, raw, worktreePath)
	require.NotContains(t, raw, filepath.ToSlash(worktreePath))

	// (3) task_transition -----------------------------------------------------
	transitioned, err := srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "ns-1",
		"task_id":      "T-hub-start",
		"transition":   "start",
		"actor_role":   "leader",
		"metadata":     startMetadata(t, prepared),
	})
	require.NoError(t, err)
	require.Equal(t, "executing", transitioned["state"])
	require.Equal(t, "hub_task_sync_ok", hubNoteOf(t, transitioned))

	syncs = f.postsTo("/v1/hub/dag/z8gw")
	require.Len(t, syncs, 3)
	require.Equal(t, "executing", decodeWireTask(t, syncs[2]).Status)

	// (4) task_create_batch ---------------------------------------------------
	batch, err := srv.Handle(ctx, "task_create_batch", map[string]any{
		"namespace_id": "ns-1",
		"tasks": []any{
			map[string]any{"task_id": "T-hub-b1", "title": "batch one", "assigned_worker": "worker-b"},
			map[string]any{"task_id": "T-hub-b2", "title": "batch two", "assigned_worker": "worker-b"},
		},
	})
	require.NoError(t, err)
	items, ok := batch["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "hub_task_sync_ok", hubNoteOf(t, item))
	}

	syncs = f.postsTo("/v1/hub/dag/z8gw")
	require.Len(t, syncs, 5)
	require.Equal(t, "T-hub-b1", decodeWireTask(t, syncs[3]).TaskID)
	require.Equal(t, "T-hub-b2", decodeWireTask(t, syncs[4]).TaskID)
}

// TestProjectionUsesNamespaceBoundTeamCode pins the LoadForNamespace wiring:
// with no HUB_BUSINESS_CODE in the environment the code must come from
// namespace metadata (the only product truth — ~/.agent-hub/config.json never
// supplies one).
func TestProjectionUsesNamespaceBoundTeamCode(t *testing.T) {
	isolateHubEnv(t)
	f := newFakeHub(t, 0, 0)
	srv := newTestServer(t)
	t.Setenv("HUB_BASE_URL", f.URL())
	t.Setenv("HUB_TOKEN", "jwt-projection-test")
	ctx := context.Background()

	_, err := srv.engine.UpdateNamespace(ctx, engine.UpdateNamespaceRequest{
		ID:       "ns-1",
		Metadata: map[string]string{hub.MetaBusinessCode: "z8gw"},
	})
	require.NoError(t, err)

	created, err := srv.Handle(ctx, "task_create", map[string]any{
		"namespace_id":    "ns-1",
		"task_id":         "T-hub-nsbind",
		"title":           "namespace bound team",
		"assigned_worker": "worker-b",
	})
	require.NoError(t, err)
	require.Equal(t, "hub_task_sync_ok", hubNoteOf(t, created))
	require.Contains(t, f.paths(), "POST /v1/hub/dag/z8gw")
}

// TestTransitionSubmitCarriesReviewedTip proves the submit leg carries
// git.branch plus the tip the reviewer will see (review.commit), not the base
// sha recorded when the task started.
func TestTransitionSubmitCarriesReviewedTip(t *testing.T) {
	isolateHubEnv(t)
	f := newFakeHub(t, 0, 0)
	srv := newTestServer(t)
	enableHubForTest(t, f)
	ctx := context.Background()

	createDagTaskForStart(t, srv, "T-hub-submit")
	prepared, err := srv.Handle(ctx, "task_prepare_start", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-hub-submit",
	})
	require.NoError(t, err)
	baseSyncs := f.postsTo("/v1/hub/dag/z8gw")
	require.Len(t, baseSyncs, 1)
	baseTip := decodeWireTask(t, baseSyncs[0]).HeadSHA

	_, err = srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-hub-submit",
		"transition": "start", "actor_role": "leader",
		"metadata": startMetadata(t, prepared),
	})
	require.NoError(t, err)

	// Worker commits in the worktree, then submits.
	task, err := srv.engine.GetTask(ctx, "ns-1", "T-hub-submit")
	require.NoError(t, err)
	worktree := task.Metadata["git.worktree_path"]
	require.NotEmpty(t, worktree)
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "work.txt"), []byte("done"), 0o644))
	runGitTest(t, worktree, "add", ".")
	runGitTest(t, worktree, "commit", "-m", "implement T-hub-submit")

	today := time.Now().UTC().Format("2006-01-02")
	_, err = srv.Handle(ctx, "worker_diary_write", map[string]any{
		"namespace_id": "ns-1", "worker_id": "worker-b", "date": today,
		"content": "finished T-hub-submit", "task_id": "T-hub-submit",
	})
	require.NoError(t, err)

	submitted, err := srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-hub-submit",
		"transition": "submit", "actor_role": "worker",
	})
	require.NoError(t, err)
	require.Equal(t, "review_pending", submitted["state"])
	require.Equal(t, "hub_task_sync_ok", hubNoteOf(t, submitted))

	task, err = srv.engine.GetTask(ctx, "ns-1", "T-hub-submit")
	require.NoError(t, err)
	reviewCommit := task.Metadata["review.commit"]
	require.NotEmpty(t, reviewCommit)
	require.NotEqual(t, baseTip, reviewCommit, "the worker commit must move the tip")

	syncs := f.postsTo("/v1/hub/dag/z8gw")
	require.Len(t, syncs, 3)
	submitBody := decodeWireTask(t, syncs[2])
	require.Equal(t, "review_pending", submitBody.Status)
	require.Equal(t, "feat/test", submitBody.Branch)
	require.Equal(t, reviewCommit, submitBody.HeadSHA)
}

// TestProjectionStaysOffWhenPartialGitMetadata covers the other transition
// return path: a task without git metadata still projects, just with an empty
// branch/tip rather than a fabricated one.
func TestProjectionStaysOffWhenPartialGitMetadata(t *testing.T) {
	isolateHubEnv(t)
	f := newFakeHub(t, 0, 0)
	srv := newTestServer(t)
	enableHubForTest(t, f)
	ctx := context.Background()

	_, err := srv.engine.CreateTask(ctx, engine.CreateTaskRequest{
		NamespaceID: "ns-1", ID: "T-hub-nogit", Title: "no git", AssignedWorker: "worker-b",
	})
	require.NoError(t, err)

	result, err := srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-hub-nogit",
		"transition": "reassign", "actor_role": "leader",
		"metadata": map[string]any{"assigned_worker": "worker-b"},
	})
	require.NoError(t, err)
	require.Equal(t, "hub_task_sync_ok", hubNoteOf(t, result))

	syncs := f.postsTo("/v1/hub/dag/z8gw")
	require.Len(t, syncs, 1)
	body := decodeWireTask(t, syncs[0])
	require.Empty(t, body.Branch)
	require.Empty(t, body.HeadSHA)
}

// ---------------------------------------------------------------------------
// AC-2 — soft-fail: 500 / timeout / connection refused / 401
// ---------------------------------------------------------------------------

func TestProjectionSoftFailKeepsLifecycleMoving(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		writeLag   time.Duration
		refused    bool
		wantNote   string
		wantDetail string
		attempted  bool
	}{
		{name: "status_500", status: http.StatusInternalServerError, wantNote: "hub_task_sync_failed: status 500", attempted: true},
		{name: "status_401", status: http.StatusUnauthorized, wantNote: "hub_task_sync_failed: status 401 forbidden", attempted: true},
		// writeLag must exceed pkg/hub's 5s request timeout.
		{name: "timeout", writeLag: 6 * time.Second, wantNote: "hub_task_sync_failed", wantDetail: "context deadline exceeded", attempted: true},
		{name: "connection_refused", refused: true, wantNote: "hub_task_sync_failed", wantDetail: "dial tcp", attempted: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolateHubEnv(t)
			srv := newTestServer(t)
			ctx := context.Background()

			f := newFakeHub(t, 0, 0)
			enableHubForTest(t, f)

			// Prepare against a healthy Hub so the probe/worktree path is not
			// what the case measures, then inject the fault for one trigger.
			createDagTaskForStart(t, srv, "T-fault")
			prepared, err := srv.Handle(ctx, "task_prepare_start", map[string]any{
				"namespace_id": "ns-1", "task_id": "T-fault",
			})
			require.NoError(t, err)

			if tc.refused {
				t.Setenv("HUB_BASE_URL", closedPortURL(t))
			} else {
				f.setFault(tc.status, tc.writeLag)
			}
			before := f.count()

			// (a) the tool call still succeeds...
			result, err := srv.Handle(ctx, "task_transition", map[string]any{
				"namespace_id": "ns-1", "task_id": "T-fault",
				"transition": "start", "actor_role": "leader",
				"metadata": startMetadata(t, prepared),
			})
			require.NoError(t, err, "a Hub fault must never surface as a tool error")

			// (b) ...the local lifecycle advanced and is durable...
			require.Equal(t, "executing", result["state"])
			task, err := srv.engine.GetTask(ctx, "ns-1", "T-fault")
			require.NoError(t, err)
			require.Equal(t, engine.TaskExecuting, task.State)

			// (c) ...and the note records what the mirror did.
			note := hubNoteOf(t, result)
			require.Contains(t, note, tc.wantNote)
			if tc.wantDetail != "" {
				require.Contains(t, note, tc.wantDetail, "note must name the concrete fault")
			}

			if tc.attempted {
				require.Greater(t, f.count(), before, "the fault must have been attempted, not skipped")
			} else {
				require.Equal(t, before, f.count(), "a refused connection cannot reach the recorder")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AC-3 / AC-4 — default off, and the kill switch dials nothing
// ---------------------------------------------------------------------------

func TestProjectionDefaultOffDialsNothing(t *testing.T) {
	isolateHubEnv(t)
	f := newFakeHub(t, 0, 0)
	srv := newTestServer(t)
	// A live, reachable recorder: if anything dialled, it would show here.
	t.Setenv("HUB_BASE_URL", f.URL())
	ctx := context.Background()

	// (1) task_create
	created, err := srv.Handle(ctx, "task_create", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-off-create", "title": "off", "assigned_worker": "worker-b",
	})
	require.NoError(t, err)
	require.Equal(t, "hub_task_sync_skipped: no login token / business_code", hubNoteOf(t, created))

	// (2) task_prepare_start (+ branch report)
	createDagTaskForStart(t, srv, "T-off-start")
	prepared, err := srv.Handle(ctx, "task_prepare_start", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-off-start",
	})
	require.NoError(t, err)
	require.Equal(t, "hub_task_sync_skipped: no login token / business_code", hubNoteOf(t, prepared))
	require.Equal(t, "hub_branch_report_skipped: no login token / business_code", prepared[hubBranchNoteKey])

	// (3) task_transition
	transitioned, err := srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-off-start",
		"transition": "start", "actor_role": "leader",
		"metadata": startMetadata(t, prepared),
	})
	require.NoError(t, err)
	require.Equal(t, "executing", transitioned["state"])
	require.Equal(t, "hub_task_sync_skipped: no login token / business_code", hubNoteOf(t, transitioned))

	// (4) task_create_batch
	batch, err := srv.Handle(ctx, "task_create_batch", map[string]any{
		"namespace_id": "ns-1",
		"tasks": []any{
			map[string]any{"task_id": "T-off-b1", "title": "b1", "assigned_worker": "worker-b"},
			map[string]any{"task_id": "T-off-b2", "title": "b2", "assigned_worker": "worker-b"},
		},
	})
	require.NoError(t, err)
	items, ok := batch["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, items, 2)
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "hub_task_sync_skipped: no login token / business_code", hubNoteOf(t, item))
	}

	require.Zero(t, f.count(), "no Hub config must mean zero outbound requests")
}

func TestProjectionKillSwitchDialsNothing(t *testing.T) {
	isolateHubEnv(t)
	f := newFakeHub(t, 0, 0)
	srv := newTestServer(t)
	enableHubForTest(t, f)
	t.Setenv("HUB_DISABLED", "1")
	ctx := context.Background()

	created, err := srv.Handle(ctx, "task_create", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-kill-create", "title": "killed", "assigned_worker": "worker-b",
	})
	require.NoError(t, err)
	require.Equal(t, "hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off", hubNoteOf(t, created))

	createDagTaskForStart(t, srv, "T-kill-start")
	prepared, err := srv.Handle(ctx, "task_prepare_start", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-kill-start",
	})
	require.NoError(t, err)
	require.Equal(t, "hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off", hubNoteOf(t, prepared))
	require.Equal(t, "hub_branch_report_disabled: HUB_SYNC/HUB_ENABLED off", prepared[hubBranchNoteKey])

	transitioned, err := srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-kill-start",
		"transition": "start", "actor_role": "leader",
		"metadata": startMetadata(t, prepared),
	})
	require.NoError(t, err)
	require.Equal(t, "executing", transitioned["state"])
	require.Equal(t, "hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off", hubNoteOf(t, transitioned))

	batch, err := srv.Handle(ctx, "task_create_batch", map[string]any{
		"namespace_id": "ns-1",
		"tasks":        []any{map[string]any{"task_id": "T-kill-b1", "title": "b1", "assigned_worker": "worker-b"}},
	})
	require.NoError(t, err)
	items, ok := batch["tasks"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Equal(t, "hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off", hubNoteOf(t, items[0].(map[string]any)))

	require.Zero(t, f.count(), "HUB_DISABLED=1 must mean zero outbound requests")
}

// ---------------------------------------------------------------------------
// Unit coverage for the projection helpers themselves
// ---------------------------------------------------------------------------

func TestTaskGitRefsPrefersReviewTipOnlyWhenAsked(t *testing.T) {
	task := &engine.Task{Metadata: map[string]string{
		"git.branch":    "feat/x",
		"git.head_sha":  "base-tip",
		"review.commit": "reviewed-tip",
		"git.status":    "active",
		"other.key":     "ignored",
	}}

	branch, head := taskGitRefs(task, false)
	require.Equal(t, "feat/x", branch)
	require.Equal(t, "base-tip", head)

	branch, head = taskGitRefs(task, true)
	require.Equal(t, "feat/x", branch)
	require.Equal(t, "reviewed-tip", head)

	// No review commit yet: the start tip is the only truth.
	task.Metadata["review.commit"] = ""
	_, head = taskGitRefs(task, true)
	require.Equal(t, "base-tip", head)

	branch, head = taskGitRefs(&engine.Task{}, true)
	require.Empty(t, branch)
	require.Empty(t, head)
}

func TestPreferReviewTipByTransitionVerb(t *testing.T) {
	require.False(t, preferReviewTip(map[string]any{"transition": "start"}))
	require.False(t, preferReviewTip(map[string]any{"transition": "resume"}))
	require.True(t, preferReviewTip(map[string]any{"transition": "submit"}))
	require.True(t, preferReviewTip(map[string]any{"transition": "pass"}))
	require.True(t, preferReviewTip(map[string]any{}))
}

// TestProjectionNeverSendsLocalPathsOrBodies guards the "never on the wire"
// red line at the payload level: the projection carries whitelisted fields only.
func TestProjectionNeverSendsLocalPathsOrBodies(t *testing.T) {
	isolateHubEnv(t)
	f := newFakeHub(t, 0, 0)
	srv := newTestServer(t)
	enableHubForTest(t, f)
	ctx := context.Background()

	secret := filepath.Join(t.TempDir(), "secret.txt")
	_, err := srv.engine.CreateTask(ctx, engine.CreateTaskRequest{
		NamespaceID:    "ns-1",
		ID:             "T-hub-privacy",
		Title:          "privacy",
		AssignedWorker: "worker-b",
		Description:    "PROMPT BODY " + secret,
		Metadata: map[string]string{
			"git.worktree_path": secret,
			"review.diff":       "diff --git a/x b/x\n+secret line",
		},
	})
	require.NoError(t, err)

	_, err = srv.Handle(ctx, "task_transition", map[string]any{
		"namespace_id": "ns-1", "task_id": "T-hub-privacy",
		"transition": "reassign", "actor_role": "leader",
		"metadata": map[string]any{"assigned_worker": "worker-b"},
	})
	require.NoError(t, err)

	syncs := f.postsTo("/v1/hub/dag/z8gw")
	require.Len(t, syncs, 1)
	raw := string(syncs[0])
	require.NotContains(t, raw, secret)
	require.NotContains(t, raw, "PROMPT BODY")
	require.NotContains(t, raw, "diff --git")
	require.NotContains(t, raw, "review.diff")
	require.NotContains(t, raw, "description")
}
