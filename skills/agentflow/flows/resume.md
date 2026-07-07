# Resume Flow

这是 `agentflow` bundle 内部的 **resume flow** 文档。

用途：当用户执行 `/agentflow resume`，或 `/agentflow` 在当前目录下发现已有 namespace/project 上下文时，主入口应按此 flow 推进。

## Step 1. 定位项目

```text
mcp__agentflow__namespace_list(workdir_contains=<当前 cwd>)
```

- 找到：拿 `namespace_id`
- 没找到：提示用户当前不在任何 agentflow 项目目录下，应改用 `/agentflow goal [目标]`

## Step 2. 恢复上下文

优先恢复三类事实：

```text
mcp__agentflow__leader_diary_list(namespace_id)
mcp__agentflow__dag_report(namespace_id, dag_id)
mcp__agentflow__worker_list(namespace_id)
```

关注点：
- 最近的关键决策和状态
- DAG 进度
- Worker 注册和忙闲情况
- 是否存在执行中但可能失联的 task

如果 namespace 下有 `executing` task：
- 先告知用户
- 再决定是继续等、重派、还是手动介入

## Step 3. 恢复推进

```text
mcp__agentflow__project_next_tasks(namespace_id)
```

- 有 ready task：按 goal flow 的 dispatch 模板继续派发
- 没有 ready task 但项目未完成：说明阻塞原因
- DAG / 项目全部完成：问用户是否要加新功能；如果要加，先回到 `intake`，再视结论切回 goal flow

## 与 Goal Flow 的关系

resume flow 不负责重新出 shape，除非：
- 当前项目缺 `.claude/PROJECT_FINAL_SHAPE.md`
- 或用户明确要求重做产品边界
- 或用户要引入一个新需求，且 intake 判断应重新进入 shape

否则 resume 的职责只是恢复并继续推进已有项目。
