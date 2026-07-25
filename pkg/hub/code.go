package hub

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// MetaBusinessCode is the namespace metadata key for the bound Hub team.
const MetaBusinessCode = "hub.business_code"

// MetaBoundAt is optional ISO timestamp of last bind.
const MetaBoundAt = "hub.bound_at"

// MetaTeamName is optional display name (never used for resolve).
const MetaTeamName = "hub.team_name"

// shortCodeRe matches current Hub short codes (4-char; 5-char is a future extension).
var shortCodeRe = regexp.MustCompile(`^[a-z0-9]{4}$`)

// NormalizeBusinessCode accepts a bare 4-char code or a display path like
// "zhiji-z8gw" / "insighttutor-z8gw" and returns the canonical short code.
func NormalizeBusinessCode(input string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(input))
	if s == "" {
		return "", fmt.Errorf("business_code is required")
	}
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "-"); i >= 0 {
		tail := s[i+1:]
		if shortCodeRe.MatchString(tail) {
			return tail, nil
		}
	}
	if shortCodeRe.MatchString(s) {
		return s, nil
	}
	return "", fmt.Errorf(
		"invalid business_code %q: need 4-char short code (e.g. z8gw) or display path ending in it (e.g. zhiji-z8gw)",
		input,
	)
}

// ResolveBusinessCode picks the active team code for a namespace/workdir.
//
// Priority (no machine-wide team bind):
//
//	env HUB_BUSINESS_CODE|HUB_BUSINESS
//	  > namespace.metadata["hub.business_code"]
//	  > {workdir}/.mycompany/hub-client.json
//
// ~/.agent-hub/config.json may hold JWT only — its business_code is IGNORED
// so multiple namespaces never fight over one global team.
// Returns ("", "unbound") when nothing is set.
func ResolveBusinessCode(nsMeta map[string]string, workdir string) (code, source string) {
	if v := firstNonEmpty(os.Getenv("HUB_BUSINESS_CODE"), os.Getenv("HUB_BUSINESS")); v != "" {
		if n, err := NormalizeBusinessCode(v); err == nil {
			return n, "env"
		}
		return strings.TrimSpace(strings.ToLower(v)), "env"
	}
	if nsMeta != nil {
		if v := strings.TrimSpace(nsMeta[MetaBusinessCode]); v != "" {
			if n, err := NormalizeBusinessCode(v); err == nil {
				return n, "namespace"
			}
			return strings.TrimSpace(strings.ToLower(v)), "namespace"
		}
	}
	if workdir == "" && nsMeta != nil {
		workdir = strings.TrimSpace(nsMeta["workdir"])
	}
	if workdir != "" {
		if v := readBusinessCodeFile(WorkdirClientPath(workdir)); v != "" {
			if n, err := NormalizeBusinessCode(v); err == nil {
				return n, "workdir"
			}
			return v, "workdir"
		}
	}
	return "", "unbound"
}

// HomeConfigPath returns ~/.agent-hub/config.json (empty if home unknown).
// Used for JWT storage only — not for team resolution.
func HomeConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".agent-hub", "config.json")
}

// WorkdirClientPath returns {workdir}/.mycompany/hub-client.json.
func WorkdirClientPath(workdir string) string {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return ""
	}
	return filepath.Join(workdir, ".mycompany", "hub-client.json")
}

func readBusinessCodeFile(path string) string {
	if path == "" {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var file struct {
		BusinessCode string `json:"business_code"`
	}
	if json.Unmarshal(raw, &file) != nil {
		return ""
	}
	return strings.TrimSpace(file.BusinessCode)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
