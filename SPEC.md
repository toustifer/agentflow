# agentflow SPEC — 状态机扩展 & 技术调研流程

## 一、reassign 状态机扩展

### 现状

```
reassign 只能在 rework_needed → assigned
（reviewer 打回后 Leader 换人接手）
```

### 扩展后

```
executing → reassign → assigned     ← 新增：执行中换 Worker
review_pending → reassign → assigned ← 新增：review 中换 Worker
rework_needed → reassign → assigned  ← 已有
```

Leader 在执行中或 review 阶段发现 Worker 不合适，直接换人，无需 cancel 重建。

```go
// engine.go applyTransition — TransReassign 扩展
case TransReassign:
    if task.State != TaskReworkNeeded &&
       task.State != TaskExecuting &&
       task.State != TaskReviewPending {
        return "", ErrInvalidTransition
    }
    return TaskAssigned, nil
```

`AvailableTransitions` 同步更新：
- `executing` 状态：加 `reassign`（role: leader, "Reassign to a different Worker"）
- `review_pending` 状态：加 `reassign`（role: leader, "Reassign to a different Worker"）

## 二、技术调研流程（谁干活谁调研）

### 原则

Worker 是 Task 的 owner，交付责任不可转移。遇到技术障碍时：

```
Worker: "这个技术我没用过，需要研究一下"
  ↓
Worker 自研：利用 WebSearch / WebFetch 搜索方案
  ↓
Worker: "找到了方案，继续实现"
  ↓
task_transition submit
```

### Leader 的角色

Leader 不需要介入调研细节。只需：
1. 在 dispatch prompt 中明确：「遇到技术问题先用 WebSearch/WebFetch 自行调研」
2. 如果 Worker 多次提交失败且原因是技术方向不对 → 考虑 `reassign` 换人

### 什么时候用 reassign

- Worker 技术栈完全不匹配，学成本太高
- Worker 连续多次 rework 且方向错误
- Worker 自己请求换人（超出能力范围）

### 不需要 Explorer Worker

Claude Code Agent 原生具备 WebSearch / WebFetch 工具，Worker 子 agent 可直接使用。Leader 在 dispatch 时加上调研指令即可：

```
## Task: T2 — 登录页 UI

### Your Identity
你是 worker-fe，前端专家。

### Task
实现登录页面，包含 WebAuthn 生物识别登录。

### Methodology
- 如果不熟悉 WebAuthn API，先用 WebSearch 搜索最佳实践
- 调研结果作为实现方案的一部分交付
- 遇到技术障碍先自行搜索，搜索后仍无法解决再向 Leader 请求

### Acceptance Criteria
1. 登录页 UI 可用
2. WebAuthn 登录流程完整
3. 测试通过
```

## 三、代码改动

### engine.go

```go
// applyTransition — TransReassign 扩展源状态
case TransReassign:
    if task.State != TaskReworkNeeded &&
       task.State != TaskExecuting &&
       task.State != TaskReviewPending {
        return "", ErrInvalidTransition
    }
    return TaskAssigned, nil

// AvailableTransitions — executing/review_pending 加 reassign
case TaskExecuting:
    return []AvailableTransition{
        {Transition: "submit", Role: "worker", ToState: "review_pending"},
        {Transition: "cancel", Role: "leader", ToState: "cancelled"},
        {Transition: "reassign", Role: "leader", ToState: "assigned",
            Hint: "Reassign to a different Worker"},
    }
case TaskReviewPending:
    return []AvailableTransition{
        {Transition: "pass", Role: "reviewer", ToState: "done"},
        {Transition: "rework", Role: "reviewer", ToState: "rework_needed"},
        {Transition: "reassign", Role: "leader", ToState: "assigned",
            Hint: "Reassign to a different Worker"},
    }
```

### roleAllowedTransitions

`reassign` 已经允许 leader 角色使用，无需修改。
