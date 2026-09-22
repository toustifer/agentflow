# HUB_ALIGNMENT — agent-hub 面 ↔ agentflow 面矩阵

> 口径：**照代码写实**。本文件描述 `feat/hub-federation-rebuild` 上**真实存在**的能力。
> 未实现的部分一律标 ❌ / ⚠️，不做"设计上将会"的表述。
> 验证方式：全部 `net/http/httptest`（本机 Hub 凭据已于 2026-07-29 过期，真实请求必 401）。

---

## 1. 总览矩阵

| Hub 面 | Hub API / 机制 | agentflow 侧实现 | MCP 工具 | 接线状态 | 真实完成度 |
|--------|---------------|------------------|----------|----------|-----------|
| **团队绑定**（namespace ↔ team） | 无网络（本地 metadata + workdir 文件） | `hub.ResolveBusinessCode` / `BindNamespaceTeam` / `SnapshotForNamespace` | ✅ `hub_bind_team`、`hub_status` | ✅ 已接线（master 既有） | **完整**。写入 `namespaces.metadata["hub.business_code"]`，同时镜像到 `{workdir}/.mycompany/hub-client.json`，并清理 home 里遗留的 legacy team code |
| **任务大盘投影** | `POST /v1/hub/dag/{code}` | `hub.SyncTask` | 无（由生命周期自动触发） | ✅ **本次接线**（4 个触发点） | **soft 投影可用**；无重试、无顺序保证（`docs/SYNC_CONTRACT.md` §5） |
| **分支上报 / 防撞车** | `POST /v1/hub/repos/{code}/branches/report` | `hub.ReportBranch` | 无（自动触发） | ✅ **本次接线**（仅 `task_prepare_start`） | **soft 上报可用**；`repo_url` 留空；不含 worktree 路径，只有 `os.Hostname()` |
| **登录 / 凭据** | `POST /v1/hub/auth/device`、`GET /v1/hub/auth/device/token?code=` | `hub.StartDeviceLogin` / `FinishDeviceLogin`（JWT 落 `~/.agent-hub/config.json`） | ❌ **没有 `hub_login` 工具** | ⚠️ 库内实现 + 单测，**未接线** | 需要由 Hub 侧自己的登录入口获取 JWT；agentflow 这边今天不会替你登录 |
| **团队成员列表** | `GET /v1/hub/me/businesses` | `hub.ListMyTeams` | ❌ **没有 `hub_list_teams` 工具** | ⚠️ 库内实现 + 单测，**未接线** | 需要 JWT；**只有 API key 时返回 `StatusSkipped`**（`hub_list_teams_skipped: ...`），不会降级成别的凭据 |
| **成员资格校验** | `GET /v1/hub/me/businesses`（JWT）/ `GET /v1/hub/dag/{code}`（API key） | `hub.EnsureMembership` | 无（内部探针） | ✅ 随投影一起跑 | **顾问而非门闸**：结果被忽略，探针失败也继续写；**冷缓存 = 1 探针 + 1 写** |
| 本地快照 / 诊断 | 无 | `hub.SnapshotForNamespace` | ✅ `hub_status` | ✅ 已接线 | 报告 code + source（env/namespace/workdir）+ legacy home code |
| 消费 Hub 侧状态（H→L） | — | — | — | ❌ **不做** | 单向投影是设计决定，不是缺口 |

---

## 2. 四个触发点接线的真实范围

| MCP 工具 | 任务投影 | 分支上报 | 回填键 |
|----------|----------|----------|--------|
| `task_create` | ✅ | — | `hub_note` |
| `task_prepare_start` | ✅（带 `branch` / `head_sha`） | ✅ `bind_type=task`、`bind_id=<task_id>` | `hub_note` + `hub_branch_note` |
| `task_transition` | ✅（`start`/`resume` 与通用路径两条都覆盖；`submit` 起带 reviewed tip） | — | `hub_note` |
| `task_create_batch` | ✅ 每个 task 一条 | — | 每个 item 的 `hub_note` |

**未接线**（走旧的进程内 `HubSyncer` seam，且 `cmd/agentflow` 从不设置 `Config.HubEnabled` ⇒ 今天零 I/O）：
`task_get`、`task_list`、`task_history`、`task_worker_sync`、`namespace_create`、`namespace_update`、`project_init`、`flow_ping`。

---

## 3. 状态语义映射

| agentflow | 线上 `status` 值 | 备注 |
|-----------|-----------------|------|
| `assigned` | `assigned` | 创建后 / 被 reassign 后 |
| `executing` | `executing` | `start` / `resume` 之后 |
| `review_pending` | `review_pending` | `submit` 之后（此时 `head_sha` = `review.commit`） |
| `rework_needed` | `rework_needed` | `rework` 之后 |
| `done` / `cancelled` | 同名 | 终态 |

状态字符串**原样透传**，agentflow 侧不做 Hub 侧词表映射。

---

## 4. 当前真实完成度（不夸大）

### 已做

- `pkg/hub` 联邦客户端重建完成，51 个包内测试全绿；零第三方依赖（纯 stdlib）。
- 四个生命周期触发点接线完成，note 回填进 MCP 返回值。
- soft-fail 有四类故障的证据：`500`、`401`、连接拒绝、超时 —— 每种都证明工具调用仍成功、本地状态仍推进、note 记录故障。全部基于 `httptest` 假 Hub + 注入的假凭据，未触碰真实 Hub。
- 默认关闭有两个零出网证据：**无配置**与 **`HUB_DISABLED=1`**，均在"base URL 指向活体计数服务"的前提下计数为 0（`pkg/server/hub_projection_test.go`）。
- `pkg/engine` 未 import `pkg/hub`。

### 未做 / 打折

1. **没有对生产 Hub 做过任何验证。** 全部用 `httptest`；`hub.stifer.xyz` 零访问。凭据 2026-07-29 过期，真打必 401。
2. **没有重试 / 离线补发**：`failed` 之后要到该任务的下一次生命周期事件才会重试。
3. **没有顺序保证**：Hub 侧 UPSERT 不比较版本，乱序旧快照可能覆盖新快照。
4. **`hub_login` / `hub_list_teams` 没有 MCP 工具**：登录设备码流程与 `ListMyTeams` 只是库代码。
5. **`repo_url` 未上报**（留空）。
6. **无 H→L**：不拉取、不对账、不解决冲突。
7. **旧的 `BindTeam` / `StatusSnapshot` 那一代没有复引入**：master 的 `BindNamespaceTeam` / `SnapshotForNamespace` 取代了它们；`hub.BindTeam` 会往 home 写 team code，与 JWT-only home 不变量冲突。`pkg/hub/bind.go` 里今天确实有 `StatusSnapshot` 类型，但那是 master 自己的本地快照结构，不是旧联邦分支那一代。
8. **`pkg/hub` 客户端里的 `login.go` / `teams.go` 在生产路径上没有被任何 MCP 工具调用**（见上）。
9. **分支上报只发生在 `task_prepare_start`**：worker 在 worktree 里 commit 之后**不会**自动上报新 tip；新 tip 只通过任务投影的 `head_sha` 体现。

---

## 5. 凭据与 team code 的既有不变量（不要"修"）

| 不变量 | 位置 | 说明 |
|--------|------|------|
| home 是 JWT-only | `pkg/hub/code.go`、`config.go` | `~/.agent-hub/config.json` 的 `business_code` 被 `ResolveBusinessCode` 忽略；`Load()` 也刻意不取它 |
| 一个 namespace ↔ 一个 team | `namespace.metadata["hub.business_code"]` | 机器级 team 绑定不存在，两个 namespace 可以绑不同团队 |
| `Load` 的分层 | `config.go` | env > workdir > home；文件只填补 env 留空的槽位 |
| kill switch 永远优先 | `config.go::killSwitchOn` | 有凭据也能被 env 强制关掉，返回 `StatusDisabled` |
| `pkg/engine` 不 import `pkg/hub` | 依赖方向 | Hub 只存在于 MCP 边缘 |
| API key 不能列团队 | `teams.go::guardJWT` | 只有 API key ⇒ `StatusSkipped` |

---

## 6. 验证入口

```powershell
# 投影接线 + soft-fail + 默认关闭（全部 httptest，零真实出网）
go test -count=1 -run "Projection|TaskGitRefs|PreferReviewTip" ./pkg/server/

# 客户端本体
go test -count=1 ./pkg/hub/

# 依赖方向
git grep -n "pkg/hub" -- pkg/engine    # 必须为空
```

四重门禁（pytest 142 / Go 全量单测 / MCP stdio 烟测 34 PASS + NDJSON 7 PASS / pack-skill）见交付报告。
