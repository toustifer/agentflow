# agentflow → Hub soft-sync + namespace↔team bind

> Bind model (durable): **one workdir / one namespace ↔ one Hub team (4-char `business_code`)**  
> Soft-sync HTTP projection: still optional / federation branch; bind APIs work on master without network.  
> Hub base: `https://hub.stifer.xyz`

## Product model

```text
目录 / workdir  →  agentflow namespace  →  Hub team (4-char business_code)
```

| Layer | Role |
|-------|------|
| `namespaces.metadata["hub.business_code"]` | **Product truth** for “which team is this project?” |
| `{workdir}/.mycompany/hub-client.json` | Operational mirror for loaders (code only; prefer home JWT) |
| `~/.agent-hub/config.json` | JWT / hub_url; `business_code` = **fallback only** |
| Env `HUB_BUSINESS_CODE` | Temporary override (wins everything) |

**Never** store JWT in SQLite namespace metadata.

### Resolution order

```text
env HUB_BUSINESS_CODE | HUB_BUSINESS
  > namespace.metadata["hub.business_code"]
  > {workdir}/.mycompany/hub-client.json
  > ~/.agent-hub/config.json
```

Canonical code is **exactly 4 chars** `[a-z0-9]{4}` (e.g. `z8gw`).  
Display path `zhiji-z8gw` / URL `/team/zhiji-z8gw` is **not** stored — strip to last 4-char segment.

## Bind API (agentflow MCP)

| Tool | Args | Behavior |
|------|------|----------|
| `namespace_update` | `namespace_id`, optional `name`, `metadata` | Shallow-merge metadata (keeps `workdir` / git keys) |
| `hub_bind_team` | **`namespace_id` + `business_code` required**; optional `workdir`, `set_home_fallback` | Normalize code → write ns `hub.business_code` + workdir file; home only if empty or flag |
| `hub_status` | optional `namespace_id`, `workdir` | Resolved code + `source` (`env`/`namespace`/`workdir`/`home`/`unbound`) |
| `namespace_get` / `list` | | Surfaces `hub.business_code` in metadata |

Example:

```json
// hub_bind_team
{ "namespace_id": "insighttutor", "business_code": "z8gw" }
// also accepts: "zhiji-z8gw" → stores "z8gw"
```

```json
// hub_status response (shape)
{
  "business_code": "z8gw",
  "source": "namespace",
  "namespace_stored_code": "z8gw",
  "home_code": "zk9a",
  "bound": true,
  "resolve_order": ["env", "namespace", "workdir", "home"]
}
```

Recipe:

1. Hub MCP `hub_login` → JWT in `~/.agent-hub/config.json`  
2. Hub `hub_list_my_businesses` / Dashboard → pick 4-char code  
3. agentflow `hub_bind_team({namespace_id, business_code})`  
4. `hub_status({namespace_id})` → `source=namespace`

## Soft-sync credentials (when HTTP sync is enabled)

Home or workdir file still carry JWT:

```json
{
  "hub_url": "https://hub.stifer.xyz",
  "token": "<Hub JWT>",
  "business_code": "z8gw"
}
```

| 项 | 说明 |
|----|------|
| `token` | 人侧 JWT（Hub stamp `actor_email`） |
| `business_code` | **4-char** team code（如 `z8gw`，不是 `zhiji` 展示名） |
| 关闭同步 | `HUB_SYNC=0` 或去掉 token |

When soft-sync is wired, call sites must use `ResolveBusinessCode(ns.Metadata, workdir)` instead of raw home `Load().BusinessCode` alone.

## 何时自动写 Hub（federation / when enabled）

本地 **成功** 后 soft 调用（失败只记 note，不挡任务）：

| 本机动作 | Hub 写入 |
|----------|----------|
| `task_create` / batch | dag UPSERT + event `task.created` |
| prepare_start / start | dag + `task.started` + 可能 branch report |
| submit / pass / rework / … | dag + matching lifecycle event |

**Master note:** if soft-sync HTTP is still a stub, bind + status still work; do not claim events landed until federation projection is on.

## 不会自动写的

- 团队 Doc 全文（Hub `hub_upsert_doc`）  
- Worker 模板发布（`hub_publish_worker_template`）  
- 本机 SQLite / 路径 / BT 状态  

## 代码入口

- `pkg/engine` — `UpdateNamespace` (metadata merge)  
- `pkg/hub/code.go` — `NormalizeBusinessCode`, `ResolveBusinessCode`  
- `pkg/hub/bind.go` — `BindNamespaceTeam`, `SnapshotForNamespace`  
- `pkg/hub/persist.go` — home / workdir JSON writers  
- `pkg/server/hub_bind.go` — MCP `hub_status` / `hub_bind_team`  
- `pkg/server/mcp.go` — `namespace_update`  
