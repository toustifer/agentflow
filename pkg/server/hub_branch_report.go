package server

import (
	"context"

	"github.com/toustifer/agentflow/pkg/hub"
)

// loadHubClientConfig is kept for tests; delegates to pkg/hub.
// Soft-fail: missing config → Enabled() false.
func loadHubClientConfig(workdir string) *hub.Config {
	cfg := hub.Load(workdir)
	if cfg == nil || !cfg.Enabled() {
		return nil
	}
	return cfg
}

// reportBranchToHub best-effort posts branch tip + bindings. Never fails callers.
// Fully decoupled: no Hub config / HUB_SYNC=0 → skip note only.
func reportBranchToHub(ctx context.Context, workdir, repoURL, branch, headSHA, bindType, bindID, reporter string) string {
	c := hub.NewFromWorkdir(workdir)
	return c.ReportBranch(ctx, hub.BranchReport{
		RepoURL:  repoURL,
		Branch:   branch,
		HeadSHA:  headSHA,
		BindType: bindType,
		BindID:   bindID,
		Reporter: reporter,
	}).Note()
}

// syncTaskToHub soft-projects task row (SYNC_CONTRACT A1). Optional; not wired to hard paths.
func syncTaskToHub(ctx context.Context, workdir string, in hub.TaskProjection) string {
	return hub.NewFromWorkdir(workdir).SyncTask(ctx, in).Note()
}

// ensureHubMembership soft auth check for future gates; never required for local ops.
func ensureHubMembership(ctx context.Context, workdir string) hub.AuthInfo {
	return hub.NewFromWorkdir(workdir).EnsureMembership(ctx)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if len(v) > 0 {
			// trim spaces without importing strings for tiny helper
			i, j := 0, len(v)
			for i < j && (v[i] == ' ' || v[i] == '\t' || v[i] == '\n' || v[i] == '\r') {
				i++
			}
			for j > i && (v[j-1] == ' ' || v[j-1] == '\t' || v[j-1] == '\n' || v[j-1] == '\r') {
				j--
			}
			if i < j {
				return v[i:j]
			}
		}
	}
	return ""
}

// detectRemoteURL best-effort origin URL for repo binding.
func detectRemoteURL(ctx context.Context, repoPath string) string {
	if repoPath == "" {
		return ""
	}
	out, err := runGit(ctx, repoPath, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	for len(out) > 0 && (out[len(out)-1] == '\n' || out[len(out)-1] == '\r') {
		out = out[:len(out)-1]
	}
	return out
}
