package hub

import "testing"

func TestResultNoteShapes(t *testing.T) {
	cases := []struct {
		r    Result
		want string
	}{
		{Result{Status: StatusOK, Op: "branch_report"}, "hub_branch_report_ok"},
		{Result{Status: StatusSkipped, Op: "branch_report", Message: "empty branch"}, "hub_branch_report_skipped: empty branch"},
		{Result{Status: StatusDisabled, Op: "task_sync", Message: "HUB_SYNC off"}, "hub_task_sync_disabled: HUB_SYNC off"},
		{Result{Status: StatusFailed, Op: "auth", Code: 403}, "hub_auth_failed: status 403"},
	}
	for _, tc := range cases {
		if got := tc.r.Note(); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}
