package hub

// Result is a soft-fail outcome for Hub side-effects.
// Callers must never treat non-OK as a hard error for local task flow.
type Result struct {
	Status  Status
	Op      string // e.g. "branch_report", "task_sync", "auth"
	Message string
	Code    int // HTTP status when relevant; 0 otherwise
}

type Status string

const (
	StatusOK       Status = "ok"
	StatusSkipped  Status = "skipped"
	StatusDisabled Status = "disabled"
	StatusFailed   Status = "failed"
)

// Note returns a stable string for optional MCP payload fields (hub_*).
func (r Result) Note() string {
	if r.Op == "" {
		r.Op = "hub"
	}
	switch r.Status {
	case StatusOK:
		if r.Message != "" {
			return "hub_" + r.Op + "_ok: " + r.Message
		}
		return "hub_" + r.Op + "_ok"
	case StatusDisabled:
		if r.Message != "" {
			return "hub_" + r.Op + "_disabled: " + r.Message
		}
		return "hub_" + r.Op + "_disabled"
	case StatusSkipped:
		if r.Message != "" {
			return "hub_" + r.Op + "_skipped: " + r.Message
		}
		return "hub_" + r.Op + "_skipped"
	default:
		if r.Code > 0 && r.Message != "" {
			return "hub_" + r.Op + "_failed: status " + itoa(r.Code) + " " + r.Message
		}
		if r.Code > 0 {
			return "hub_" + r.Op + "_failed: status " + itoa(r.Code)
		}
		if r.Message != "" {
			return "hub_" + r.Op + "_failed: " + r.Message
		}
		return "hub_" + r.Op + "_failed"
	}
}

func (r Result) OK() bool { return r.Status == StatusOK }

func (r Result) SoftSkip() bool {
	return r.Status == StatusSkipped || r.Status == StatusDisabled
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
