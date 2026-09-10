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

如果用户执行的是：

```text
/agentflow resume dag <dag_id>
```

则本次恢复属于 **targeted resume**：
- 仍先恢复项目级 snapshot
- 但后续 `project_next_steps` / `project_inspect` / `dag_get` 应携带 `dag_id=<dag_id>`
- 不再让默认 recommended DAG 覆盖该目标
- 若目标 DAG 带有 `legacy=true` 或 `resume_priority=deprioritized`，必须明确提示：这是显式恢复历史线，而不是默认主线

resume 的首屏优先读这些事实：

```text
mcp__agentflow__project_next_steps(namespace_id)
mcp__agentflow__project_report(namespace_id)
mcp__agentflow__dag_list(namespace_id)
mcp__agentflow__project_inspect(namespace_id, focus="project")
```

先回答：
- 当前项目处于什么 `phase`
- 当前系统推荐的 `next_steps / actions` 是什么
- 一共有多少 DAG、多少完成、多少仍在推进
- 当前有哪些非完成 DAG

这一步的输出应先是 **项目摘要**，不是直接钻进某条 task。

如果 `project_inspect` 可用：
- 执行链固定为：`mcp__agentflow__project_inspect(...) -> node hooks/render-inspect.js`
- 额外输出一版紧凑树状 project 视图
- 该脚本同时负责刷新 `.claude/agentflow/status.json`
- 在结尾明确给出 inspect 入口：
  - `/agentflow inspect`
  - `/agentflow inspect dag <dag_id>`
  - `/agentflow inspect task <task_id>`

## Step 3. 输出 DAG 列表

resume 默认要先展示一个紧凑 DAG 列表，优先列 **非完成 DAG**：
- `dag_id`
- `title`
- `execution_branch`
- `base_branch`（如果可见）
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
- 自动聚焦这条 DAG（`leader_tick` / `project_next_steps` 可不传 `dag_id`，server 会 `single_auto`）
- 告诉用户当前 phase、当前 DAG、下一步建议
- 后续推进仍建议显式带上 `dag_id`（`focused_dag_id`）

### 有多条 active DAG

动作：
- 先展示 DAG 列表
- **禁止**无 `dag_id` 调用 `leader_tick` / 推进用 `project_next_steps`（server 会报 `dag_id required`）
- 给出推荐 DAG（`recommended_dag_id` 或列表中的主线）
- 向用户明确后任选其一：
  - `/agentflow resume dag <dag_id>`
  - 在会话中选定 `dag_id` 后再 tick
  - 查看另一条 / 回到 intake

### 全部 done

动作：
- 明确告诉用户当前项目已完成
- 不要假装还有 dispatch 主线
- 如果用户要新需求：回到 `intake`

## Step 6. 恢复推进

恢复完成后，默认推进机制：

```text
mcp__agentflow__leader_tick(namespace_id, dag_id=<focused>)
```

注意：
- `leader_tick` 只刷新 phase / next_tasks，**prepare-only**（不会 TransStart）
- 多 DAG 时必须先选定 `dag_id`，否则会 error
- 真实开工仍走 skill-primary：`task_prepare_start → spawn Agent → transition(start)`

resume 的职责是：
- 恢复现场
- 锁定 `dag_id` 焦点
- 给出推荐继续项
- 然后回到 leader + skill 派工主链

而不是自己重新发明一整套分支推进逻辑，也不是把 `lifecycle_tick` 当生产主循环。

### 恢复后的 execute 约束

resume 一旦进入可执行态，必须遵守 `flows/goal.md` 的 Execute 铁律：

1. Leader 只负责 prepare / spawn / transition / sync / 协调
2. Leader **不得**在主会话实现 ready task 的产品代码
3. 主仓必须留在 `base_branch`；`execution_branch` 只存在于 DAG worktree
4. 每条 ready task 的固定顺序：

```text
task_prepare_start   // BT dispatch_task 与此等价，不会 start
  -> spawn real Agent subagent in worktree_path
  -> task_transition(start) + launch.ticket + real worker_agent_id
       + runtime.provider + runtime.status=started
  -> worker implement/commit/submit
  -> task_worker_sync / review
```

如果 `task_prepare_start` 或 worktree 失败：
- 只修 git 占用 / invalid worktree / 权限
- 或 escalate
- **禁止** 退化成主会话手写代码

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
