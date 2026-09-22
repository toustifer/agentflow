package hub

import "strconv"

// Status is the four-state outcome of a soft Hub side effect.
//
// The distinction matters for callers (and for humans reading MCP output):
//
//	ok       — the Hub accepted the write
//	skipped  — no credentials / nothing to send; Hub was never called
//	disabled — the operator killed Hub I/O via env; Hub was never called
//	failed   — we tried and it did not work (transport, 4xx, 5xx)
//
// Only "failed" is worth retrying or alarming on.
type Status string

const (
	// StatusOK means the Hub accepted the request.
	StatusOK Status = "ok"
	// StatusSkipped means prerequisites were missing (no token/business_code,
	// empty payload); no network I/O was attempted and this is not an error.
	StatusSkipped Status = "skipped"
	// StatusDisabled means HUB_SYNC / HUB_DISABLED / HUB_ENABLED turned Hub I/O off.
	StatusDisabled Status = "disabled"
	// StatusFailed means the request was attempted and did not succeed.
	StatusFailed Status = "failed"
)

// Result is a soft-fail outcome for a Hub side effect.
//
// Callers must never turn a non-OK Result into a hard error for the local task
// lifecycle: Hub is a mirror, agentflow SQLite stays the source of truth.
type Result struct {
	Status  Status
	Op      string // stable op token, e.g. "task_sync", "branch_report", "auth"
	Message string // short human-readable detail
	Code    int    // HTTP status when relevant; 0 otherwise
}

// Note renders the Result as a single stable, human-readable string for MCP
// payloads (e.g. "hub_task_sync_ok", "hub_report_failed: status 401").
//
// The exact shapes are pinned by TestResultNote so downstream notes stay stable:
//
//	ok       → hub_<op>_ok[: <message>]
//	skipped  → hub_<op>_skipped[: <message>]
//	disabled → hub_<op>_disabled[: <message>]
//	failed   → hub_<op>_failed[: status <code>][ <message>]
func (r Result) Note() string {
	op := r.Op
	if op == "" {
		op = "hub"
	}
	switch r.Status {
	case StatusOK:
		if r.Message != "" {
			return "hub_" + op + "_ok: " + r.Message
		}
		return "hub_" + op + "_ok"
	case StatusDisabled:
		if r.Message != "" {
			return "hub_" + op + "_disabled: " + r.Message
		}
		return "hub_" + op + "_disabled"
	case StatusSkipped:
		if r.Message != "" {
			return "hub_" + op + "_skipped: " + r.Message
		}
		return "hub_" + op + "_skipped"
	default:
		if r.Code > 0 && r.Message != "" {
			return "hub_" + op + "_failed: status " + strconv.Itoa(r.Code) + " " + r.Message
		}
		if r.Code > 0 {
			return "hub_" + op + "_failed: status " + strconv.Itoa(r.Code)
		}
		if r.Message != "" {
			return "hub_" + op + "_failed: " + r.Message
		}
		return "hub_" + op + "_failed"
	}
}

// OK reports whether the Hub accepted the side effect.
func (r Result) OK() bool { return r.Status == StatusOK }

// SoftSkip reports whether nothing was attempted for a benign reason
// (missing config or kill-switch). Callers use this to avoid noisy notes.
func (r Result) SoftSkip() bool {
	return r.Status == StatusSkipped || r.Status == StatusDisabled
}
