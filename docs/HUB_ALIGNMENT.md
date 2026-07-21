# Agent Hub ↔ agentflow 对齐说明书

> **状态：** 2026-07-21 对照代码事实  
> **仓库：** `agent-hub`（Hub 服务 / Dashboard / MCP） · `agentflow`（本机 MCP 调度 / BT / SQLite）  
> **Hub 基址：** https://hub.stifer.xyz  
> **受众：** 人 + AI。改任何一端的对接前先读本文，避免「以为齐了、实际没写」。

相关文档：

| 文档 | 用途 |
|------|------|
| **[SYNC_CONTRACT.md](./SYNC_CONTRACT.md)** | **同步内容契约（先定字段，再谈展示）** · 线上 `/sync-contract.md` |
| [INTEGRATION.md](./INTEGRATION.md) | 业务接入、JWT 主路径、branch API |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Hub 内部架构与鉴权原则 |
| [MCP_FOR_AI.md](./MCP_FOR_AI.md) / `/mcp.md` | AI 配置 MCP |
| agentflow `skills/agentflow/SETUP.md` | agentflow 本机安装 |

---

## 0. 一句话边界

```text
agentflow = 本机过程库 + 任务生命周期 + worktree/git + BT 调度
Agent Hub = 团队身份 + 跨机共享索引（branch / dag 摘要 / workers / locks / playbooks / events）
MCP hub_* = 人/机器调 Hub 的工具面（不替代 agentflow 的本地 DAG）
```

- **没有 Hub：** agentflow 仍可完整跑任务（纯本地 SQLite + git）。  
- **没有 agentflow：** Hub 仍可做人侧团队（Members / Approvals / 手动 branch report）。  
- **对齐目标：** 让 Dashboard 团队页尽量成为 agentflow 运行态的**只读镜像**；写路径优先 soft-fail，不挡任务。

---

## 1. 鉴权与配置（两端共用约定）

### 1.1 两套凭证

| 路径 | 凭证 | 用途 |
|------|------|------|
| **人侧（主）** | JWT：`Authorization: Bearer <token>` | invite / members / link-request / branch / dag get·sync / list_* |
| **机器（可选）** | `X-API-Key` + `X-Business-Code` | heartbeat / locks / append event / create playbook / sync workers |

创建团队 **默认不生成 API Key**。`RequireMembership`：JWT 必须是该 `business` 成员；Key 必须与 path 上的 `code` 匹配。

### 1.2 配置文件优先级（agentflow `loadHubClientConfig`）

与 `pkg/server/hub_branch_report.go` 一致：

1. 环境变量  
   - `HUB_BASE_URL`（默认 `https://hub.stifer.xyz`）  
   - `HUB_TOKEN` 或 `HUB_JWT`  
   - `HUB_API_KEY`（可选）  
   - `HUB_BUSINESS_CODE` 或 `HUB_BUSINESS`  
2. `{workdir}/.mycompany/hub-client.json`  
3. `~/.agent-hub/config.json`（MCP `hub_login` 落盘）

**生效条件：** 有 `business_code` **且**（`token` 或 `api_key`）。否则 soft-skip。

### 1.3 推荐本机最小配置（无 API Key）

`~/.agent-hub/config.json`（`hub_login` 后）：

```json
{
  "hub_url": "https://hub.stifer.xyz",
  "token": "<jwt>",
  "business_code": "my-team"
}
```

或业务仓（gitignore）：

```json
{
  "hub_url": "https://hub.stifer.xyz",
  "token": "<jwt>",
  "business_code": "my-team"
}
```

路径：`.mycompany/hub-client.json`。

---

## 2. 系统拓扑

```text
┌─────────────────── Claude Code ───────────────────┐
│  MCP: agentflow (stdio)     MCP: hub (stdio)      │
│  namespace/task/dag/worker   hub_login / hub_*      │
└──────────┬──────────────────────────┬─────────────┘
           │                          │
           ▼                          ▼
   agentflow SQLite            hub.stifer.xyz (PG hub.*)
   worktree / git              Dashboard Team 页
           │                          ▲
           │  soft: branches/report   │
           └──────────────────────────┘
              （目前唯一 agentflow→Hub 自动写路径）
```

**禁止混用：**

- 不要把 agentflow 的 `namespace_id` 当成 Hub `business_code`。  
- 不要把 Hub `hub_dag_state` 当成 agentflow 可执行 DAG（Hub 侧是**摘要镜像**）。  
- 不要假设 HTTP `https://hub.stifer.xyz/mcp` 可用（生产曾 502；主路径是 stdio）。

---

## 3. 团队页 Tab × 数据源 × agentflow × MCP 对齐矩阵

状态图例：

| 标记 | 含义 |
|------|------|
| ✅ 已齐 | 代码路径存在，契约稳定 |
| 🟡 半齐 | Hub/MCP 有读写，agentflow 不自动写（或仅手推） |
| ❌ 未齐 | agentflow 无对应推送；页上常空属预期 |
| 👤 人侧 | 与 agentflow 无关，Web/MCP 人操作 |

### 3.1 总表

| 团队页 Tab | Hub 表 / API | 谁写 | 谁读（UI） | agentflow 自动？ | MCP 工具 | 状态 |
|------------|--------------|------|------------|------------------|----------|------|
| **概览** | 聚合 workers/locks/dag/events | 各写路径 | `TeamPage` overview | 随子项 | — | 🟡 跟子项 |
| **Workers** | `hub_workers` · `POST /workers/heartbeat` · `POST /sync/workers` | 机器 Key | `getWorkers` | **否** | `hub_heartbeat` | ❌ |
| **任务 DAG** | `hub_dag_state` · `POST/GET /dag/:code` | JWT member（或将来 soft-sync） | `getDAG` | **否**（本地 DAG 不同步） | `hub_sync_dag` `hub_get_dag` | 🟡 |
| **分布式锁** | `hub_locks` · acquire/renew/release | 机器 Key | `getLocks` | **否**（本地 lease ≠ Hub 锁） | `hub_acquire_lock` 等 | ❌ |
| **经验库** | `hub_playbooks` · create/search · sync workers 灌库 | 机器 Key 写 / JWT 搜 | `searchPlaybooks` | **否**（`docs_sync` 只写本地盘） | `hub_create_playbook` `hub_search_playbooks` | ❌ |
| **事件流** | `hub_events` · `POST /events` · SSE stream | 机器 Key 写 | `getEvents` + SSE | **否** | `hub_append_event` `hub_list_events` | ❌ |
| **Approvals** | `hub_link_requests` | 申请人 JWT；admin 审 | link-request API | 无 | `hub_create_link_request` `hub_list_link_requests` `hub_review_link_request` | 👤 ✅ |
| **Branches** | `hub_branches` / `hub_branch_bindings` · report/list/bind/refresh | JWT 或 Key；agentflow soft report | `listBranches` + refresh | **是**（prepare/start/submit） | `hub_report_branches` `hub_list_branches` `hub_bind_branch` `hub_refresh_branches` | ✅ |
| **Members** | `hub_memberships` / `hub_invites` | admin/owner JWT | invite/list/revoke | 无 | `hub_invite_member` `hub_accept_invitation` | 👤 ✅ |

### 3.2 Branches（已对齐 — 详细契约）

**Hub 路由（JWT 组，middleware 可 tryAPIKey）：**

| Method | Path | Handler | 门禁 |
|--------|------|---------|------|
| POST | `/v1/hub/repos/:code/branches/report` | `ReportBranches` | `RequireMembership` |
| GET | `/v1/hub/repos/:code/branches` | `ListBranches` | 同上 |
| POST | `/v1/hub/repos/:code/branches/bind` | `BindBranch` | 同上 |
| POST | `/v1/hub/repos/:code/branches/unbind` | `UnbindBranch` | 同上 |
| POST | `/v1/hub/repos/:code/branches/refresh` | `RefreshBranches` | 同上（GitHub / ls-remote） |

**agentflow 调用点**（`pkg/server/mcp.go` + `hub_branch_report.go`）：

| 时机 | 函数 | bind |
|------|------|------|
| `task_prepare_start` 成功建 worktree 后 | `reportBranchToHub` | `bind_type=task`，可选再 `dag` |
| task 某些 start 路径 | 同上 | task |
| `task_transition` **submit** | 同上（`review.commit` / head） | task |

**请求体形状（report）：**

```json
{
  "repo_url": "https://github.com/org/repo.git",
  "reporter": "<worker_id>",
  "branches": [{ "name": "feature/x", "tip_sha": "<sha>", "source": "report" }],
  "bindings": [{
    "bind_type": "task|dag|worker",
    "bind_id": "T12",
    "branch_name": "feature/x",
    "head_sha": "<sha>",
    "worktree_host": "<hostname>",
    "status": "active"
  }]
}
```

**鉴权头：** 有 token → `Authorization: Bearer`；否则 `X-API-Key` + `X-Business-Code`。

**失败语义：** 永不阻塞任务；返回 note 字符串：

- `hub_report_skipped: no login token / business_code`
- `hub_report_skipped: empty branch`
- `hub_report_failed: status N` / 网络错误
- `hub_report_ok`

**Tip 权威：** Dashboard「Refresh」走 GitHub API / `ls-remote` 的 tip 高于本机 report；binding head ≠ tip → UI **stale**。

**过程库原则：** docs/tasks **不按 branch 分库**；Hub 只镜像 branch tip + 占用，不存 doc 全文。

### 3.3 任务 DAG（半齐）

| 层 | 存储 | 同步 |
|----|------|------|
| agentflow | 本机 SQLite `dag` / `task` | 权威执行源 |
| Hub | `hub.hub_dag_state`（task 摘要 + 可选 branch/head_sha） | **需** `POST /v1/hub/dag/:code` 或 MCP `hub_sync_dag` |
| UI | `GET /v1/hub/dag/:code` | 只读镜像 |

**MCP `hub_sync_dag` 字段（与 handler 一致）：**  
`task_id, title, status, assigned_worker, depends_on, output_files, branch, head_sha`。

**缺口：** agentflow 在 `task_transition` / `lifecycle_tick` **不会**自动 `SyncDAG`。  
**建议对齐（未做）：** soft-fail `syncDAGToHub` 挂在 create/transition，与 branch report 同级；payload 可带 `hub_dag_sync` note。

### 3.4 Workers（未齐）

| 层 | 行为 |
|----|------|
| Hub | `hub_workers`：heartbeat 刷新 `last_heartbeat_at` / status；`SyncWorkers` 可批量灌 handbook + playbooks |
| agentflow | `worker_register` 等**只写本地**；无 HTTP 调 Hub |
| UI | 90s 无 heartbeat → **stale** |

**缺口：** 无 API Key 时 heartbeat 路径本身就需要机器凭证（或将来开放 JWT 心跳）。  
**建议对齐：**  

1. 可选：JWT 允许 `POST /workers/heartbeat`（membership + 声明 worker_id），或  
2. Phase 5 机器 Key + agentflow 定时 soft heartbeat；  
3. 或 leader_tick 末尾 bulk `sync/workers`（handbook 从本地 handbook 读）。

### 3.5 分布式锁（未齐）

| 层 | 行为 |
|----|------|
| Hub | Redis + `hub_locks`；跨机器文件/资源互斥 |
| agentflow | DAG runtime lease / worktree 占用在**本地 DB**，不注册 Hub 锁 |
| 语义 | **不是同一把锁** — 不要指望 Hub 锁列表反映 agentflow lease |

**何时该接：** 多台机器上的 Claude/agentflow 抢同一文件路径时，才值得 `hub_acquire_lock`。  
单机多 worktree 用 agentflow 本地语义即可。

### 3.6 经验库 Playbooks（未齐）

| 层 | 行为 |
|----|------|
| agentflow | `docs_sync` → `{workdir}/.mycompany/agentflow/`；diary/handbook 本地 |
| Hub | `hub_playbooks` 全文搜；`CreatePlaybook` / `SyncWorkers` patterns·gotchas·decisions |
| UI | 团队页搜索 + worker drawer 关联 playbook |

**缺口：** 本地 experience **不会**自动上传。  
**建议：** 可选 `docs_sync` 之后或 diary 写完 soft `hub_create_playbook`；或批处理脚本读 `.mycompany` 调 `POST /sync/workers`。

### 3.7 事件流（未齐）

| 层 | 行为 |
|----|------|
| Hub | `AppendEvent` + SSE `GET /events/stream?business=&token=` |
| agentflow | lifecycle / BT 事件**不**上报 |
| UI | 轮询 + SSE LIVE |

**建议：** 在 `task_transition` / `lifecycle_tick` 关键节点 soft `hub_append_event`（`event_type` 如 `task.submit`，`actor`=worker_id）。需机器 Key 或扩展 JWT 写事件。

### 3.8 Approvals / Members（人侧已齐）

与 agentflow **无数据依赖**。对齐点只有：

- 用户须 `hub_login` 或 Web 登录；  
- 操作他人团队资源前须 membership；  
- agentflow report 用的 JWT 用户必须已是该 team 成员（否则 403 → soft fail note）。

---

## 4. MCP 工具矩阵（hub 服务器）

源：`agent-hub/mcp-server/lib/tools.js`（人侧 JWT-only；机器 `MACHINE_TOOLS` + `requireApiKey`）。

### 4.1 人侧（JWT，membership 由服务端 Enforce）

| 工具 | Hub API 概念 |
|------|----------------|
| `hub_login` | device OAuth |
| `hub_list_my_businesses` | `GET /me/businesses` |
| `hub_invite_member` / `hub_accept_invitation` | invite |
| `hub_create_link_request` / `hub_list_link_requests` / `hub_review_link_request` | Approvals |
| `hub_add_repo` | 绑 remote |
| `hub_get_dag` / `hub_sync_dag` | 任务 DAG 镜像 |
| `hub_list_workers` / `hub_list_locks` / `hub_list_events` / `hub_search_playbooks` | 读管理面 |
| `hub_report_branches` / `hub_list_branches` / `hub_bind_branch` / `hub_refresh_branches` | Branches |

### 4.2 机器侧（API Key）

| 工具 | 用途 |
|------|------|
| `hub_heartbeat` | Workers 在线 |
| `hub_acquire_lock` / `hub_renew_lock` / `hub_release_lock` | 跨机锁 |
| `hub_append_event` | 事件流写入 |
| `hub_create_playbook` | 经验库写入 |

### 4.3 agentflow MCP 工具（本地，不经过 Hub）

`namespace_*` · `task_*` · `dag_*` · `worker_*` · `lifecycle_tick` · `leader_tick` · `bt_*` · `docs_sync` · `doc_*` · diary/handbook …

**唯一内嵌 Hub HTTP：** `reportBranchToHub`（非独立 MCP 工具名）。

---

## 5. 代码锚点（改对接时先改这里）

### agent-hub

| 区域 | 路径 |
|------|------|
| 路由 | `internal/hub/router.go` |
| membership | `internal/hub/handler/access.go` → `RequireMembership` |
| branch | `branch_handler.go` · `branch_refresh.go` |
| dag 镜像 | `sync_handler.go` → `SyncDAG` / `GetDAG` |
| workers 同步 | `sync_handler.go` → `SyncWorkers` |
| heartbeat / locks / events / playbooks | 对应 `*_handler.go` |
| MCP | `mcp-server/lib/{tools,auth,config}.js` |
| 团队 UI | `frontend/src/views/TeamPage.vue` |
| 建团默认无 Key | `business_handler.go` · `HUB_CREATE_API_KEY` |

### agentflow

| 区域 | 路径 |
|------|------|
| Hub 客户端 + report | `pkg/server/hub_branch_report.go` |
| 调用点 | `pkg/server/mcp.go`（prepare_start / start / submit） |
| 测试 | `pkg/server/hub_branch_report_test.go` |
| 本地文档镜像 | `pkg/server/handlers_docs_sync.go`（**不上 Hub**） |
| 工具列表 | `pkg/server/mcp.go` → `Tools()` |

---

## 6. ID / 命名对齐约定

| 概念 | agentflow | Hub | 对齐规则 |
|------|-----------|-----|----------|
| 团队 | 无（可用 metadata） | `business.code` | 人配置 `business_code`；**不要**自动等于 `namespace_id` |
| 命名空间 | `namespace_id` | — | 仅本地 |
| 任务 | `task_id` | `hub_dag_state.task_id` / binding `bind_id` | sync 时 **同名拷贝** |
| DAG | `dag_id` | binding `bind_type=dag` | report 可绑；Hub dag 表按 **task** 行存 |
| Worker | `worker_id` | `hub_workers.worker_id` | 同名；心跳/sync 用同一 ID |
| Branch | task metadata `git.branch` | `hub_branches.name` | 同 git 名 |
| HEAD | `git.head_sha` / `review.commit` | tip_sha / binding head_sha | submit 用 commit SHA |

---

## 7. 空 Tab 诊断清单

| 现象 | 先查 |
|------|------|
| Branches 空 | 是否 `hub_login` + `business_code`？prepare/submit 是否走过？note 是否 skip？ |
| Branches 有 tip 但 stale | 点 Refresh；本机 head 是否未 push |
| 任务 DAG 空 | 是否有人调过 `hub_sync_dag`？agentflow **不会**自动填 |
| Workers 全 stale/空 | 是否有机器 Key + heartbeat？ |
| 经验库空 | 是否 create_playbook / sync workers？ |
| 事件空 | 是否 append_event？SSE token 是否过期？ |
| Approvals 空 | 正常：无人申请 link |
| report 403 | JWT 用户不是成员 → invite/accept |

---

## 8. 对齐路线图（建议实现顺序）

> 原则：每条都是 **soft-fail**；缺配置不挡 agentflow 任务。

| 阶段 | 内容 | 消费 Tab | 鉴权 |
|------|------|----------|------|
| **A0（已完成）** | JWT 主路径 + membership + branch soft report | Branches | JWT |
| **A1** | agentflow `task_create` / `task_transition` soft `SyncDAG` | 任务 DAG · 概览 pending | JWT |
| **A2** | soft `AppendEvent` 关键节点（start/submit/pass/rework） | 事件流 · 概览 | JWT 扩展 **或** Key |
| **A3** | soft heartbeat 或 `SyncWorkers`（register/handbook） | Workers | JWT 扩展 **或** Key |
| **A4** | 可选：关键文件路径 `withLock` 包装 | 锁 | Key |
| **A5** | 可选：diary/experience → playbook | 经验库 | Key / JWT 写接口 |
| **A6** | Phase 5 机器 Key 管理 UI | 运维 | owner |

**A1 草图（agentflow）：**

```text
syncDAGToHub(ctx, workdir, code from config, task) 
  → POST /v1/hub/dag/{code} { task_id, title, status, assigned_worker, depends_on, branch, head_sha }
  → note: hub_dag_sync_ok | skipped | failed
挂到: task_create, task_transition 成功路径（与 reportBranch 并列）
```

**服务端若坚持 JWT 写 events/heartbeat：** 把部分 worker 路由迁入 JWT 组并加 `RequireMembership`，避免人人被迫造 API Key。

---

## 9. 验收用例（对齐完成度）

### 9.1 当前（A0）必过

1. Web 登录 → 建团 → 响应**无** `api_key`。  
2. MCP `hub_login` → `hub_list_my_businesses`。  
3. 配置 `business_code` + token → agentflow `task_prepare_start` payload 含 `hub_branch_report` 为 `hub_report_ok` 或明确 skip。  
4. Team → **Branches** 可见 tip/binding。  
5. 非成员 JWT report → 403；任务仍成功（soft）。  
6. Members / Approvals 仅人侧可用。

### 9.2 A1+ 追加

7. transition 后 **任务 DAG** 出现对应 `task_id` 与 status。  
8. submit 后事件流出现 `task.submit`（若做 A2）。  
9. heartbeat 后 Workers 非 stale（若做 A3）。

---

## 10. 反模式（明确不要做）

1. **用 API Key 做人侧主路径** — 已否决；文档与 UI 不主推。  
2. **agentflow 把 Hub 当执行引擎** — Hub 不调度 BT，不持有 worktree。  
3. **按 branch 拆 agentflow SQLite** — 过程库统一 namespace。  
4. **硬失败** — Hub 网络抖动不得 fail 任务 transition。  
5. **魔法 business_code**（如历史 `ai-medbox`）— 禁止默认。  
6. **把 HTTP `/mcp` 写进安装默认** — 以 stdio 为准。  
7. **混淆 Approvals 与 agentflow review** — Hub Approvals = 入团审批；agentflow review = 任务 pass/rework。

---

## 11. 文档维护

| 变更类型 | 更新本文章节 |
|----------|----------------|
| 新增 agentflow→Hub 写路径 | §3 对应 Tab + §5 锚点 + §8 勾掉阶段 |
| 改鉴权矩阵 | §1 + §4 |
| 改 Team 页 Tab | §3 总表 |
| 新 MCP 工具 | §4 |

**对外 URL（部署后）：**  
- 源文件：`docs/AGENTFLOW_ALIGNMENT.md`  
- 静态/路由：`/agentflow-alignment.md`（与 `/mcp.md` 同级 serve，见 `internal/server/server.go`）

修订时请改文首日期，并在 PR 描述贴矩阵 diff。
