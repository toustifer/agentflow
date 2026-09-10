# Inspect Flow

这是 `agentflow` bundle 内部的 **inspect flow** 文档。

用途：当用户执行 `/agentflow inspect` 或 `/agentflow inspect <subview>` 时，主入口应按此 flow 推进，输出 **树状进度视图**，而不是平铺 JSON。

## 目标

优先把项目当前执行面恢复成 5 类可读视图：

- project：项目级摘要与 DAG 树
- dag：单 DAG 的 bucket / 依赖图
- task：单 task 的运行态与 lease/worktree 明细
- blockers：当前阻塞面
- workers：worker 负载面
- next：ready queue

## Step 1. 定位 namespace

优先复用 `resume` 的 namespace 恢复逻辑：

```text
mcp__agentflow__namespace_list(workdir_contains=<当前 cwd>)
```

若当前 sticky mode 的 `mode.json` 已有 `namespace_id`，可直接优先使用。

## Step 2. 拉统一快照

统一调用：

```text
mcp__agentflow__project_inspect(namespace_id, focus?, dag_id?, task_id?)
```

参数约定：

- `/agentflow inspect`
  - `focus="project"`
- `/agentflow inspect dag <dag_id>`
  - `focus="dag"`, `dag_id=<dag_id>`
- `/agentflow inspect task <task_id>`
  - `focus="task"`, `task_id=<task_id>`
- `/agentflow inspect blockers`
  - `focus="blockers"`
- `/agentflow inspect workers`
  - `focus="workers"`
- `/agentflow inspect next`
  - `focus="next"`

## Step 3. 渲染与输出规范

拿到 `project_inspect` 返回值后，不要直接平铺打印 JSON。

统一执行：

```text
mcp__agentflow__project_inspect(...)
-> node hooks/render-inspect.js
```

做两件事：
- 把 snapshot 写入 `.claude/agentflow/status.json`
- 按 focus 渲染成树状文本

### project 视图

先输出项目摘要：

- namespace_id / name
- phase / progress
- ready / running / blocked / done
- workers busy/total

然后按 DAG 输出树：

```text
<Project>
├─ DAG <dag_id> <title>
│  ├─ Ready
│  ├─ Running
│  ├─ Blocked
│  └─ Done
└─ ...
```

每个 task 必须显示：

```text
task_id + title
```

### dag 视图

先输出 DAG 摘要：

- dag_id + title
- execution_branch / base_branch
- completion_pct
- ready / running / blocked / done

如果 `dag_detail` 可用：
- 可额外展示轻量 depends_on 关系
- 不要一次把全量 graph JSON 原样打印出来

### task 视图

至少输出：

- task_id + title
- state
- assigned_worker
- blocked_by
- available_transitions
- worker_status
- branch / base_branch / worktree_path
- active_task_id / lease_holder_task_id / lease_holder_worker_id / lease_holder_agent_id

### blockers 视图

按 DAG 或 blocked_by 聚合输出当前 blocker 列表。

### workers 视图

按 worker 输出：

- worker_id + name
- status
- total_tasks / done_tasks（若有）
- 当前关联的执行任务（如果 inspect 快照里能推断）

### next 视图

只输出 ready queue，顺序按聚合返回顺序展示。

## Step 4. 更新 status 快照

每次 `resume` 或 `inspect` 产出快照后，都应把摘要写入：

```text
.claude/agentflow/status.json
```

供 `hooks/statusline.js` 读取并渲染两行状态。

快照至少应包含：

- `project`
- `summary`
- `dags`
- `workers`
- `blockers`
- `next_tasks`

## 结束条件

当且仅当以下条件满足时，本 flow 才算完成：

- 已解析要查看的子视图
- 已恢复 namespace
- 已调用 `project_inspect`
- 已输出树状结果
- 已刷新 `.claude/agentflow/status.json`（如果当前目录可写）
