# SYNC_CONTRACT — agentflow → agent-hub 单向投影契约

> 状态：**已接线**（`task-2-lifecycle-wiring`，分支 `feat/hub-federation-rebuild`）。
> 实现：`pkg/hub`（客户端）+ `pkg/server/hub_project.go`（唯一的接线点）。
> 本文只描述**代码里真实存在**的行为；未实现的部分集中在 §7。

---

## 1. 三条铁律

| # | 铁律 | 含义 |
|---|------|------|
| 1 | **单向 L→H** | 只做 agentflow → Hub。没有任何代码把 Hub 状态读回 SQLite；Hub 是镜像，`agentflow` SQLite 是唯一真源。 |
| 2 | **soft-fail** | 任何 Hub 故障（DNS/连接拒绝/超时/4xx/5xx/解析失败）都**不得**让 MCP 工具调用返回错误，**不得**回滚任何本地状态。失败只体现为一个 note 字符串。 |
| 3 | **默认关闭 / 零出网** | 没有 team code **或**没有凭据 ⇒ `StatusSkipped`，**零请求**。用户不配置就绝不会被上报。 |

实现上，`pkg/engine` **不 import `pkg/hub`**（`pkg/engine` 不知道 Hub 存在）；投影只发生在 MCP 边缘 `pkg/server`。

---

## 2. 开关、凭据与 team code

### 2.1 是否出网的唯一判据

```text
Config.Enabled() == true
  ⟺ business_code 非空
  ∧ ( token 非空 ∨ api_key 非空 )
  ∧ kill switch 未触发
```

其余一切情况都是 `StatusSkipped` / `StatusDisabled`，**不发起任何请求**。

### 2.2 kill switch（三者任一，且永远优先）

| 变量 | 触发 "关闭" 的取值 |
|------|-------------------|
| `HUB_SYNC` | `0` / `false` / `off` / `no` / `disabled` |
| `HUB_DISABLED` | `1` / `true` / `on` / `yes` / `disabled` |
| `HUB_ENABLED` | `0` / `false` / `off` / `no` / `disabled`（留空 = 按凭据自动） |

命中 kill switch ⇒ `StatusDisabled`（不是 `failed`），note 为 `hub_<op>_disabled: HUB_SYNC/HUB_ENABLED off`。

### 2.3 凭据分层（env > workdir > home）

| 层 | 键 |
|----|----|
| env | `HUB_BASE_URL`、`HUB_TOKEN` \| `HUB_JWT`、`HUB_API_KEY`、`HUB_BUSINESS_CODE` \| `HUB_BUSINESS` |
| workdir | `{workdir}/.mycompany/hub-client.json` |
| home | `~/.agent-hub/config.json` |

文件只会填补 env 留空的槽位。凭据在线上以 `Authorization: Bearer <jwt>` 优先；没有 JWT 时用 `X-API-Key` + `X-Business-Code`。**JWT 永远优先于 API key。**

### 2.4 team code（业务码）解析顺序

```text
env HUB_BUSINESS_CODE | HUB_BUSINESS
  > namespace.metadata["hub.business_code"]     ← 唯一产品真源（hub_bind_team 写的就是它）
  > {workdir}/.mycompany/hub-client.json
  # ~/.agent-hub/config.json 的 business_code：永不使用（JWT-only home）
```

**home 是 JWT-only。** `Load()` 刻意不取 home 的 team code：home 是机器级的，若承认它，同一台机器上的两个 namespace 会争抢同一个 Hub 团队。接线点因此使用 `hub.NewFromNamespace(nsMeta, workdir)`（= `LoadForNamespace` + `New`），team code 只可能来自 env / namespace / workdir。这条不变量由 `pkg/hub/config_test.go`、`bind_test.go` 钉住，**不要"修"它**。

### 2.5 出网代价（冷缓存）

每个触发点都**新建一个 client**（`hub.NewFromNamespace`）。这是有意的：`hub_bind_team`、换凭据、`HUB_DISABLED=1` 都会在下一次工具调用立即生效，而不是等进程重启。

代价是 `pkg/hub` 的成员缓存**每次都从冷态开始**：

```text
一次已启用的任务投影 = 1 次成员探针(GET) + 1 次写入(POST) = 2 个请求
一次已启用的分支上报 = 1 次成员探针(GET) + 1 次写入(POST) = 2 个请求
未配置 / 被 kill switch ⇒ 0 个请求
```

### 2.6 鉴权是顾问，不是门闸

`EnsureMembership` 的结果**被忽略**（`_ = c.EnsureMembership(ctx)`）：探针失败也继续尝试写入，真正的 enforcer 是 Hub 服务端。本地任务永远照常推进。`ListMyTeams`（MCP：`hub_list_teams`，见 §4.5）是唯一的例外——它需要 JWT，只有 API key 时返回 `StatusSkipped` 且**零出网**。

---

## 3. 出网字段白名单

### 3.1 任务行 — `POST /v1/hub/dag/{business_code}`

`pkg/hub.TaskProjection`，**恰好 8 个字段**：

| 字段 | 来源 |
|------|------|
| `task_id` | `Task.ID` |
| `title` | `Task.Title` |
| `status` | `Task.State`（agentflow 状态字符串，原样透传） |
| `assigned_worker` | `Task.AssignedWorker` |
| `depends_on` | `Task.DependsOn` |
| `output_files` | `Task.OutputFiles` |
| `branch` | task metadata `git.branch`（无 worktree 时为空串） |
| `head_sha` | 见 §4.2 |

字段名由 `pkg/hub/task_test.go::TestSyncTaskBodyFields` 钉死，**改名即毁约**。

### 3.2 分支上报 — `POST /v1/hub/repos/{business_code}/branches/report`

```jsonc
{
  "reporter":   "agentflow",
  "repo_url":   "",                       // 当前留空：namespace metadata 没有规范 remote 字段
  "branches":   [{ "name": "<branch>", "tip_sha": "<sha>", "source": "report" }],
  "bindings":   [{
    "bind_type":     "task",
    "bind_id":       "<task_id>",
    "branch_name":   "<branch>",
    "head_sha":      "<sha>",
    "worktree_host": "<os.Hostname()>",   // 只放主机名
    "status":        "active"
  }]
}
```

> `bindings[].head_sha` 与 `branches[].tip_sha` 同源，都是本次投影的 `head_sha`（见 §4.2）。

### 3.3 永不上网

- ❌ 任何绝对路径（worktree 路径、repo 路径、home 目录）
- ❌ `description` / `acceptance_criteria` / `tags` / `metadata` 全文
- ❌ `review.diff` / 任何 diff 正文
- ❌ prompt 正文、worker prompt template
- ❌ 日记 / 文档 / handbook 正文
- ❌ BT（行为树）内部状态、blackboard
- ❌ 任何密钥：token / api_key / JWT 只出现在请求头，绝不进 body
- ✅ 唯一与机器身份相关的字段是 `worktree_host = os.Hostname()`

---

## 4. 触发点与 note 回填

### 4.1 触发点

| 时机 | 入口 | task 投影 | 分支上报 | payload 键 |
|------|------|-----------|----------|------------|
| `task_create` | `Handle` → `handleTaskCreate` | ✅ 8 字段 | — | `hub_note` |
| `task_prepare_start` | `Handle` → `handleTaskPrepareStart` | ✅ 8 字段（带 branch/head） | ✅ `bind_type=task` | `hub_note` + `hub_branch_note` |
| `task_transition` | `Handle` → `handleTaskTransition`（start/resume 路径与通用路径**两条**都覆盖） | ✅ 8 字段（submit 及之后带 reviewed tip） | ✅ **仅当 `transition=submit`**（`bind_type=task`，`tip_sha` = `review.commit`） | `hub_note`（submit 时另有 `hub_branch_note`） |
| `task_create_batch` | `handleTaskCreateBatch` | ✅ **每个** task 一条 | — | 每个 item 的 `hub_note` |

判定由 `submitReportsBranch(input)` 唯一决定（`pkg/server/hub_project.go`），只有 `submit` 返回 true。**一个事件只走一条路径**：`start`/`resume`/`pass`/`rework`/`reassign`/`cancel` 只投影 task row，不重复上报分支。

note 在本地状态**已经提交之后**才计算并回填，所以 Hub 故障对生命周期完全不可见：工具照常返回推进后的 task，只有 note 记录镜像发生了什么。

### 4.2 branch / head_sha 的取值

| 触发点 | `branch` | `head_sha` |
|--------|----------|------------|
| `task_create` / `task_create_batch` | 空 | 空（新任务还没有 worktree） |
| `task_prepare_start` | `git.branch` | `git.head_sha`（刚建好的 worktree 顶端） |
| `task_transition` = `start` / `resume` | `git.branch` | `git.head_sha`（刚刷新，此时 `review.commit` 可能是上一轮的陈旧值） |
| `task_transition` = `submit` / `pass` / `rework` / `cancel` / `reassign` | `git.branch` | `review.commit`（reviewer 将看到的那一个 tip），缺失时回退 `git.head_sha` |
| 分支上报（`task_prepare_start` / `submit`） | `git.branch` | 同上，即 `bindings[].head_sha` 与 `branches[].tip_sha` **相同** |

`submit` 上的这个 `review.commit` 就是防撞车的关键：它是 worker「已经落地、reviewer 即将看到」的那个 tip，而不是 `task_prepare_start` 当时的 base tip（`TestTransitionSubmitCarriesReviewedTip` 直接对线上 body 断言 `tip_sha == review.commit != baseTip`）。

### 4.3 note 的确切格式

`Result.Note()`，由 `pkg/hub/result_test.go::TestResultNote` 钉死：

```text
hub_task_sync_ok
hub_task_sync_skipped: no login token / business_code
hub_task_sync_disabled: HUB_SYNC/HUB_ENABLED off
hub_task_sync_failed: status 401 forbidden
hub_task_sync_failed: status 500
hub_task_sync_failed: <transport error>
hub_branch_report_ok
hub_branch_report_skipped: no login token / business_code
hub_branch_report_disabled: HUB_SYNC/HUB_ENABLED off
hub_branch_report_failed: status 403 forbidden
hub_branch_report_skipped: empty branch
hub_auth_skipped / hub_auth_ok / hub_auth_failed: ...
hub_list_teams_ok: N teams / hub_list_teams_skipped: not logged in — call hub_login first
hub_list_teams_failed: status 401 <message>
hub_login_start_ok: <user code> / hub_login_start_disabled: HUB_SYNC/HUB_ENABLED off
hub_login_start_failed: status <code> <message> / hub_login_start_failed: <transport error>
hub_login_finish_ok: token saved to ~/.agent-hub/config.json
hub_login_finish_skipped: pending approval — open verification URL and click Approve
hub_login_finish_failed: status <code> <message> / hub_login_finish_failed: <transport error>
```

op token 只有这六个：`task_sync`、`branch_report`、`auth`、`list_teams`、`login_start`、`login_finish`。

> `login_start` / `login_finish` 是**两个** op，不是笼统的 `login`：开始设备码与轮询取件是两条独立的路由（`POST /v1/hub/auth/device` / `GET /v1/hub/auth/device/token`），失败时要能分辨是哪一步坏了。由 `pkg/hub/login_test.go` 钉住。

### 4.4 旧 seam 已删除（不再有第二条 Hub 路径）

`task_get` / `task_list` / `task_history` / `task_worker_sync` / `namespace_create` / `namespace_update` / `project_init` / `flow_ping` **没有任何 Hub 副作用**，而且代码里也不再有能被误读成「已接线」的路径：

- 旧的进程内 `pkg/server.HubSyncer` seam（`noopHubSyncer`）连同 `Server.hub` 字段、`Config.HubEnabled`、`Config.HubBusinessCode` 与 `pkg/server/sync.go` 整个文件已**删除**。它只在 `Config.HubEnabled` 为 true 时被装上，而 `cmd/agentflow` 从不设置该字段 ⇒ **从来就是零 I/O 的死路径**。它的接口只能返回 `error`，承载不了 note（`hub.Result`），本身就是错误的抽象。
- 今天 `pkg/server` 里 Hub 出口**只有一个**：`hubProjector` / `realHubProjector`（`pkg/server/hub_project.go`），已接在 §4.1 的 4 个触发点上。
- 上述 8 个工具为什么不需要投影：`task_get` / `task_list` / `task_history` / `flow_ping` 是读/诊断接口，投影它们只会产生无意义的重复写；`namespace_*` / `project_init` 的团队绑定真值走的是 `hub_bind_team`（本地 metadata，无网络）；`task_worker_sync` 带来的状态变化由紧随其后的 `task_transition` 承担。
- 回归证明：`git grep -n "HubSyncer\|noopHubSyncer\|HubEnabled\|HubBusinessCode" -- '*.go'` 在 `pkg/` `cmd/` 下**已无任何代码引用**（只剩 `pkg/server/server.go` 与 `pkg/server/types.go` 里两处解释性**注释**在说明它为何被删除）；`pkg/server/mcp_test.go` 里依赖旧 seam 的 8 个用例（`failingHubSyncer` / `trackingHubSyncer`）作为**预期测试删除**一并移除，这几个工具本身的正向覆盖仍在（`task_get` / `task_list` / `task_history` / `namespace_create` 各有其它用例）。

### 4.5 凭据类工具（不参与任务投影）

`pkg/server/hub_login_tools.go` 暴露两个**凭据**入口。它们不投影任何任务、不写 SQLite，唯一的持久化副作用是 `hub_login` 成功时把 JWT 写进 `~/.agent-hub/config.json`。

| MCP 工具 | 端点 | 入参 | payload 关键字段 |
|----------|------|------|------------------|
| `hub_login`（`step=start`） | `POST /v1/hub/auth/device` | 无（`code` 缺省） | `status=pending_approval`、`code`、`verification_url`、`expires_in`、`approved=false` |
| `hub_login`（`step=finish`） | `GET /v1/hub/auth/device/token?code=` | `code` | 未批准 ⇒ `status=pending_approval`（**不是** `failed`）；成功 ⇒ `status=ok`、`logged_in=true`、`token_saved=true`、`business_code=""`、`home_config_jwt_only=true` |
| `hub_list_teams` | `GET /v1/hub/me/businesses` | 可选 `namespace_id` / `workdir` | `status`、`count`、`teams[]`、`has_jwt`、`has_api_key` |

三条硬约束：

1. **`pending_approval` 不是失败。** `202 Accepted`、`{"status":"pending"}`、以及 200-但无 token 三种情形都映射成可反复重试的 `pending_approval`，`hint` 里带着原 `code`。
2. **JWT 绝不回显。** 工具返回值里没有 token 字段（SYNC_CONTRACT §3.3），也不发明 team code —— home 保持 JWT-only（§2.4）。
3. **API key 不能列团队。** 只有 `HUB_API_KEY` 时 `hub_list_teams` 返回 `status=skipped`，并在 `hint` 里点名 `hub_login`；**零出网**（请求根本不发）。

`hub_login` / `hub_list_teams` 的入参全部可选，且 `namespace_id` / `workdir` 只影响**用哪一层配置去解析 base URL 与凭据**，不影响投影行为。验证：`go test -count=1 -run "HubLogin|HubListTeams" -v ./pkg/server/`（全部 `httptest`，`HOME` 被重定向到 `t.TempDir()`）。

---

## 5. 重试、补偿、顺序

- **没有重试队列，没有离线补发。** 一次 `failed` 之后，该任务要到**下一次生命周期事件**才会重新投影。
- **没有顺序保证。** Hub 侧的 UPSERT 以 `task_id` 为键；乱序到达的旧快照可能覆盖新快照。当前不做版本号/时间戳比对。
- **不做 H→L**：不拉取、不对账、不冲突解决。

---

## 6. 端点清单

| 方法 + 路径 | 用途 | 触发时机 |
|-------------|------|----------|
| `POST /v1/hub/dag/{code}` | 任务行 UPSERT | §4.1 四个触发点 |
| `POST /v1/hub/repos/{code}/branches/report` | 分支 tip + 绑定 | 仅 `task_prepare_start` |
| `GET /v1/hub/me/businesses` | `EnsureMembership` 探针（JWT）、`ListMyTeams`（`hub_list_teams`） | 每次投影前（探针，结果被忽略）；以及显式调用 `hub_list_teams` 时 |
| `GET /v1/hub/dag/{code}` | `EnsureMembership` 探针（API key） | 同上 |
| `POST /v1/hub/auth/device` | 设备码登录开始 | `hub_login({})`（`step=start`） |
| `GET /v1/hub/auth/device/token?code=` | 设备码登录完成（轮询一次） | `hub_login({code})`（`step=finish`） |

请求超时 `5s`（`pkg/hub.defaultTimeout`），响应体上限 1 MiB。

---

## 7. 明确的非目标 / 当前未完成

1. **没有 H→L**：不拉 Hub 状态，不做双向对账。
2. **没有重试/补偿**（见 §5）。
3. **登录与团队列表只有 `httptest` 验证，没有对生产 Hub 跑通**：`hub_login`（设备码两段式）与 `hub_list_teams` 已是 MCP 工具（§4.5），走 `pkg/hub` 的真实实现；但本机 JWT 于 2026-07-29 过期，真实请求必 401，因此两个工具只在 `httptest` 假 Hub 上被验证过。**"能刷新过期 JWT" 这件事尚未在真机端到端证明**。
4. **未对生产 Hub 做过端到端验证**：本机凭据已于 2026-07-29 过期，任何真实请求都会 401。本次全部验证使用 `net/http/httptest`；`hub.stifer.xyz` **零访问**。
5. **`repo_url` 未上报**（留空）。
6. **旧联邦分支那一代 API 未复引入**：`hub.BindTeam` 与旧分支的 `StatusSnapshot` 语义没有回来；master 的等价物是 `BindNamespaceTeam` 与 `SnapshotForNamespace`（`StatusSnapshot` 类型本身是 master 自己的）。
7. **`pkg/engine` 不 import `pkg/hub`**，这条是硬约束，不是巧合。

---

## 8. 相关文档

- `docs/HUB_ALIGNMENT.md` — Hub 面 ↔ agentflow 面矩阵与真实完成度
- `docs/HUB_SOFT_SYNC.md` — namespace ↔ team 绑定模型（无网络的那一半）
- `pkg/hub/README.md` — 客户端实现细节
