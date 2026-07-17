# Goal Flow

这是 `agentflow` bundle 内部的 **goal flow** 文档。

用途：当用户执行 `/agentflow goal [目标描述]`，或 `/agentflow [目标描述]` 且当前目录还没有进入现有项目恢复流时，主入口应先经过 `intake`，再按此 flow 推进。

新绑定成功但尚未形成 shape / DAG 的已有内容项目，也可以先从 `init` 转入 `intake -> goal`。

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

在 `worker_register` 阶段，要把下面这些字段当成 worker 定义的一部分一起配好：
- `kind`
- `scope`
- `skills`
- `task_tags`
- `prompt_template`
- `required_reads`
- `recommended_mcp`
- `launch_mode`
- `handoff_targets`

其中最少不能缺：
- `prompt_template`
- `kind`
- `launch_mode`

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

**Branch 约束：**
- project / namespace 只持有默认 `base_branch`（通常是 `main`）
- 每个 DAG 必须显式创建自己的 `execution_branch`（通常是 `feature/...`）
- `main` 只作为基线/最终合并目标，不作为活跃 DAG 的执行 branch
- 同一 DAG 下的 task 默认在同一条 `execution_branch` 上顺序推进

## Execute

### Leader 铁律（禁止代做）

Leader 主会话 **不是** 实现者。

Leader 只允许：
- `project_next_steps` / `project_inspect` / `leader_tick`
- `task_prepare_start`
- spawn 真实 `Agent` subagent
- `task_transition(start|...)` + `task_worker_sync`
- diary / doc / shape 文件（通常只在 `.claude/`）
- 修 git/worktree 所有权冲突，或 escalate 给用户

Leader **禁止**：
- 在产品 workdir 直接 `Write` / `Edit` 业务代码来完成 task
- 在主仓 checkout `execution_branch` 后自己 commit 交付
- 因 prepare/start 失败就“我先帮 worker 写完”
- 把 `task` 标成 done/executing，却没有真实 `worker_agent_id`

失败时的唯一合法路径：

```text
prepare/start 失败
  -> 修 base/execution branch 占用 / worktree / 权限
  -> 重新 task_prepare_start
  -> 再 spawn Agent
  -> 仍失败则 escalate 给用户
  -> 永远不要主会话代写产品代码
```

### Leader 派工协议（每条 ready task）

```text
1. task_get(task_id) / project_next_tasks
2. task_prepare_start(namespace_id, task_id)
3. 读取返回的 worker_launch / prompt_template / worktree_path / launch_ticket
4. Agent({
     description: "worker:<assigned_worker> <task_id>",
     prompt: prompt_template + task context + required_reads,
     // cwd / isolation 指向 briefing.worktree_path
   })
5. task_transition(start) with:
     launch.ticket
     worker_agent_id = 真实 Agent subagent id
6. 等待 worker 完成
7. task_worker_sync + worker submit
8. reviewer pass/rework
```

没有第 4 步真实 spawn，就不得进入第 5 步 start。

### Worker 在 worktree 内的实现模板

```text
0. doc_search(当前模块关键词)
1. task_get(task_id)
2. 确认 cwd = prepared worktree_path，且 branch = DAG execution_branch
3. 写代码 + 测试
4. git add -p && git commit -m "task=..."
5. doc_write(...)
6. worker_diary_write(...)
7. task_transition submit
```

### Branch / worktree 所有权

- 主会话 / 主仓 `workdir` 必须停留在 `base_branch`（通常 `main`）
- **禁止** 在主仓 checkout DAG 的 `execution_branch`
- Worker 只在 DAG shared `worktree_path` 工作
- task worktree 永远围绕 DAG 的 `execution_branch` 创建
- `git.branch` = DAG execution branch；`git.base_branch` = project/DAG base branch
- 同一 DAG 若共用单一 execution branch + shared worktree，则同一时刻只有一个 lease holder；不要假装多 worker 可并行改同一 branch
- 不能跳过 commit
- 不能跳过 worker diary
- 触碰 Rigid 决策必须先和用户重新对齐

## Stuck / Done

- stuck：明确展示阻塞原因，不要假装系统还能自动推进
- done：报告完成情况，并询问用户是否要加新功能或进入下一轮目标
