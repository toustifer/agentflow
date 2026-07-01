# agentflow SPEC — DAG 驱动的 Worker 协作引擎

## 核心概念

```
Namespace (项目)
  ├── DAG-1 ("添加登录功能", branch: "feat/login")
  │     T1 (worker-auth) ──→ T2 (worker-fe)
  │     T3 (worker-auth) ──→ T4 (worker-qa)
  │                           T5 (worker-fe)  [并行]
  │
  ├── DAG-2 ("暗黑模式", branch: "feat/dark-mode")
  │     T6 (worker-fe) ──→ T7 (worker-qa)
  │
  └── Worker 注册表（全局，跨 DAG 共享）
        ├── worker-auth — 认证服务 (busy: T3 review_pending)
        ├── worker-fe   — 前端     (busy: T2 executing, T5 done)
        └── worker-qa   — 测试     (idle)
```

### 关键原则

1. **DAG 节点 = Task**，Task 之间有 `depends_on` 依赖
2. **每个 Task 有一个 `assigned_worker`**，多个 Task 可分配给同一 Worker（并行或串行）
3. **Worker 是全局实体**，不会因为出现在多个 DAG 中被复制
4. **Worker 状态从 Task 派生**：任一 DAG 中的任一 Task 处于非终态 → busy
5. **Worker busy 直到该 DAG 中最后一个 Task pass review**，不是 submit 就释放
6. **DAG 绑定 Git Branch**，一个 DAG = 一个功能/修改
7. **Review pass 后 Worker 写总结**到自己的目录

---

## 数据模型

### Namespace (项目)

```go
type Namespace struct {
    ID        string
    Name      string
    Status    string            // "active" | "archived"
    Metadata  map[string]string
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### Worker (全局注册表，状态从 Task 派生)

```go
type Worker struct {
    ID          string
    NamespaceID string
    Name        string
    Scope       string          // 领域描述
    Skills      []string        // 技术栈/能力标签
    Metadata    map[string]string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

Worker 的 `Status` 不存数据库，而是运行时计算：

```go
// 扫描所有 DAG 中分配給該 Worker 的 Task
// 任一 Task 处于 executing / review_pending / rework_needed → busy
// 全部终态 (done / cancelled) → idle
func (e *Engine) WorkerStatus(workerID string) WorkerStatus
```

### DAG (轻量容器，图结构从 Task 派生)

```go
type DAG struct {
    ID          string
    NamespaceID string
    Title       string          // 功能标题
    Branch      string          // 绑定的 git branch
    Status      DAGStatus       // "planning" | "in_progress" | "done" | "cancelled"
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

DAG 不显式存储节点和边。图结构完全从 Task 的 `dag_id` + `depends_on` 派生：
- 查询 `tasks WHERE dag_id = X` → 所有节点
- 每个 Task 的 `depends_on` → 边
- `dag_get` 自动构建完整图结构返回

### Task (节点 ∈ DAG)

```go
type Task struct {
    ID                 string
    DAGID              string           // 所属 DAG
    NamespaceID        string
    Title              string
    Description        string
    State              TaskState        // 同现有状态机
    AssignedWorker     string           // 引用 Worker ID
    DependsOn          []string         // Task ID 列表（同一 DAG 内）
    AcceptanceCriteria []string
    OutputFiles        []string
    EstimatedHours     float64
    ActualHours        float64
    Tags               []string
    Priority           int
    ReviewCycle        int
    CreatedAt          time.Time
    UpdatedAt          time.Time
    Metadata           map[string]string
}
```

---

## MCP 工具集

### Phase 1 — DAG + Worker 实体

| 工具 | 说明 | 角色 |
|------|------|------|
| `dag_create` | 创建 DAG（指定 title, branch, nodes, edges） | leader |
| `dag_get` | 查询 DAG 详情（含所有节点状态 + task 摘要） | 通用 |
| `dag_list` | 列出命名空间下所有 DAG | 通用 |
| `dag_report` | DAG 进度报告（各节点状态、阻塞、完成率） | 通用 |
| `worker_register` | 注册 Worker（name, scope, skills） | leader |
| `worker_list` | 列出 Worker（可按 status/skill 过滤） | 通用 |
| `worker_get` | 查询 Worker 详情 | 通用 |
| `worker_update` | 更新 Worker 信息 | leader |
| `task_create` | 升级：接受 `dag_id`、`worker_id`、`depends_on` 等 | leader |
| `task_query` | 按 DAG/Worker/state/tag/priority 过滤 | 通用 |

### Phase 2 — 完整 Worker 生命周期

| 工具 | 说明 | 角色 |
|------|------|------|
| `dag_transition` | 推进 DAG 节点状态（start / submit / pass / rework / resume） | 按角色 |
| `task_transition` | 现有，worker submit 后不释放 worker（worker 保持 busy） | 按角色 |
| `worker_summary_write` | Worker 在 review pass 后将总结写入指定目录 | worker |

### Phase 3 — 查询 & 可视化

| 工具 | 说明 |
|------|------|
| `dag_flowchart` | 输出 DAG 的流程图数据（节点 + 边 + 状态，可用于可视化渲染） |
| `project_timeline` | 项目时间线（DAG + Worker 历史） |
| `project_report` | 跨 DAG 项目报告 |

---

## 完整生命周期场景

### 场景：一个 DAG（"添加登录功能"）的全流程

```
── Phase 0: 项目初始化 ──

leader → namespace_create
leader → worker_register × 3（worker-auth, worker-fe, worker-qa）

── Phase 1: 创建 DAG + Task ──

leader → dag_create
  title: "添加登录功能"
  branch: "feat/login"

leader → task_create × 4
  T1: title: "用户表 + JWT 签发", assigned_worker: "worker-auth", dag_id: "dag-1"
  T2: title: "登录 API 页面",     assigned_worker: "worker-fe",  dag_id: "dag-1", depends_on: ["T1"]
  T3: title: "单元测试",          assigned_worker: "worker-qa",  dag_id: "dag-1", depends_on: ["T2"]
  T4: title: "登出功能",          assigned_worker: "worker-auth", dag_id: "dag-1"

# 此时 DAG 的图结构自动派生：
# worker-auth: T1 ──→ worker-fe: T2 ──→ worker-qa: T3
# worker-auth: T4（可并行，因为 T4 不依赖 T1）

── Phase 2: 执行 ──

leader → task_transition(T1, start, actor_role: "leader")
leader → Agent(worker-auth): "实现 T1, T4"
# worker-auth 全局状态自动变为 busy（T1 在 executing）

worker-agent → task_transition(T1, submit, actor_role: "worker")
worker-agent → task_transition(T4, submit, actor_role: "worker")
# worker-auth 仍为 busy（还有 task 在 review_pending）
# 但 T1 review_pending 不会阻塞 T4（T4 不依赖 T1）

leader → 审查代码
leader → task_transition(T1, pass, actor_role: "reviewer")
# worker-auth 依然 busy（T4 还在 review_pending）
leader → task_transition(T4, pass, actor_role: "reviewer")
# worker-auth 现在 idle ✅（本 DAG 所有 task done）
# T1 pass 触发 T2 变为 ready（依赖已满足）

worker-auth → 写总结（不通过 MCP，子 agent 自己写文件）

leader → task_transition(T2, start, actor_role: "leader")
leader → Agent(worker-fe): "实现 T2"

...循环直到 T3 done

── Phase 3: 完成 ──

# DAG 状态自动计算：所有 Task done → DAG done
leader → dag_get("dag-1")
→ { status: "done", tasks: [...], completion: 100% }
```

---

## Worker 生命周期（从 Task 派生）

```
idle ──→ (任一 task executing) ──→ busy
                                       │
                                  task submit → review_pending
                                       │
                                  reviewer pass → 该 task done
                                       │
                                  Worker 还有活跃 task? ──→ 保持 busy
                                       │
                                  全部 task done → idle ✅
```

**关键规则**：
- Worker 没有自己的状态机，状态完全从 Task 派生
- Worker `busy` 条件：任一 DAG 中任一 Task 处于 `executing / review_pending / rework_needed`
- Worker `idle` 条件：所有 DAG 中所有 Task 处于终态（`done / cancelled / assigned`）
- Worker 在 review pass 后应写总结到自己的目录（子 agent 自行写入文件系统，非 MCP 操作）

## 跨 DAG 阻塞推导（不用显式配置）

```
DAG-1: T1(worker-auth) ──→ T2(worker-fe)
DAG-2: T3(worker-fe)

worker-fe busy on DAG-1 (T2 executing) 时：
  - DAG-2 的 T3 虽然在 assigned 状态且无依赖
  - 但 engine 知道 worker-fe busy → project_next_tasks 不会推荐 T3
  - 引擎用服务端逻辑标记，不需要 Leader 自己推算
```

依赖分两层：
1. **显式依赖**：`Task.depends_on`，同一 DAG 内的任务依赖（T1→T2）
2. **隐式阻塞**：Worker 不可用导致的任务暂时不可调度（跨 DAG）

---

## 迭代路径

### Phase 1 — DAG + Worker 实体 + Task 扩展（核心架构）

**引擎改动**：
- `engine.go` — 新增 `DAG` 结构体（轻量容器）；`Worker` 结构体；Task 扩展 `DAGID`、`DependsOn`、`Tags`、`Priority`、`EstimatedHours`
- `store.go` — 新增 `dags`、`workers` SQLite 表 + Task 表加列迁移
- `dag.go` — DAG CRUD；`dag_get` 自动构建图结构（从 Task 查询 + depends_on 生成 edges）
- `worker.go` — Worker CRUD；Worker 状态运行时计算
- 循环依赖检测（Task 级别的 `depends_on`，同一 DAG 内）

**MCP 工具**：
- `dag_create` / `dag_get` / `dag_list`
- `worker_register` / `worker_get` / `worker_list` / `worker_update`
- `task_create` 升级（接受 `dag_id`, `depends_on`, `tags`, `priority`, `estimated_hours`）
- `task_query` 新增（按 DAG/Worker/state/tag 过滤，支持 `ready_only` 仅返回依赖已满足的任务）

**产出**：可以创建 DAG、注册 Worker、创建带依赖的任务、查询 DAG 图结构。

### Phase 2 — Worker 派生状态 + 跨 DAG 感知

**引擎改动**：
- 跨 DAG Worker 冲突检测：`project_next_tasks` 排除 Worker 不可用的任务
- 隐式阻塞标记：Task 虽然无依赖但 Worker 不可用 → 在 `available_transitions` 或 `blocked_by` 中标记原因
- DAG 状态自动计算：扫描 DAG 下所有 Task 状态聚合得出

**MCP 工具**：
- `dag_report`（各 Worker 任务进度、完成率）
- `project_next_tasks`（跨 DAG 推荐可执行任务，排除 Worker busy 的）
- `project_blockers`（显式依赖阻塞 + 隐式 Worker 阻塞，带阻塞原因）

**产出**：Leader 能问"下一步做什么"，引擎自动考虑依赖 + Worker 可用性。

### Phase 3 — DAG 查询 & 可视化

**引擎改动**：
- `dag_flowchart` — 输出格式化的流程图数据（节点 = Task，边 = depends_on，含状态和 Worker 标签）
- `project_report` — 跨 DAG 项目报告

**MCP 工具**：
- `dag_flowchart`
- `project_report` / `project_timeline`

**产出**：Leader 可以可视化 DAG、获取项目级别进度。

### Phase 4 — 批量操作

**MCP 工具**：
- `task_create_batch`（批量创建 Task + 依赖关系一次搞定）

### Phase 5 — Hub 同步 & 可观测性

- 实现 HubSyncer（DAG 状态 → Hub Dashboard）
- Worker 心跳
- 事件查询增强

## 非功能性原则

- **向后兼容**：现有 `task_create` 调用（无 dag_id/worker_id）继续可用
- **Worker 不可删除**：只能 merge/split，保证历史不丢失（参考 agent-company 的 worker 不可删除原则）
- **DAG 可取消**：cancelled 状态，保留结构供历史查询
- **SQLite 优先**：所有新实体走 SQLite 持久化，内存模式可选用于测试
