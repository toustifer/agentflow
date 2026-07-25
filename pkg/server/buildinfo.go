package server

// Build metadata injected by cmd/agentflow via SetBuildInfo (ldflags on release).
var (
	BuildVersion = "dev"
	BuildCommit  = "unknown"
	BuildDate    = "unknown"
)

// SetBuildInfo records binary version for flow_ping / diagnostics.
func SetBuildInfo(version, commit, date string) {
	if version != "" {
		BuildVersion = version
	}
	if commit != "" {
		BuildCommit = commit
	}
	if date != "" {
		BuildDate = date
	}
}

// BuildInfoMap is safe to embed in tool responses.
func BuildInfoMap() map[string]any {
	return map[string]any{
		"version": BuildVersion,
		"commit":  BuildCommit,
		"date":    BuildDate,
	}
}
