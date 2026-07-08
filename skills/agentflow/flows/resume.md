# Resume Flow

这是 `agentflow` bundle 内部的 **resume flow** 文档。

用途：当用户执行 `/agentflow resume`，或 `/agentflow` 在当前目录下发现已有 namespace/project 上下文时，主入口应按此 flow 推进。

新绑定成功的已有内容项目，如果已经拥有 phase / DAG / task 上下文，也可以从 `init` 直接切入本 flow。

目标不是假装自己是 Claude 内建 `/resume`，而是尽量做到同样的体验顺序：

```text
先恢复项目现场
-> 先看到当前有哪些 DAG
-> 再看到系统建议继续哪条线
-> 然后继续推进
```

## Step 1. 定位项目

```text
mcp__agentflow__namespace_list(workdir_contains=<当前 cwd>)
```

处理规则：
- 找到 0 个：提示当前目录不在任何 agentflow 项目中，应改用 `/agentflow goal [目标]`
- 找到 1 个：直接拿 `namespace_id`
- 找到多个：
  - 先展示候选 namespace
  - 优先选与当前 cwd 最匹配的一项
  - 如果仍不明确，再向用户确认

## Step 2. 先恢复项目级 snapshot

resume 的首屏优先读这些事实：

```text
mcp__agentflow__project_next_steps(namespace_id)
mcp__agentflow__project_report(namespace_id)
mcp__agentflow__dag_list(namespace_id)
```

先回答：
- 当前项目处于什么 `phase`
- 当前系统推荐的 `next_steps / actions` 是什么
- 一共有多少 DAG、多少完成、多少仍在推进
- 当前有哪些非完成 DAG

这一步的输出应先是 **项目摘要**，不是直接钻进某条 task。

## Step 3. 输出 DAG 列表

resume 默认要先展示一个紧凑 DAG 列表，优先列 **非完成 DAG**：
- `dag_id`
- `title`
- `branch`
- `status`
- 如果容易拿到，再补 `progress / completion`

建议顺序：
- `in_progress`
- `planning`
- `blocked / cancelled`（如果有）
- `done` 放最后，或只做汇总不展开

如果只有 1 条 active DAG：
- 自动聚焦它

如果有多条 active DAG：
- 先展示列表
- 再给出“推荐继续项”

## Step 4. 再补决策痕迹与执行面

在项目摘要和 DAG 列表之后，再恢复：

```text
mcp__agentflow__leader_diary_list(namespace_id)
mcp__agentflow__worker_list(namespace_id)
```

必要时才继续读：

```text
mcp__agentflow__leader_diary_get(...)
mcp__agentflow__dag_get(namespace_id, dag_id, with="graph")
mcp__agentflow__project_next_tasks(namespace_id)
```

关注：
- 最近有没有 intake / shape / priority / defer 决策
- 当前有没有 `executing` task
- worker 当前定义和状态是否完整
- 哪条 DAG 更像当前主线
- 当前 ready task 是哪些
- 如果没有 ready task，阻塞原因是什么

注意：
- `project_next_tasks` 是补充事实，不应取代 DAG 列表成为首屏
- `dag_get(..., with=graph)` 只在依赖关系不清楚时再读，不要一上来全展开

## Step 5. 给出 resume 决策

resume 至少要收敛出：
- `namespace_id`
- `phase`
- `active_dags`
- `recommended_dag_to_continue`（如果能判断）
- `recommended_next_action`
- `blocking_reason`（如果当前不能继续）

### 只有 1 条 active DAG

动作：
- 自动聚焦这条 DAG
- 告诉用户当前 phase、当前 DAG、下一步建议
- 默认按 `leader_tick` 继续推进

### 有多条 active DAG

动作：
- 先展示 DAG 列表
- 默认推荐一条最值得继续的 DAG
  - 优先依据 `project_next_steps`
  - 或唯一存在 ready task 的 DAG
  - 或最近已有 executing task 的主 DAG
- 然后向用户明确：
  - 继续推荐 DAG
  - 查看另一条 DAG
  - 或回到 intake 接一个新需求

### 全部 done

动作：
- 明确告诉用户当前项目已完成
- 不要假装还有 dispatch 主线
- 如果用户要新需求：回到 `intake`

## Step 6. 恢复推进

恢复完成后，默认推进机制应优先使用：

```text
mcp__agentflow__leader_tick(namespace_id)
```

因为 `goal flow` 已经定义：
- `leader_tick` 是主推进机制
- MCP server 才是系统事实源

所以 resume 的职责是：
- 恢复现场
- 给出推荐继续项
- 然后回到 leader 驱动主链

而不是自己重新发明一整套分支推进逻辑。

## 与 Goal / Intake / Shape 的关系

- `resume` 不负责重新做 `shape`
- `resume` 不负责默认新开 DAG
- `resume` 不负责重做 intake

只有以下情况才切回别的 flow：
- 当前目录不在任何项目里 -> 改走 `goal`
- 当前项目已完成、用户想加新需求 -> 先回 `intake`
- 用户明确要求重做产品边界，或缺 `.claude/PROJECT_FINAL_SHAPE.md` -> 再考虑回 `shape`

否则，resume 的职责只是：

```text
恢复已有项目上下文
-> 找回当前 DAG 盘面
-> 继续推进已有执行链
```

## 结束条件

当且仅当以下条件满足时，本 flow 才算完成：
- 已定位 namespace
- 已恢复项目 snapshot
- 已给出 DAG 列表或 active DAG 焦点
- 已给出推荐继续项 / 阻塞原因 / 完成态说明
- 如可继续，已切回 `leader_tick` 主链
