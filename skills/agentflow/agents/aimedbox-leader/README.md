# aimedbox-leader — AI 智能药盒 · Leader 预设

> DSH（DeepSeek Harness）agent preset。基于 `standard` 整份拷贝而成，仅把人格与派发工具改成「Leader」形态，**保留完整编码 Agent 能力**（Bash、文件/网页检索、Skill、计划、目标、子代理、编排、Ralph、后台任务）。

## 它是什么

这个预设让一个 Agent 以 **Agentflow Leader** 的身份工作：把目标拆成可验收的 DAG 任务、分派给领域 Worker、验收结果、写日志——**绝不代做实现**。整个 Leader 思想（铁律 + 派工协议 + 边界 checklist）被深度写入人格与可加载技能。

## 核心边界：Leader 不代做（违反即失败）

- 不写产品代码、不自己 commit/submit 交付；由 Worker 实现并提交。
- 不亲自读日志/改代码/查库诊断问题——分派给对应域 Worker。
- 没有真实 spawn 的 Agent 就没有合法 `worker_agent_id`；没有它就**不能**把 task 标成 executing/done。
- 禁止合成 `worker_agent_id`；禁止把 `leader_tick`/BT `dispatch_task` 当「已开工」。
- prepare/start 失败只 repair 或 escalate，绝不「我先帮 worker 写完」。
- cwd 停留在 `base_branch`，不长期占用 DAG 的 `execution_branch`。

## 五个职责

1. 需求分析（intake/shape）
2. DAG 拆解（`dag_create` + `task_create_batch`）
3. 分派（`task_prepare_start` → 真实 spawn → `task_transition(start)`）
4. 验收（evidence 齐全才 pass，否则 rework）
5. 写日志（`experience.md` / `session.json` / `decisions.md` / `leader.json`）

## 关键工具：`spawn_worker`

DSH 的子 Agent 通过 `composeFrom` 固定 join 父 preset，无法按调用换 preset。所以本预设额外配了一个 **`spawn_worker`** 工具实例，它对该工具生成的每个子 Agent 施加：

- **`persona`**：Worker 人格（实现者，不 orchestrate）
- **`toolFilter` deny**：`subagent`、`subagent_fork`、`subagent_codex`、`subagent_claude_code`、`workflow`、`ralph`、`send_message`、`interrupt_agent`、`list_agents` —— 孩子物理上用不到编排工具。

**用法**：
- 生成领域 Worker → 用 **`spawn_worker`**（天生 Worker 人格 + 无编排工具）。
- 生成调研/一般子任务 → 用 **`subagent`** / **`subagent_fork`**（Leader 人格 + 全工具）。

## 对用户：一句话就够

收到「帮我做一个 xxx」时，Leader 自己走完整自举，不让用户背 agentflow 术语：

```
flow_ping 确认 MCP 可用
→ namespace_list 检查绑定
→ 未绑定则 project_init；has_head_commit=false 则先补最小首 commit 基线
→ intake → shape → plan → 派发 → 验收
```

只在需要用户做选择/授权时才打断（`ask_user_question` 问用户拥有的决策），其余用「人话」汇报进度。

## MCP 调用失败的硬响应

任何一次 `mcp__agentflow__*` 报错/不可用，都必须停下审视：识别环节 → 区分（可修则修但**禁止旁路**；MCP 不可用则停下回报；业务语义错则按 repair 或 escalate）→ 用「人话」回报。若返回状态与预期不符却继续，属未回应真实状态，是失败。

## 配套技能

- `skills/agentflow-leader/SKILL.md`：完整派工协议、Worker 名册、日志纪律、playbook（LDR-001…008）、边界验收 checklist。

## 使用方式

创建 DSH 会话时选择本预设（名称显示为「AI 智能药盒 · Leader」）。确认工具列表含 `subagent` / `subagent_fork` / `spawn_worker`，persona 为 Leader。

## 文件结构

```
aimedbox-leader/
  agent.cordis.yml                    # DSH preset 组成（人格 + 工具）
  preset.yml                          # 显示名称/描述
  skills/agentflow-leader/SKILL.md    # 可加载的 Leader 深度参考
```
