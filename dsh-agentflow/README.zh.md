# @stifer/dsh-agentflow

[![dshfind](https://dshfind.com/api/badge/toustifer/agentflow)](https://dshfind.com/en/plugins/toustifer/agentflow?ref=badge)
[![npm version](https://img.shields.io/npm/v/@stifer/dsh-agentflow.svg)](https://www.npmjs.com/package/@stifer/dsh-agentflow)
[![license](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> DeepSeek Harness (DSH) 原生多智能体自主工程编排器：融合 Live-Spec 4D 动态画布与 Team Hub 联邦协作。

为 DeepSeek Harness 提供完整的 Agentflow 生命周期编排支持。安装本插件后：
- 自动挂载 [agentflow](https://github.com/toustifer/agentflow) MCP 核心服务（提供 `mcp__agentflow__*` 共 61+ 个原子调度工具）。
- 自动同步最新版 `agentflow` 技能（带版本守卫与防降级校验）至 DSH 技能根目录。
- 支持与 Live-Spec 4D 交互式拓扑画布协同，实现任务时序推演与状态联动。

---

## ⚡ 30 秒上手指南

### 1. 安装插件
```bash
dsh plugin --profile web add @stifer/dsh-agentflow
```

### 2. 启用配置
在 `<dshHome>/profiles/web/cordis.patch.yml` 中追加配置项：

```yaml
- insert:
    - id: agentflow
      name: '@stifer/dsh-agentflow'
```

### 3. 重启与验证
重启 DSH（或在 Web 会话中新开窗口），立刻体验：
- 会话中输入 `/agentflow` 唤醒编排引导助手。
- 调用 `mcp__agentflow__flow_ping` 验证连通性（返回 `{ "ok": true }`）。
- 调用 `mcp__agentflow__project_init(workdir="...")` 一键接入本地代码仓库。

---

## 🏛️ 双引擎架构设计

Agentflow 采用解耦而严密的双引擎架构，确保高可靠性与沙箱安全性：

```
┌─────────────────────────────────────────────────────────────┐
│                    DeepSeek Harness (DSH)                   │
│   ┌───────────────────────┐       ┌─────────────────────┐   │
│   │  @stifer/dsh-agentflow     │ ────> │  Live-Spec 4D 动态画布│   │
│   └───────────────────────┘       └─────────────────────┘   │
└───────────────┬─────────────────────────────────────────────┘
                │ stdio (自适应 Content-Length / NDJSON 双模帧)
┌───────────────▼─────────────────────────────────────────────┐
│ 1. Go 状态机内核 (Core State Engine)                        │
│    - 状态单一事实源 (SSOT)：基于 SQLite 的强一致性持久化     │
│    - 物理沙箱隔离：基于 Git Worktree 为每个 Task 创建独立工作区│
│    - 交付门禁流转：ready → executing → review → pass/rework │
│    - 暴露 61+ 项原子 MCP 工具 (mcp__agentflow__*)           │
└───────────────▲─────────────────────────────────────────────┘
                │ RPC / Local Socket
┌───────────────▼─────────────────────────────────────────────┐
│ 2. Python 行为树引擎 (BT Sidecar Engine)                    │
│    - 零 pip 第三方依赖：纯 Python 标准库实现                 │
│    - Leader / Worker / Reviewer 三角色自治行为树闭环         │
│    - 黑板共享机制 (Blackboard) 与自省反思日记 (Diaries)      │
└─────────────────────────────────────────────────────────────┘
```

1. **Go 状态机内核**：
   - 作为系统的**单一事实源**，内置 SQLite 数据库管理项目元数据、DAG 依赖关系与任务状态机。
   - **Git Worktree 物理隔离**：每个任务独享独立 Git 工作区，杜绝多 Agent 并行执行时的代码污染与文件冲突。
   - **自适应双模帧**：原生自适应识别 Content-Length 与换行分隔 JSON (NDJSON) 协议帧，与 DSH 官方 MCP Client 完美契合。
2. **Python 行为树引擎 (bt_service)**：
   - 遵循严格的自举铁律，采用**纯标准库无外部依赖**实现。
   - 驱动 Leader（阶段推进与任务分发）、Worker（工单执行与代码提交）和 Reviewer（代码审查与门禁决策）的行为逻辑树。

---

## 前置要求

- 已安装并初始化 DeepSeek Harness（任意 profile）。
- **agentflow 核心二进制 v0.2.8+**（推荐最新 **v0.3.0**）：
  从 v0.2.8 起原生支持 stdio 自适应 Content-Length 与 NDJSON 双模帧。由于 DSH（基于 TypeScript MCP SDK）默认使用换行分隔 JSON 帧，早期版本可能引发协议超时。
  验证本地版本：
  ```bash
  agentflow version    # 应输出 agentflow v0.3.0 (commit ...)
  ```

---

## 详细配置

所有字段均为可选，默认配置如下：

| 字段 | 默认值 | 说明 |
|---|---|---|
| `serverName` | `agentflow` | 工具名前缀，注册为 `mcp__<serverName>__*` |
| `command` | `''` → `$AGENTFLOW_BIN` → `agentflow` | agentflow 二进制可执行文件路径 |
| `args` | `['stdio']` | 传递给二进制的命令行参数 |
| `dbPath` | `''` | 非空时重定向 `AGENTFLOW_DB_PATH` 数据库位置 |
| `syncSkill` | `true` | 是否自动同步随包内置技能（带版本守卫） |
| `toolCallTimeoutMs` | `60000` | 单次工具调用超时时间（毫秒） |
| `failOnStartupError` | `false` | MCP 启动连接失败时是否中断插件激活 |
| `reconnect` | mcp-client 默认 | 连接断开时的自动重试策略 |

配置示例（显式指定路径与独立数据库）：

```yaml
- insert:
    - id: agentflow
      name: '@stifer/dsh-agentflow'
      config:
        command: 'D:\myprogram\agentflow\bin\agentflow.exe'
        dbPath: 'C:\Users\me\.dsh\agentflow\agentflow.db'
```

---

## 验证与验收

1. **技能就绪**：在 DSH 会话中查看技能列表，确认包含 `agentflow`。
2. **工具挂载**：在工具列表中确认已挂载 `mcp__agentflow__flow_ping` 及相关工具。
3. **冒烟调用**：执行 `mcp__agentflow__flow_ping`，返回 `{ "ok": true }`。
