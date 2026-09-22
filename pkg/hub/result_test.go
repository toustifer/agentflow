package hub

import "testing"

// TestResultNote pins the exact note strings. task-2 backfills these into MCP
// tool payloads, so a change here is a contract change.
func TestResultNote(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		r    Result
		want string
	}{
		{
			name: "ok bare",
			r:    Result{Status: StatusOK, Op: "task_sync"},
			want: "hub_task_sync_ok",
		},
		{
			name: "ok with message",
			r:    Result{Status: StatusOK, Op: "list_teams", Message: "3 teams"},
			want: "hub_list_teams_ok: 3 teams",
		},
		{
			name: "skipped bare",
			r:    Result{Status: StatusSkipped, Op: "task_sync"},
			want: "hub_task_sync_skipped",
		},
		{
			name: "skipped with message",
			r:    Result{Status: StatusSkipped, Op: "task_sync", Message: "empty task_id"},
			want: "hub_task_sync_skipped: empty task_id",
		},
		{
			name: "disabled with message",
			r:    Result{Status: StatusDisabled, Op: "task_sync", Message: "HUB_SYNC/HUB_ENABLED off"},
			want: "hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off",
		},
		{
			name: "failed status only",
			r:    Result{Status: StatusFailed, Op: "report", Code: 401},
			want: "hub_report_failed: status 401",
		},
		{
			name: "failed with transport message",
			r:    Result{Status: StatusFailed, Op: "task_sync", Message: "dial tcp: refused"},
			want: "hub_task_sync_failed: dial tcp: refused",
		},
		{
			name: "failed status and message",
			r:    Result{Status: StatusFailed, Op: "branch_report", Code: 403, Message: "forbidden"},
			want: "hub_branch_report_failed: status 403 forbidden",
		},
		{
			name: "zero value falls back to hub op and failed status",
			r:    Result{},
			want: "hub_hub_failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertNote(t, tc.r, tc.want)
		})
	}
}

func TestResultOKAndSoftSkip(t *testing.T) {
	t.Parallel()
	ok := Result{Status: StatusOK}
	if !ok.OK() || ok.SoftSkip() {
		t.Fatalf("ok: OK=%v SoftSkip=%v", ok.OK(), ok.SoftSkip())
	}
	for _, s := range []Status{StatusSkipped, StatusDisabled} {
		r := Result{Status: s}
		if r.OK() {
			t.Fatalf("%s must not be OK()", s)
		}
		if !r.SoftSkip() {
			t.Fatalf("%s must be SoftSkip()", s)
		}
	}
	failed := Result{Status: StatusFailed, Code: 500}
	if failed.OK() || failed.SoftSkip() {
		t.Fatalf("failed: OK=%v SoftSkip=%v", failed.OK(), failed.SoftSkip())
	}
}

// TestStatusConstantsAreStable pins the wire-visible enum values.
func TestStatusConstantsAreStable(t *testing.T) {
	t.Parallel()
	for want, got := range map[string]Status{
		"ok": StatusOK, "skipped": StatusSkipped,
		"disabled": StatusDisabled, "failed": StatusFailed,
	} {
		if string(got) != want {
			t.Fatalf("Status constant drifted: got %q want %q", got, want)
		}
	}
}
