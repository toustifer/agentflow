# Hub client (optional federation)

> Package: `pkg/hub` · Branch: `feat/agentflow-hub-federation`  
> **Default: fully local.** Hub is opt-in via credentials; kill-switch always wins.

## Decoupling rules

1. **No Hub config** → zero network; task/BT/worktree unchanged.  
2. **`HUB_SYNC=0`** (or `HUB_DISABLED=1` / `HUB_ENABLED=false`) → force off even if token present.  
3. All Hub I/O is **soft-fail** (`Result`); never returns `error` into engine transitions.  
4. **Auth is not a hard gate** for local ops. `EnsureMembership` is advisory for write paths.  
5. Engine (`pkg/engine`) must **not** import `pkg/hub`. Only `pkg/server` (MCP edge) may.

## Layout

```text
pkg/hub/
  config.go    Load / kill-switch / Enabled()
  client.go    HTTP + membership cache
  auth.go      EnsureMembership (JWT /me/businesses or key probe)
  branch.go    ReportBranch (existing soft path)
  task.go      SyncTask (A1 projection, whitelist fields)
  result.go    Result / Note() for MCP payload
```

## Config

| Source | Keys |
|--------|------|
| env | `HUB_BASE_URL`, `HUB_TOKEN`\|`HUB_JWT`, `HUB_API_KEY`, `HUB_BUSINESS_CODE`\|`HUB_BUSINESS` |
| workdir | `.mycompany/hub-client.json` |
| home | `~/.agent-hub/config.json` |
| kill | `HUB_SYNC=0`, `HUB_DISABLED=1`, `HUB_ENABLED=0` |

## Usage (server edge only)

```go
c := hub.NewFromWorkdir(workdir)
note := c.ReportBranch(ctx, hub.BranchReport{...}).Note()
// or
auth := c.EnsureMembership(ctx) // soft; auth.Member
_ = c.SyncTask(ctx, hub.TaskProjection{...})
```

Thin wrappers remain in `pkg/server/hub_branch_report.go` so existing `reportBranchToHub` call sites keep working.

## AI bind flow (MCP tools on agentflow)

```text
hub_status → hub_login (device) → hub_list_teams → hub_bind_team(business_code)
```

| File | Role |
|------|------|
| `login.go` | device start/finish → persist JWT |
| `teams.go` | ListMyTeams / BindTeam / Snapshot |
| `persist.go` | `~/.agent-hub/config.json` merge |

Local-only remains default until JWT + `business_code` are present.

## Next (same branch)

- A2 events / A3 heartbeat (JWT write) when needed.  
- Optional create-team from agentflow MCP.  
- Never require Hub for `flow_ping` / local DAG.
