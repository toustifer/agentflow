# agentflow → Hub soft-sync + namespace↔team bind

> Bind model: **one namespace ↔ one Hub team (4-char `business_code`)**  
> **No machine-wide team bind.** `~/.agent-hub/config.json` is **JWT / hub_url only**.  
> Soft-sync HTTP projection: optional / federation; bind APIs work without network.  
> Hub base: `https://hub.stifer.xyz`

## Product model

```text
目录 / workdir  →  agentflow namespace  →  Hub team (4-char business_code)
```

| Layer | Role |
|-------|------|
| `namespaces.metadata["hub.business_code"]` | **Only product truth** for “which team is this project?” |
| `{workdir}/.mycompany/hub-client.json` | Per-repo mirror of that namespace’s code |
| `~/.agent-hub/config.json` | **JWT only** — `business_code` here is ignored / stripped on bind |
| Env `HUB_BUSINESS_CODE` | Process override (CI / debug) |

**Never** store JWT in SQLite. **Never** use home `business_code` as multi-project default.

### Resolution order

```text
env HUB_BUSINESS_CODE | HUB_BUSINESS
  > namespace.metadata["hub.business_code"]
  > {workdir}/.mycompany/hub-client.json
  # home business_code: NOT used
```

Canonical code is **exactly 4 chars** `[a-z0-9]{4}` (e.g. `z8gw`).  
Display path `zhiji-z8gw` is stripped to the last 4-char segment.

## Bind API (agentflow MCP)

| Tool | Args | Behavior |
|------|------|----------|
| `namespace_update` | `namespace_id`, optional `name`, `metadata` | Shallow-merge metadata |
| `hub_bind_team` | **`namespace_id` + `business_code` required** | Write ns + workdir; clear legacy home team code; **never** set home as team bind |
| `hub_status` | optional `namespace_id`, `workdir` | Resolved code + source (`env`/`namespace`/`workdir`/`unbound`); may show `home_legacy_code` if residual |

```json
{ "namespace_id": "insighttutor", "business_code": "z8gw" }
```

Two namespaces can bind two different codes on one machine with no conflict.

## Soft-sync credentials

```json
{
  "hub_url": "https://hub.stifer.xyz",
  "token": "<Hub JWT>"
}
```

Do not put `business_code` in home for product binding. Soft-sync callers must use `ResolveBusinessCode(ns.Metadata, workdir)`.

## Migration

If home still has `"business_code": "..."` from older soft-sync:

1. Call `hub_bind_team` per namespace (clears home legacy), or  
2. Manually delete `business_code` / `bound_at` from `~/.agent-hub/config.json`

## 代码入口

- `pkg/hub/code.go` — `NormalizeBusinessCode`, `ResolveBusinessCode` (no home)
- `pkg/hub/bind.go` — `BindNamespaceTeam`, `SnapshotForNamespace`
- `pkg/hub/persist.go` — JWT home writers + `ClearHomeBusinessCode`
- `pkg/server/hub_bind.go` — MCP tools
