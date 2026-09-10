# 原生 Backlog / Goal 储备池设计方案 (Backlog Goals Design)

## 1. 背景与目标 (Background & Goals)

在 Agentflow 现有的执行链路中，核心流转主要以立即执行为导向（`Intake -> Shape -> DAG -> Task`）。但在实际工程管理中，用户与 Leader 会频繁产生“暂不启动但在未来有价值”的目标（如技术债、需求池、待办灵感）。
若直接建 DAG 会污染执行拓扑和调度面；若存入散乱的 diary/doc 则缺乏一等公民的实体生命周期管理。

本特性旨在将 **Goal / Backlog 储备池** 作为 Agentflow 的一等公民（First-class Citizen）引入，实现执行面（DAG & Tasks）与规划面（Backlog Goals）的彻底解耦与闭环联动。

---

## 2. 数据模型与存储架构 (Data Model & Storage)

### 2.1 实体定义 (`pkg/engine/goal.go`)

```go
type GoalStatus string

const (
    GoalPending   GoalStatus = "pending"   // 活跃待办
    GoalDeferred  GoalStatus = "deferred"  // 挂起/延期
    GoalPromoted  GoalStatus = "promoted"  // 已物化为 DAG
    GoalDropped   GoalStatus = "dropped"   // 废弃/归档
)

type Goal struct {
    ID          string            `json:"id"`           // 短标识，如 "G-1", "G-2"
    NamespaceID string            `json:"namespace_id"` // 所属项目空间
    Title       string            `json:"title"`        // 目标标题
    Description string            `json:"description"`  // 详细描述
    Status      GoalStatus        `json:"status"`       // 状态 (pending/deferred/promoted/dropped)
    Priority    int               `json:"priority"`     // 优先级，数字越大越高，默认 0
    Tags        []string          `json:"tags"`         // 标签列表
    Context     string            `json:"context"`      // 补充背景上下文、相关 Issue/链接
    DAGID       string            `json:"dag_id"`       // 晋升物化后的 DAG ID
    Metadata    map[string]string `json:"metadata"`     // 扩展元数据
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### 2.2 存储设计 (`pkg/engine/store.go`)

在 SQLite 中增加 `goals` 表及关联索引：

```sql
CREATE TABLE IF NOT EXISTS goals (
    id           TEXT NOT NULL,
    namespace_id TEXT NOT NULL,
    title        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'pending',
    priority     INTEGER NOT NULL DEFAULT 0,
    tags         TEXT NOT NULL DEFAULT '[]',
    context      TEXT NOT NULL DEFAULT '',
    dag_id       TEXT NOT NULL DEFAULT '',
    metadata     TEXT NOT NULL DEFAULT '{}',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (namespace_id, id),
    FOREIGN KEY (namespace_id) REFERENCES namespaces(id)
);

CREATE INDEX IF NOT EXISTS idx_goals_ns_status ON goals(namespace_id, status);
```

### 2.3 ID 自动生成策略
- 若调用方未显式传递 `goal_id`，系统将在当前 namespace 中扫描现有匹配 `^G-(\d+)$` 格式的 ID，递增生成下一个短序号（如 `G-1`, `G-2`, `G-3`...）。
- 若调用方显式传递 `goal_id`，则校验其在 namespace 内唯一性。

---

## 3. 状态机与生命周期流转 (State Machine)

### 3.1 状态转移矩阵

| 当前状态 | 触发操作 | 目标状态 | 约束与副作用 |
|---|---|---|---|
| (None) | `goal_create` | `pending` | 创建新目标，默认放入待办储备池 |
| `pending` | `goal_update(status="deferred")` | `deferred` | 挂起/暂缓，不作为高优先级自动推荐 |
| `deferred` | `goal_update(status="pending")` | `pending` | 重新激活至活跃待办 |
| `pending` / `deferred` | `goal_update(status="dropped")` | `dropped` | 归档/放弃该目标 |
| `dropped` | `goal_update(status="pending")` | `pending` | 恢复废弃目标回待办 |
| `pending` / `deferred` | `goal_promote` | `promoted` | **原子触发**创建新 DAG，绑定 `dag_id`，锁定目标关系 |
| `promoted` | 任何状态修改 | - | **禁止修改**，已物化目标不可变更状态 |

---

## 4. MCP 工具接口契约 (MCP Tool Contracts)

### 4.1 `goal_create`
- **入参**：
  - `namespace_id` (string, 必填): 项目空间 ID
  - `title` (string, 必填): 目标简述
  - `description` (string, 可选): 详细背景与规格
  - `goal_id` (string, 可选): 显式指定 ID，缺省自动生成 `G-N`
  - `priority` (number, 可选): 默认 0
  - `tags` (string[], 可选): 默认 `[]`
  - `context` (string, 可选): 关联上下文
  - `metadata` (object, 可选): 默认 `{}`
- **返回**：`{ "goal": Goal }`

### 4.2 `goal_list`
- **入参**：
  - `namespace_id` (string, 必填)
  - `status` (string[], 可选): 默认 `["pending", "deferred"]`
  - `tags` (string[], 可选)
  - `priority_gte` (number, 可选)
- **返回**：
  ```json
  {
    "namespace_id": "...",
    "total": 2,
    "goals": [ Goal, ... ]
  }
  ```
  *(按 priority 降序、created_at 升序排列)*

### 4.3 `goal_get`
- **入参**：
  - `namespace_id` (string, 必填)
  - `goal_id` (string, 必填)
- **返回**：`{ "goal": Goal }`

### 4.4 `goal_update`
- **入参**：
  - `namespace_id` (string, 必填)
  - `goal_id` (string, 必填)
  - `title`, `description`, `status`, `priority`, `tags`, `context`, `metadata` (均为可选更新)
- **返回**：`{ "goal": Goal }`

### 4.5 `goal_promote`
- **入参**：
  - `namespace_id` (string, 必填)
  - `goal_id` (string, 必填)
  - `dag_id` (string, 可选): 默认 `dag-<goal_id>`
  - `dag_title` (string, 可选): 默认继承 `goal.title`
  - `execution_branch` (string, 可选): 默认 `feature/<dag_id>`
  - `base_branch` (string, 可选): 默认项目主分支
- **返回**：
  ```json
  {
    "goal": Goal,
    "dag": DAG,
    "next_steps": [
      "已为目标 G-1 创建执行 DAG 'dag-G-1'",
      "请调用 task_create / task_create_batch 为该 DAG 编排任务拓扑"
    ],
    "actions": ["task_create", "task_create_batch"]
  }
  ```

---

## 5. 联动视图呈现 (Inspect & NextSteps Integration)

### 5.1 `project_inspect`
- `summary` 中增加 `backlog_count`（统计 pending + deferred 目标总数）。
- 顶层增加 `backlog` 概览数组（包含未物化目标的 `id`, `title`, `status`, `priority`, `tags`）。
- 支持 `focus: "backlog"`，展开详细列表。

### 5.2 `project_next_steps`
- 当活跃 DAG 全部任务完成（phase: `done`）时，检查 Backlog 储备池：
  - 若存在待办目标，在 `next_steps` 中提示：“当前 DAG 已结案。Backlog 储备池中有 N 个待办目标可供评估晋升（推荐最高优先级 G-X: '...'）”。
  - 在 `actions` 中追加 `["goal_promote", "goal_list"]`。
- 当项目处于等待拆解 DAG（phase: `plan` 且 dags 为空）时，若 Backlog 有目标，推荐通过 `goal_promote` 直接激活。

---

## 6. 测试与质量保障 (Testing & Verification)

1. **Engine 单元测试 (`pkg/engine/goal_test.go`)**：
   - Goal 创建、自动短 ID 生成 (`G-1`, `G-2`)、查询过滤与更新；
   - 状态流转限制（`promoted` 锁定校验、非法状态转移拦截）；
   - `PromoteGoal` 原子性测试：验证 DAG 成功创建、元数据反查绑定及 Goal 状态跃迁；
   - SQLite 持久化与重启复原测试。
2. **Server/MCP 契约测试 (`pkg/server/goal_mcp_test.go`)**：
   - 测试 `goal_create`, `goal_list`, `goal_get`, `goal_update`, `goal_promote` RPC 调用；
   - 测试 `project_inspect` 中的 `backlog` 字段与 `backlog_count` 聚合；
   - 测试 `project_next_steps` 在 DAG 结案后的智能 Backlog 推荐文案与 actions。
