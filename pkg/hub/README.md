# pkg/hub — optional agent-hub federation client

> **Default: fully local.** Hub is opt-in via credentials; the kill-switch always wins.
> Nothing in this package is wired into the task lifecycle yet — see "Wiring" below.

This package has two independent halves:

| Half | Files | Network |
|------|-------|---------|
| **Namespace team bind** (product truth for the team code) | `code.go`, `bind.go`, `persist.go` | none |
| **Federation client** (soft task/branch projection, auth, login) | `result.go`, `config.go`, `client.go`, `auth.go`, `task.go`, `branch.go`, `teams.go`, `login.go` | soft, opt-in |

## Decoupling rules

1. **No Hub config** → zero network I/O. Task/BT/worktree behaviour is unchanged.
2. **Kill-switch** (`HUB_SYNC=0` / `HUB_DISABLED=1` / `HUB_ENABLED=false`) → force off even
   with valid credentials. Returns `StatusDisabled`, never `StatusFailed`.
3. All Hub I/O is **soft-fail** (`Result`); it never returns an `error` into engine transitions.
4. **Auth is advisory, not a hard gate.** `EnsureMembership` informs write paths; the Hub
   server is the real enforcer. Local tasks always proceed.
5. `pkg/engine` must not import `pkg/hub`. Only the MCP edge (`pkg/server`) may.

## Result semantics

```go
StatusOK       "ok"        Hub accepted the write
StatusSkipped  "skipped"   prerequisites missing / nothing to send — no dial
StatusDisabled "disabled"  kill-switch — no dial
StatusFailed   "failed"    attempted and did not succeed (transport, 4xx, 5xx)
```

`Result.Note()` renders one stable string for MCP payloads:

```
hub_task_sync_ok
hub_branch_report_failed: status 401 forbidden
hub_task_sync_failed: status 500
hub_auth_skipped: no login token / business_code
hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off
```

Exact shapes are pinned by `TestResultNote` — changing them is a contract change.

## Configuration

`Load(workdir)` layers credentials env → workdir → home; a file only fills a slot env left
empty. `LoadForNamespace(nsMeta, workdir)` additionally resolves the team code from namespace
metadata, which is the normal entry point at a call site that knows its namespace.

| Layer | Keys |
|-------|------|
| env | `HUB_BASE_URL`, `HUB_TOKEN`\|`HUB_JWT`, `HUB_API_KEY`, `HUB_BUSINESS_CODE`\|`HUB_BUSINESS` |
| workdir | `{workdir}/.mycompany/hub-client.json` |
| home | `~/.agent-hub/config.json` |
| kill | `HUB_SYNC=0`, `HUB_DISABLED=1`, `HUB_ENABLED=0` |

`Enabled()` is true only with **a team code AND (token or api_key)** and the kill-switch off.
Anything else is a soft skip.

Two origins are reported separately, because they can legitimately differ:

- `Config.Source` — where the **credential** came from (`env` / file path / `none` / `disabled`).
- `Config.BusinessCodeSource` — where the **team code** came from (`env` / `namespace` /
  workdir path / `""`).

**`~/.agent-hub/config.json` never supplies a team code.** That file is JWT-only by design:
home is machine-wide, so honouring its team code would make two namespaces on one machine
fight over one Hub team. Team code truth lives in namespace metadata (`hub.business_code`),
pinned via `BindNamespaceTeam`, mirrored to the workdir file.

## Endpoints

| Method + path | Purpose |
|---------------|---------|
| `POST /v1/hub/dag/{code}` | `SyncTask` — one task row UPSERT (whitelist fields only) |
| `POST /v1/hub/repos/{code}/branches/report` | `ReportBranch` — branch tip + optional binding |
| `GET /v1/hub/me/businesses` | `EnsureMembership` (JWT) and `ListMyTeams` |
| `GET /v1/hub/dag/{code}` | `EnsureMembership` API-key probe |
| `POST /v1/hub/auth/device` | `StartDeviceLogin` |
| `GET /v1/hub/auth/device/token?code=` | `FinishDeviceLogin` |

Credentials go out as `Authorization: Bearer <jwt>` when a JWT exists, otherwise
`X-API-Key` + `X-Business-Code`. JWT always wins when both are configured.

`EnsureMembership` remembers a **successful** probe for `MembershipCacheTTL` (5 min) and
drops the cache on any 401/403 (`InvalidateAuth`). A failed probe is never cached.

## Never on the wire

Per `docs/SYNC_CONTRACT.md`: no absolute paths (only `os.Hostname()` as `worktree_host`),
no BT internals, no full docs/diary/description, no prompt bodies, no diffs, no secrets.

## Design decisions taken while rebuilding on master

1. **Additive only.** `code.go`, `bind.go`, `persist.go` and their tests are master's and were
   not replaced. The old branch's `persist.go` was an *earlier* generation; its
   `StatusSnapshot` and `BindTeam` are **not** reintroduced — master's `SnapshotForNamespace`
   and `BindNamespaceTeam` supersede them, and `BindTeam`'s write of a team code into the home
   file would contradict the JWT-only-home invariant that `bind_test.go` pins.
2. **`Load` ignores a home team code** (see above). Use `LoadForNamespace` for namespace code.
3. **`ReportBranch` failure notes carry no response body**, so the `status N` suffix matches the
   documented shape exactly. `login_*` keeps the body, as the old branch did.
4. **API-key listing is refused.** `ListMyTeams` needs a JWT; an API key returns `StatusSkipped`.

## Wiring

This package performs no I/O unless a caller invokes it. Connecting `SyncTask` /
`ReportBranch` to `task_create` / `task_transition` is a separate change: it must go through
`pkg/server`'s `HubSyncer` seam, not `pkg/engine`.

## Tests

`go test ./pkg/hub/...` — every case uses `net/http/httptest`; no test reaches a real Hub and
no test needs a credential. Do not add a case that requires either.
