# agentflow DSH Presets — Leader / Worker

本目录提供两套**项目无关（project-agnostic）**的 DSH agent preset，把 agentflow 的 Leader/Worker 协议深度注入任意项目的 Agent：

| Preset | 目录 | 角色 | 核心边界 | 关键工具 |
|--------|------|------|----------|----------|
| `agentflow-leader` | [`agentflow-leader/`](./agentflow-leader/) | **Leader / 编排** | 不代做（不写产品代码、不自己 commit/submit、不亲自调试/查库） | `subagent` / `subagent_fork` / **`spawn_worker`** |
| `agentflow-worker` | [`agentflow-worker/`](./agentflow-worker/) | **Worker / 实现** | 不 orchestrate（不拆 DAG、不派发、不 spawn 子代理；`delegation` 组已移除） | （无编排工具） |

两者共用一套 Agentflow 协议语言：都基于 `standard` 拷贝，保留完整编码能力，仅改写人格与工具边界，并各自携带一份可加载的深度参考技能。预设不内置任何业务域；项目自身的 Worker 名册以仓库 `AGENTS.md` / `CLAUDE.md` 为准。

## 一键使用

- **当 Leader**：创建 DSH 会话选 `agentflow-leader`。用 `mcp__agentflow__*` 拆 DAG、`task_prepare_start`、生成真实 Agent、`task_transition`、验收。绝不代写。
- **生成 Worker**：在 `agentflow-leader` 里用 **`spawn_worker`**——该工具实例给子 Agent 施加 Worker 人格并剥掉编排工具。
- **当独立 Worker**：需要工具级剥离的独立实现者会话，选 `agentflow-worker`。

## 为什么用 `spawn_worker` 而不是切 preset

DSH 的子 Agent 通过 `composeFrom` 固定 join 父 preset，**没有按调用换 preset 的机制**。因此 Leader 预设内配一个专属 `spawn_worker` 工具实例，用它固定的 `persona` + `toolFilter` 达成「Worker 人格 + 无编排工具」的边界，而不是试图切到 `agentflow-worker`。

## 首次使用（一句话自举）

对 `agentflow-leader` 一句话「帮我做一个 xxx」，它会自动：

```
flow_ping 确认 MCP 可用
→ namespace_list 检查绑定
→ 未绑定则 project_init；has_head_commit=false 则补最小首 commit 基线
→ intake → shape → plan → 派发 → 验收
```

并把进度用「人话」回报；只需用户做选择/授权时才打断。

## 目录说明

```
agents/
  README.md                    # 本发现页
  agentflow-leader/            # Leader 预设（agent.cordis.yml + preset.yml + skill + README）
  agentflow-worker/            # Worker 预设（agent.cordis.yml + preset.yml + skill + README）
```
