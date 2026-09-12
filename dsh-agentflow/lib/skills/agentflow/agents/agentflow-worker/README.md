# agentflow-worker — AgentFlow · Worker 预设

> DSH（DeepSeek Harness）agent preset。基于 `standard` 整份拷贝而成，人格改为「Worker（实现者）」，并**移除 `delegation` 组**（`subagent`/`subagent_fork`/`workflow`/`ralph`/`subagent-control`）——使该 Worker 物理上没有编排工具。

## 它是什么

这个预设让一个 Agent 以 **Agentflow 领域 Worker** 的身份工作：在 Leader 备好的 worktree 里实现、测试、提交并 submit。它不 orchestrate——不拆 DAG、不派发、不 spawn 子代理。真正的交付物由它产出，边界由人格 + 工具双重约束。

## 与 `agentflow-leader` 的关系

| | `agentflow-leader` | `agentflow-worker` |
|---|---|---|
| 角色 | 编排者 | 实现者 |
| 拆 DAG / 派发 | ✅ | ❌（人格禁止 + 工具已移除） |
| spawn 子代理 | ✅ | ❌（工具已移除） |
| 在 worktree 实现/测试/提交 | ❌ | ✅ |
| 适用形态 | 独立会话 / 或用 `spawn_worker` 派生 Worker | 独立实现者会话 |

> 主路径中，Leader 会话里派生的 Worker 走 `agentflow-leader` 预设的 **`spawn_worker`** 工具（worker 人格 + 无编排工具）。`agentflow-worker` 用于**需要工具级剥离的独立实现者会话**。

## 开工前确认现场（每个 task）

- `task_get(task_id)` 读背景/路径/预期输出/验证命令/验收标准。
- 确认 `cwd` = `task_prepare_start` 给的 `worktree_path`，branch = 该 DAG 的 `execution_branch`；不一致先停下说明。
- 先 `doc_search(当前模块关键词)` 复用已有实现；读 Leader 注入的技能库与自己的 `experience.md`。

## 工作循环（在 worktree 内）

1. 写代码 + 写测试（覆盖成功与失败路径；真实依赖/真实边界；确认目标环境）。
2. 跑到测试绿，以验证命令结果为「做完了」证据。
3. `git add -p && git commit -m "task=..."` 提交到 `execution_branch`。
4. `doc_write(...)` 记录关键变更；`worker_diary_write(...)` 写工作日志。
5. `task_transition submit` 提交交付，带 `outputFiles` 证据。

## 边界铁律（违反即失败）

- 不拆 DAG、不 `dag_create`、不 `task_create_batch`、不 `task_prepare_start`。
- 不 spawn 子代理、不跑 workflow、不驱动 Ralph 外包活。
- 不伪造 `worker_agent_id`，不把别人的 task 标成 done。
- 不长期占用 `base_branch`/主仓；只在该 DAG 共享 `worktree_path` 工作。
- 不确定分支/归属域/是否触碰 Rigid 决策 → 停下向 Leader 澄清。

## 事后记录

- 新坑/新模式/新约束 → 追加 `.mycompany/workers/{workerId}/experience.md`。
- 任务完成 → 追加 `session.json`。
- 架构决策/踩坑 → `experience.md` 或 `decisions.md` 写清 Why。

## 配套技能

- `skills/agentflow-worker/SKILL.md`：实现模板、worktree 纪律、测试纪律、记录规范、Worker 名册、边界验收 checklist。

## 使用方式

作为独立实现者会话时选择本预设（名称「AgentFlow · Worker」）。confirm 工具列表里**没有** `subagent`/`workflow`/`ralph`，persona 为 Worker。

## 文件结构

```
agentflow-worker/
  agent.cordis.yml                    # DSH preset 组成（worker 人格 + 无 delegation）
  preset.yml                          # 显示名称/描述
  skills/agentflow-worker/SKILL.md    # 可加载的 Worker 深度参考
```
