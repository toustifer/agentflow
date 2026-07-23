# Hub ↔ agentflow 同步内容契约（Sync Contract）

> **状态：** 2026-07-21 草案 v0.1 — **先定内容，再谈展示**  
> **原则：** Hub 是公司协作中心；同步 = 业务对象投影（event-driven UPSERT），**不是**整库复制。  
> **源：** agentflow SQLite 为执行真相；Hub PG 为团队可见镜像。  
> **写策略：** soft-fail，永不阻塞本地任务。

相关：

| 文档 | 关系 |
|------|------|
| [AGENTFLOW_ALIGNMENT.md](./AGENTFLOW_ALIGNMENT.md) | Tab / API 对齐矩阵与路线图 A0–A6 |
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Hub 表结构与鉴权 |
| 本文 | **只规定「同步什么字段」**；不规定 Dashboard 怎么画 |

---

## 0. 总原则

```text
协作问题（谁在干什么 / 卡在哪 / 谁占分支 / 有没有经验）→ 进 Hub
执行细节（worktree 路径 / BT 状态机 / 全文 diary / 大 diff）→ 留 agentflow
```

| 规则 | 说明 |
|------|------|
| **投影，非整库** | 只 UPSERT 协作字段；不推 SQLite 文件、不推 schema |
| **事件驱动** | 状态变化时写；禁止定时全量 dump |
| **方向默认 L→H** | agentflow → Hub；Hub → agentflow 仅人侧动作（invite 等） |
| **源真相** | 任务生命周期 / worker 注册：agentflow；成员 / 审批：Hub |
| **幂等** | 同一 `business_id + 业务主键` 可重复 UPSERT |
| **soft-fail** | Hub 不可达 → 记 note，任务继续 |
| **体量上限** | 单条 content ≤ 8KB 建议；事件 payload ≤ 4KB；超限截断或 skip |

---

## 1. 对象总览

| # | 对象 | 方向 | 优先级 | 现网状态 | Hub 表 |
|---|------|------|--------|----------|--------|
| 1 | **Members / Approvals** | Hub-native | — | ✅ 已有 | memberships / invites / link_requests |
| 2 | **Branches + Bindings** | L→H | P0 | ✅ 自动 soft-report | hub_branches / hub_branch_bindings |
| 3 | **Tasks（DAG 投影）** | L→H | P0 | 🟡 API 有，agentflow 未自动写 | hub_dag_state |
| 4 | **Task lifecycle events** | L→H | P1 | ❌ | hub_events |
| 5 | **Workers 在线态** | L→H | P1 | ❌（需 heartbeat） | hub_workers |
| 6 | **Workers 角色卡（摘要）** | L→H | P2 | 🟡 SyncWorkers 有，未接 agentflow | hub_workers (+ handbook jsonb) |
| 7 | **Locks** | L→H | P2 | ❌（跨机时才需要） | hub_locks |
| 8 | **Playbooks（精选）** | L→H | P2 | 🟡 MCP 可写，未自动 | hub_playbooks |
| 9 | **DAG 元信息（可选）** | L→H | P3 | ❌ 无独立表 | 可塞 metadata / 未来扩展 |
| 10 | **Docs / Diary / Handbook 全文** | — | **永不整库** | — | 仅可选精选条目走 playbook |

---

## 2. 不进同步（Never-sync）

以下 **永远不** 作为常规同步内容（避免变相整库复制）：

| 类别 | 例子 | 原因 |
|------|------|------|
| 本地路径 | `worktree_path`, absolute path, `DBPath` | 跨机无意义且泄漏环境 |
| BT / 调度内部 | behavior tree 节点态、lease 细节全文 | 执行器私有 |
| 大文本过程体 | `ProjectDoc.content` 全文、`WorkerDiary` 全文、`LeaderDiary` 全文 | 体积大；协作用摘要/事件即可 |
| 提示词本体 | `Worker.PromptTemplate` 全文 | 可能含密钥/内网假设；社区发布另走去敏通道 |
| Git 实体 | 完整 diff、blob、pack | 真相在 GitHub/origin |
| 命名空间内部 ID 映射 | 仅本机有意义的临时 agent session id | 除非出现在事件 actor |
| SQLite / 文件镜像 | `docs_sync` 全量、MANIFEST 整树 | 备份通道，非团队主路径 |
| 密钥 | token、api_key、设备码 | 永不经 sync payload |

**可选例外（人工/显式工具，非自动）：**

- 精选 knowledge/pitfall → `hub_playbooks`（category 约束）
- 社区市场发布 worker（已有去敏流水线，独立产品路径）

---

## 3. 逐对象字段契约

### 3.1 Members / Approvals — **Hub 原生，不同步自 agentflow**

| 字段 / 动作 | 源 | 说明 |
|-------------|-----|------|
| membership role | Hub | owner / admin / member |
| invite email + token | Hub | 人侧 |
| link-request approve/deny | Hub | 人侧 |

agentflow **不读不写** 这些表。Dashboard Members / Approvals Tab 只看 Hub。

---

### 3.2 Branches + Bindings — **已实现（A0）**

**触发：** `task_prepare_start` / `start` / `submit`（agentflow soft-report）

**API：** `POST /v1/hub/repos/:code/branches/report`（JWT 优先）

#### Branch 投影

| Hub 字段 | 来源 | 必填 |
|----------|------|------|
| `name` | DAG `execution_branch` 或 task 关联 branch | ✅ |
| `tip_sha` | `head_sha` | 建议 |
| `source` | 固定 `"report"`（GitHub refresh 另写 `github_api`/`ls_remote`） | ✅ |
| `repo_url` | 可选，请求级 | 可选 |

#### Binding 投影

| Hub 字段 | 来源 | 必填 |
|----------|------|------|
| `bind_type` | `dag` \| `task` \| `worker` \| `user` | ✅ |
| `bind_id` | dag_id / task_id / worker_id | ✅ |
| `branch_name` | 同上 branch | ✅ |
| `head_sha` | 当前 tip | 建议 |
| `worktree_host` | `os.Hostname()`（主机名，**不是**路径） | 建议 |
| `status` | `active` 等 | 建议 |
| `reporter` | 请求级 actor | 可选 |

**不报：** worktree 绝对路径、完整 git status、untracked 列表。

---

### 3.3 Tasks（DAG 投影）— **P0 待接 agentflow 自动写（A1）**

**真相：** agentflow `engine.Task`  
**镜像表：** `hub.hub_dag_state`  
**API：** `POST /v1/hub/businesses/:code/dag/sync`（JWT + membership）  
**语义：** 单任务 UPSERT（现 handler 一次一条）

#### 字段映射

| Hub `hub_dag_state` | agentflow `Task` | 同步？ | 备注 |
|---------------------|------------------|--------|------|
| `task_id` | `ID` | ✅ 必填 | 业务主键（+ business_id） |
| `title` | `Title` | ✅ | |
| `status` | `State` | ✅ | **枚举见下** |
| `assigned_worker` | `AssignedWorker` | ✅ | |
| `depends_on` | `DependsOn` | ✅ | text[] |
| `output_files` | `OutputFiles` | ✅ | 相对路径列表即可；勿扩成内容 |
| `branch` | DAG.ExecutionBranch 或 metadata git.branch | ✅ 建议 | 与 branch 索引对齐 |
| `head_sha` | DAG.HeadSHA / metadata | ✅ 建议 | |
| `updated_at` | 服务端 now() | 自动 | |

#### 建议扩展（契约预留，表可后续 migration）

| 逻辑字段 | agentflow | 同步？ | 说明 |
|----------|-----------|--------|------|
| `dag_id` | `DAGID` | 建议 P1 | 现表无列 → 可先放事件 payload 或加列 |
| `priority` | `Priority` | 可选 | |
| `tags` | `Tags` | 可选 | |
| `review_cycle` | `ReviewCycle` | 可选 | |
| `namespace_id` | `NamespaceID` | **默认不同步** | 本机概念；多机同 business 时可用 metadata 约定 |

#### 明确不同步的 Task 字段

| agentflow 字段 | 原因 |
|----------------|------|
| `Description` 全文 | 可能很长；需要时用事件摘要或 playbook |
| `AcceptanceCriteria` 全文数组 | 体积；协作看 status 即可 |
| `EstimatedHours` / `ActualHours` | 非协作刚需；可后续 |
| `WorkerAgentID` | 会话级 |
| `Metadata` 整包 | 只抽 `git.branch` / `git.head_sha` 等白名单键 |
| `CreatedAt` 强制覆盖 | Hub 自有 created_at |

#### Status 枚举对齐

| agentflow `TaskState` | Hub `status` 建议原样 | 展示语义（以后） |
|-----------------------|----------------------|------------------|
| `assigned` | `assigned` | 待开工 |
| `executing` | `executing` | 进行中 |
| `review_pending` | `review_pending` | 待审 |
| `rework_needed` | `rework_needed` | 返工 |
| `done` | `done` | 完成 |
| `cancelled` | `cancelled` | 取消 |

**禁止** 把 Hub 旧 agent-company 自由字符串与 agentflow 枚举混用而不文档化；新写入统一用上表。

#### 触发时机（A1 实现时）

| agentflow 动作 | 是否 sync task 行 |
|----------------|-------------------|
| `task_create` / batch | ✅ UPSERT（status=assigned） |
| `task_transition` start | ✅ status=executing + branch/sha |
| submit | ✅ review_pending |
| pass / rework / cancel / reassign / resume | ✅ 对应 status + assigned_worker |
| prepare_start | 可选（若已有 task 行则刷新 branch） |

与 branch report **可同事务语义上解耦**：两次 soft HTTP 即可。

---

### 3.4 Task lifecycle events — **P1（A2）**

**表：** `hub.hub_events`（append-only）  
**API：** `POST` append event（现偏 API Key；契约要求 **JWT 也可写** 后再自动接）

#### 标准 event_type（agentflow → Hub）

| `event_type` | 何时 | `actor` | `payload` 键（均短） |
|--------------|------|---------|----------------------|
| `task.created` | create | leader/system | `task_id`, `title`, `dag_id?`, `assigned_worker?` |
| `task.started` | start | worker_id | `task_id`, `from`, `to`, `branch?` |
| `task.submitted` | submit | worker_id | `task_id`, `review_cycle?` |
| `task.passed` | pass | reviewer | `task_id` |
| `task.rework` | rework | reviewer | `task_id`, `reason?`（≤500 字） |
| `task.reassigned` | reassign | leader | `task_id`, `from_worker`, `to_worker` |
| `task.cancelled` | cancel | actor | `task_id`, `reason?` |
| `task.resumed` | resume | worker | `task_id` |
| `branch.reported` | report 成功后可选 | reporter | `branch`, `tip_sha`（可省略，避免噪声） |
| `worker.online` / `worker.offline` | heartbeat 边沿 | worker_id | `host?` |
| `lock.acquired` / `lock.released` / `lock.conflict` | 锁操作 | worker_id | `resource_key` |

#### payload 禁止

- 完整 description / diary / diff
- 绝对路径
- token / 密钥

本地 `engine.Event`（FromState/ToState/Transition）是生成上述事件的源；**不必**把本地 history 表逐行复制。

---

### 3.5 Workers 在线态 — **P1（A3）**

**表：** `hub.hub_workers`  
**API：** Heartbeat（现 API Key；契约：**JWT + membership 应可写**）

| Hub 字段 | agentflow 来源 | 同步？ |
|----------|----------------|--------|
| `worker_id` | `Worker.ID` | ✅ |
| `version` | agentflow / skill 版本字符串 | ✅ 建议 |
| `status` | 由 heartbeat 推为 online；超时 Hub 标 stale/offline | ✅ |
| `last_heartbeat_at` | 服务端 now | ✅ |
| `host` | hostname | ✅ |
| `pid` | 可选 | 可选 |

**触发：** worker 进程/会话周期心跳（如 30–60s）或 task start/submit 时顺便 heartbeat。

**不算在线态、默认不同步：** `PromptTemplate`、`Skills` 全量、`RecoveryPolicy` 全文（见 3.6 摘要）。

---

### 3.6 Workers 角色卡摘要 — **P2**

用于「团队有哪些角色」，不是心跳。

| 逻辑字段 | agentflow | Hub 落点 | 同步？ |
|----------|-----------|----------|--------|
| `worker_id` | ID | hub_workers.worker_id | ✅ |
| `name` | Name | 扩展列或 handbook jsonb | 建议 |
| `kind` | Kind | 同上 | 建议 |
| `scope` | Scope | scope 列（SyncWorkers 已有） | 建议 |
| `skills` | Skills | jsonb 摘要 | 可选 |
| `task_tags` | TaskTags | 可选 | 可选 |
| `prompt_template` | PromptTemplate | **默认不同步** | 社区发布另议 |
| handbook knowledge/pitfalls 全文 | Handbook | **不自动**；精选 → playbooks | |

**API 参考：** 现有 `SyncWorkers`（偏 agent-company 批量）；agentflow 可改为「注册/更新时单 worker UPSERT」。

---

### 3.7 Locks — **P2，仅跨机协作时（A4）**

**表：** `hub.hub_locks`  
**语义：** 跨机器互斥（文件/资源），**不是** agentflow DAG lease 的完整镜像。

| Hub 字段 | 含义 | 同步？ |
|----------|------|--------|
| `resource_key` | 逻辑资源（建议 `repo:path` 或约定 key，**非**本机绝对路径） | ✅ |
| `holder_worker_id` | 持有者 | ✅ |
| `holder_token` | 租约 token | ✅（协议需要） |
| `acquired_at` / `expires_at` / `heartbeat_at` | TTL | ✅ |

**agentflow DAG lease 字段**（`LeaseHolderTaskID` 等）：

- **默认同机执行 → 不同步到 hub_locks**
- 若未来「跨机抢同一 execution branch」→ 另定 `resource_key` 规范，再映射

单机 dev：**可以不上报锁**。

---

### 3.8 Playbooks — **P2 精选 opt-in（A5）**

**表：** `hub.hub_playbooks`  
**不是** handbook/diary 镜像。

| Hub 字段 | 要求 |
|----------|------|
| `category` | 约定：`patterns` \| `gotchas` \| `decisions` \| `pitfalls` \| `knowledge` |
| `title` | ≤256 |
| `content` | 建议 ≤8KB；超限截断 |
| `tags` | 可选 |
| `created_by_worker_id` | worker_id |
| `business_id` | 团队内；NULL 仅跨业务社区场景 |

**写入条件（自动路径若做）：**

- 显式 `promote_to_hub` / leader 批准，或  
- 标签 `hub:share`，或  
- 人工 MCP `hub_create_playbook`

**禁止：** 每次 diary_write / doc_write 自动推全文。

---

### 3.9 DAG 元信息 — **P3 可选**

| 字段 | 同步？ | 说明 |
|------|--------|------|
| `dag_id` + `title` + `status` | 建议最终有 | 现无独立 Hub 表；可先靠 tasks 聚合 |
| `execution_branch` / `base_branch` / `head_sha` | 经 branch + task.branch | 已有通道 |
| `worktree_path` / lease 细节 | ❌ | never |
| `active_task_id` | 可选 | 可作 event 或 dag 摘要列 |

---

## 4. 协作问题 → 数据覆盖（验收用）

| 协作问题 | 依赖对象 | 字段是否已在契约内 |
|----------|----------|-------------------|
| 现在有哪些任务、谁做、什么状态？ | Tasks | ✅ 3.3 |
| 任务依赖卡在哪？ | Tasks.depends_on + status | ✅ 3.3 |
| 谁在线 / 谁闲？ | Workers heartbeat | ✅ 3.5 |
| 谁占着哪条分支？ | Branches + Bindings | ✅ 3.2 |
| 最近发生了什么？ | Events | ✅ 3.4 |
| 有没有可复用的坑/经验？ | Playbooks | ✅ 3.8（精选） |
| 跨机是否撞文件？ | Locks | ✅ 3.7（按需） |
| 谁是团队成员、谁待批？ | Members/Approvals | ✅ 3.1 Hub 原生 |

**不靠同步回答的问题（本机 inspect）：**

- 某 task 的 BT 跑到哪一步  
- 某 worktree 脏文件列表  
- 某日 diary 原文  

---

## 5. 触发矩阵（实现清单，非 UI）

| 优先级 | 内容 | agentflow 挂载点（建议） | Hub API | 鉴权 |
|--------|------|--------------------------|---------|------|
| **A0 已做** | Branch + binding | prepare_start / start / submit | `branches/report` | JWT |
| **A1** | Task 行 UPSERT | create + 每次 transition | `dag/sync` | JWT |
| **A2** | lifecycle event | 同 transition 后 | `events` append | JWT（需放开） |
| **A3** | Worker heartbeat | 会话周期 / start | `heartbeat` | JWT（需放开）或 Key |
| **A4** | Lock | 仅跨机资源 API | locks/* | Key/JWT |
| **A5** | Playbook 精选 | 显式 promote | playbooks create | JWT/Key |
| **A6** | 机器 Key UI | — | api-keys | 人侧 admin |

每条写路径：**超时短（≤5s）、失败只 note、可降级关闭**（env 如 `HUB_SYNC=0`）。

---

## 6. 体量与频率建议

| 对象 | 频率 | 合并策略 |
|------|------|----------|
| Task UPSERT | 每次状态变更 1 次 | 同 task 覆盖写 |
| Branch report | prepare/start/submit | 同 tip 可重复 |
| Event | 每次 transition 1 条 | append，不改历史 |
| Heartbeat | 30–60s 或边沿 | UPSERT worker 行 |
| Playbook | 人工/稀有 | title+category 去重（已有 upsert 语义则跟服务） |
| Lock | 获取/续约/释放 | 行级 |

---

## 7. 与「整库复制」的边界声明

| 做法 | 本契约 |
|------|--------|
| 复制 SQLite 文件到服务器 | ❌ |
| 镜像全部表行（docs/diary/history/BT） | ❌ |
| 业务对象字段 UPSERT + 事件 append | ✅ |
| 看起来像「多表同步」 | ✅ 但是 **白名单字段** 的多对象投影 |

若未来某协作问题必须读长文，优先：

1. 链到 GitHub / 文档 URL，或  
2. 单条 promote → playbook，  
而不是打开 docs 全量同步。

---

## 8. 开放问题（定内容时需你拍板）

下列不影响「大类」，但影响 A1 字段是否加列：

1. **`dag_id` 是否进 `hub_dag_state`？** 建议：是（migration 加列），否则多 DAG 团队难筛。  
2. **`namespace_id` 是否上云？** 建议：否；用 `business_code` 做团队边界，本机 namespace 仅执行器。  
3. **Description 是否同步前 200 字摘要？** 建议：v1 不同步，需要再加 `summary` 可选字段。  
4. **Heartbeat / Events 是否必须先改 JWT 可写再接 agentflow？** 建议：是（与人侧无 Key 主路径一致）。  
5. **多机同一 `task_id` 冲突？** 约定：`task_id` 在 business 内全局唯一（leader 派号）；后写覆盖并打 event。

---

## 9. 修订流程

1. 改本文字段表 → 再改 `AGENTFLOW_ALIGNMENT.md` 路线图状态  
2. 再实现 agentflow soft-sync 与（如需）Hub migration/JWT 写  
3. **最后** 才改 Dashboard Tab 展示  

**未进入本文白名单的字段，默认禁止当同步内容实现。**

---

## 10. 一页纸清单（给实现者）

**Must sync（P0–P1）**

- [x] branch name, tip_sha, bindings(bind_type/id, host名)  
- [ ] task_id, title, status, assigned_worker, depends_on, output_files, branch, head_sha  
- [ ] events: task.* 短 payload  
- [ ] worker_id + heartbeat(host, version, online)

**Should sync（P2）**

- [ ] worker name/kind/scope 摘要  
- [ ] 跨机 lock resource_key  
- [ ] 精选 playbook

**Never**

- [ ] paths, BT, full docs/diary, prompt 全文, diffs, secrets, SQLite 文件
