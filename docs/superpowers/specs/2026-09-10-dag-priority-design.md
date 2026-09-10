# DAG 优先级 (P0-P3) 设计方案 (DAG Priority Level Design)

## 1. 背景与目标 (Background & Goals)

随着项目内功能拓展和多个 DAG 分支的演进，团队常需要并行规划和维护多个不同紧迫程度的执行拓扑。
为了保证关键主链与紧急修复任务优先推进，需要支持对 DAG 显式设置与动态调整优先级。

用户既可手动指定优先级等级，也可委托 Agent/Leader 根据上下文与影响面自主估量并赋予等级。系统以通行的 **P0 ~ P3** 四个优先级为核心基准，并在调度、恢复、排序全链路打通。

---

## 2. 优先级等级规范 (Priority Levels)

| 等级 | 标识 | 语义 | 权重分值 | 适用场景 |
|---|---|---|---|---|
| **P0** | `"P0"` | 最高 / 紧急 (Critical/Blocker) | 100 | 线上严重故障、阻塞性缺陷、最高顺位优先执行任务 |
| **P1** | `"P1"` | 高 (High) | 50 | 核心业务主流程、关键 Milestone 交付 |
| **P2** | `"P2"` | 中 / 常规 (Normal, **默认值**) | 20 | 日常迭代功能、常规特性开发 |
| **P3** | `"P3"` | 低 (Low) | 0 | 次要优化、非紧急技术债、备选实验分支 |

- **大小写兼容**：输入时兼容小写（如 `"p0"` 自动规范化存储为 `"P0"`）。
- **缺省与容错**：未显式传入时默认分配为 `"P2"`。

---

## 3. 核心数据模型与存储 (Data Model & Storage)

### 3.1 实体定义 (`pkg/engine/dag.go`)

```go
type DAGPriority string

const (
    DAGPriorityP0 DAGPriority = "P0"
    DAGPriorityP1 DAGPriority = "P1"
    DAGPriorityP2 DAGPriority = "P2"
    DAGPriorityP3 DAGPriority = "P3"
)

type DAG struct {
    ID                  string            `json:"id"`
    NamespaceID         string            `json:"namespace_id"`
    Title               string            `json:"title"`
    Priority            DAGPriority       `json:"priority"` // "P0" | "P1" | "P2" | "P3"
    ExecutionBranch     string            `json:"execution_branch"`
    // ...其余字段保持不变
}
```

### 3.2 权重与比较辅助函数

```go
func DAGPriorityWeight(p DAGPriority) int {
    switch p {
    case DAGPriorityP0:
        return 100
    case DAGPriorityP1:
        return 50
    case DAGPriorityP2:
        return 20
    case DAGPriorityP3:
        return 0
    default:
        return 20 // 默认等同于 P2
    }
}
```

### 3.3 SQLite 表结构与迁移 (`pkg/engine/store.go`)

在 `schemaSQL` 的 `dags` 表中加入：
```sql
priority TEXT NOT NULL DEFAULT 'P2'
```

在 `migrateDAGsTable` 中增加迁移检查：
```go
"priority TEXT NOT NULL DEFAULT 'P2'"
```
并在 `insertDAG`, `updateDAG`, `loadDAGs` 中持久化与读取该字段。

---

## 4. MCP 工具入参与接口契约 (MCP Tool Contracts)

### 4.1 `dag_create`
- **新增入参**：`priority` (string, 可选, 默认 `"P2"`)
  - 允许值：`"P0"`, `"P1"`, `"P2"`, `"P3"`（大小写不敏感）
  - 若传入非法字符串，返回清晰的输入校验错误。
- **出参**：返回的 `dag` 对象包含 `"priority": "P0"` 等字段。

### 4.2 `dag_update`
- **新增入参**：`priority` (string, 可选)
  - 允许随时将 DAG 升降级（例如遇到突发事故升级为 `"P0"`）。

### 4.3 `goal_promote`
- **新增入参**：`priority` (string, 可选)
  - 若显式指定则采纳；若未指定且目标关联了非零 Goal Priority，支持按数值平滑映射（>=100 为 P0，>=50 为 P1，>=20 为 P2，其余 P3），缺省为 `"P2"`。

---

## 5. 调度、排序与视图联动 (Scheduling & Inspect Integration)

### 5.1 候选推荐与恢复权重 (`orderResumeCandidates`)
在 `pkg/server/legacy_dag_policy.go` 中，当存在多个非结案 DAG 时，按以下顺序排序推荐：
1. **Priority 权重（P0 > P1 > P2 > P3）**；
2. **执行状态**：`DAGInProgress` 优先于非活跃状态；
3. **最近更新时间**：`UpdatedAt` 降序；
4. **DAG ID**：字母序确定性保底。

### 5.2 全景视图呈现 (`project_inspect` & `dag_list`)
- `dag_list` 和 `project_inspect` 中的 `dags` 列表按 `Priority` 降序呈现，使 Leader 和开发者一目了然当前最高优先级的执行拓扑。

---

## 6. 测试策略与质量保证 (Testing Strategy)

1. **Engine 单元测试 (`pkg/engine/dag_priority_test.go`)**：
   - 验证 `CreateDAG` 默认赋予 `"P2"`，显式指定 `"P0"`/`"P1"` 成功规范化；
   - 验证非法 priority 输入报错；
   - 验证 `UpdateDAG` 修改优先级成功；
   - 验证 SQLite 持久化与重开引擎后优先级依然完好；
   - 验证 `ListDAGs` 按 Priority 权重降序排序。
2. **Server MCP 契约测试 (`pkg/server/dag_priority_mcp_test.go`)**：
   - 测试 `dag_create` / `dag_update` 支持 `priority` 参数；
   - 测试 `goal_promote` 支持设置 DAG 优先级；
   - 测试 `project_inspect` 与 `pickResumeDAG` 优先推荐高优先级（如 P0）DAG。
