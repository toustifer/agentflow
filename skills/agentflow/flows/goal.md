# Goal Flow

这是 `agentflow` bundle 内部的 **goal flow** 文档。

用途：当用户执行 `/agentflow goal [目标描述]`，或 `/agentflow [目标描述]` 且当前目录还没有进入现有项目恢复流时，主入口应先经过 `intake`，再按此 flow 推进。

## Intake 前置

在进入本 flow 之前，主入口必须先读取 `flows/intake.md`。

只有当 intake 给出：
- `decision = accepted`
- `next_action = enter_shape`

才允许继续进入本 flow 的 `shape -> plan -> execute` 主链。

如果 intake 给出：
- `deferred`
- `rejected`
- 或仍然 `ask_user`

则本 flow 不应继续假装推进。

## 核心原则

由 Behavior Tree 驱动。不断调用 `mcp__agentflow__leader_tick` 推进一步，让 MCP server 作为系统事实源。

## Agent Loop

```text
while phase != "done":
  result = mcp__agentflow__leader_tick(namespace_id)
  phase = result.phase
  tree_status = result.tree_status

  setup   -> namespace / project bootstrap
  intake  -> 已在主入口完成 accept / defer / reject 决策
  shape   -> 调用 shape flow，写 .claude/PROJECT_FINAL_SHAPE.md
  plan    -> dag_create + task_create_batch
  execute -> dispatch / monitor / stuck handling
  done    -> completion reporting

  if tree_status == "failure": human intervention
  if phase == "done": break
```

## Shape

进入 `shape` 阶段的前提：
- `intake` 已明确接受这次需求
- `intake` 已给出继续携带的约束

进入 `shape` 阶段时：
- 调用 bundle 内部 `shape flow`
- 不调用通用 `brainstorming`
- shape 的唯一正式产物是 `.claude/PROJECT_FINAL_SHAPE.md`
- 默认不得写 `docs/superpowers/specs/*`

shape flow 完成后，先确认当前 repo 已经有首个 commit，可作为 worktree 派发基线。

- 如果还没有首个 commit：先提交 `README.md`、`.claude/PROJECT_FINAL_SHAPE.md`、`.gitignore`
- 完成后再继续：`worker_register -> dag_create -> task_create_batch`

在 `worker_register` 阶段，要把 `prompt_template` 作为 worker 定义的一部分一起配好。

注册完 worker 后，先做一次 prompt preflight：
- 对每个将参与当前 DAG 的 worker 调一次 `worker_prompt_get`
- 只要有一个 worker 缺模板或 prompt 展开失败，就先停住修好
- 不要等到第一条 task `start` 之后才发现 `worker has no prompt template configured`

## Plan

当 `shape` 已确认、worker 已就位后：
- 按 `.claude/PROJECT_FINAL_SHAPE.md` 拆 DAG
- 用 `dag_create`
- 用 `task_create_batch`
- task 粒度应与 worker 角色、Rigid 边界、依赖顺序一致

## Execute

每条 task 的 dispatch 模板：

```text
0. doc_search(当前模块关键词)
1. task_get(task_id)
2. 进入已准备好的 worktree
3. 写代码 + 测试
4. git add -p && git commit -m "task=..."
5. doc_write(...)
6. worker_diary_write(...)
7. task_transition submit
```

约束：
- Worker 只在独立 worktree 工作
- 不能跳过 commit
- 不能跳过 worker diary
- 触碰 Rigid 决策必须先和用户重新对齐

## Stuck / Done

- stuck：明确展示阻塞原因，不要假装系统还能自动推进
- done：报告完成情况，并询问用户是否要加新功能或进入下一轮目标
