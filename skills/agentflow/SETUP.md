# agentflow Setup Guide

## 概述

agentflow 是一个 Go 语言的 MCP 服务，通过 stdio 协议与 Claude Code 通信，提供 24 个 `mcp__agentflow__*` 工具。

### 与 Agent Hub 的对齐

本机调度（namespace / task / dag / worktree）与团队云端索引（branch / dag 摘要 / workers…）的边界、团队页每个 Tab 的数据来源、已齐/未齐矩阵与 soft-sync 路线图见：

- 本仓副本：[`docs/HUB_ALIGNMENT.md`](../../docs/HUB_ALIGNMENT.md)
- 线上（与 Hub 同源）：https://hub.stifer.xyz/agentflow-alignment.md

**自动写 Hub 的 soft 路径（有 JWT + business_code 时）：**

- branch report：`task_prepare_start` / start / `submit`
- **task 投影（A1）：** `task_create` / `task_create_batch` / 任意 `task_transition`（含 prepare_start）→ `hub_dag_state`  
  响应字段 `hub_task_sync`（`ok` / `skipped` / `disabled` / `failed`），**从不挡本地任务**

配置：`~/.agent-hub/config.json` 或 `.mycompany/hub-client.json`。  
**与正常使用解耦（`pkg/hub`）：** 未配置 / `HUB_SYNC=0` 时完全本地。见 `pkg/hub/README.md`（分支 `feat/agentflow-hub-federation`）。

### AI 绑定团队（推荐主路径）

配好 agentflow MCP 后，**不必**让人去 Web 抄 `business_code`。模型直接：

1. `hub_status` — 看是否已登录 / 已绑定  
2. `hub_login` — 无 code 拿 `verification_url`；用户浏览器 Approve 后 `hub_login({code})` 落 JWT  
3. `hub_list_teams` — 读当前用户所属团队  
4. `hub_bind_team({business_code})` — 写入 `~/.agent-hub/config.json`（可选 workdir 镜像）

之后 task create/transition 会 soft 投影。未绑定 / 未登录 → 完全本地，不影响 `flow_ping` 与任务流。

| 工具 | 作用 |
|------|------|
| `hub_status` | 本地状态 + `next` 步骤提示 |
| `hub_login` | 设备码登录（JWT → `~/.agent-hub/config.json`） |
| `hub_list_teams` | `GET /v1/hub/me/businesses` |
| `hub_bind_team` | 绑定 / 解绑 `business_code` |

建团仍可在 Dashboard；绑定由 AI 完成。

## 前置条件

- **Go 1.25+**：编译 agentflow
- **Node.js 18+**：运行 MCP 桥接脚本
- **Git**：克隆源码

## 快速安装

### Windows

```powershell
# 1. 克隆并编译
git clone https://github.com/toustifer/agentflow.git
cd agentflow
go build -o $env:USERPROFILE\.claude\skills\agent-company\bin\agentflow.exe .\cmd\agentflow\

# 2. 安装 MCP SDK（一次性）
cd $env:USERPROFILE\.claude\skills\agent-company\bin
npm init -y
npm install @modelcontextprotocol/sdk

# 3. 配置 MCP（手动，见下方说明）
# 4. 重启 Claude Code
```

### macOS / Linux

```bash
# 1. 克隆并编译
git clone https://github.com/toustifer/agentflow.git
cd agentflow
go build -o ~/.claude/skills/agent-company/bin/agentflow ./cmd/agentflow/

# 2. 安装 MCP SDK（一次性）
cd ~/.claude/skills/agent-company/bin
npm init -y
npm install @modelcontextprotocol/sdk

# 3. 配置 MCP（手动，见下方说明）
# 4. 重启 Claude Code
```

## MCP 配置

编辑 `~/.claude.json`（用户级配置，**不是** `~/.claude/.mcp.json`），在 `mcpServers` 中添加：

```json
{
  "mcpServers": {
    "agentflow": {
      "command": "cmd",
      "args": ["/c", "node", "C:\\Users\\你的用户名\\.claude\\skills\\agent-company\\bin\\agentflow-mcp.mjs"],
      "type": "stdio"
    }
  }
}
```

macOS/Linux 版本的 command：

```json
{
  "mcpServers": {
    "agentflow": {
      "command": "node",
      "args": ["~/.claude/skills/agent-company/bin/agentflow-mcp.mjs"],
      "type": "stdio"
    }
  }
}
```

**注意：** `agentflow-mcp.mjs` 会从同目录启动 `agentflow` 二进制。确保编译好的二进制放在 `bin/` 目录下。

## 验证

重启 Claude Code 后，输入：

```
/mcp
```

应显示：

```
User MCPs (C:\Users\你的用户名\.claude.json)
  agentflow · ✔ connected · 24 tools
```

或手动测试：

```
mcp__agentflow__flow_ping
```

返回 `{"ok": true}` 即成功。

## 升级

```bash
cd ~/agentflow
git pull
go build -o ~/.claude/skills/agent-company/bin/agentflow ./cmd/agentflow/
```

重启 Claude Code 即可生效。

## Sticky Mode（会话保持 /agentflow on）

目标：`/agentflow on` 后，后续普通对话仍自动带上 agentflow 规则。

> Claude Code **不能**在输入框 UI 里挂住 `agentflow` 文本前缀。  
> Sticky mode 用 mode 文件 + `UserPromptSubmit` hook 实现等价效果。

### 1. 安装 skill hooks

确保本仓库 skill 已同步到 Claude skills 目录，例如：

```bash
# Windows 示例
xcopy /E /I /Y D:\myprogram\agentflow\skills\agentflow %USERPROFILE%\.claude\skills\agentflow
```

hooks 路径（安装后）：

```text
~/.claude/skills/agentflow/hooks/mode-cli.js
~/.claude/skills/agentflow/hooks/mode-inject.js
~/.claude/skills/agentflow/hooks/statusline.js
```

### 2. 注册 UserPromptSubmit hook

编辑 `~/.claude/settings.json`，在现有 `hooks` 中**追加**（不要覆盖你已有的 Stop/SubagentStop）：

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "node C:\\\\Users\\\\你的用户名\\\\.claude\\\\skills\\\\agentflow\\\\hooks\\\\mode-inject.js",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

macOS / Linux：

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "node ~/.claude/skills/agentflow/hooks/mode-inject.js",
            "timeout": 5
          }
        ]
      }
    ]
  }
}
```

### 3.（可选）statusline 指示器

```json
{
  "statusLine": {
    "type": "command",
    "command": "node C:\\\\Users\\\\你的用户名\\\\.claude\\\\skills\\\\agentflow\\\\hooks\\\\statusline.js",
    "refreshInterval": 5
  }
}
```

mode on 时输出类似：`agentflow:on`  
有 `status.json` 时升级为两级摘要：第一行显示 `dag + working/ready/blocked`，第二行显示 `phase/progress + workers + busy workers`。  
mode off 时输出空字符串。

### 4. 使用

`/agentflow resume` 与 `/agentflow inspect ...` 现在共享同一条本地渲染链：

```text
mcp__agentflow__project_inspect(...)
-> node ~/.claude/skills/agentflow/hooks/render-inspect.js
```

这会同时：
- 渲染树状项目 / DAG / task 视图
- 刷新 `<project>/.claude/agentflow/status.json`
- 让 `statusline.js` 读取到最新摘要


```text
/agentflow on
/agentflow on --namespace my-ns --dag feature-x
/agentflow status
/agentflow off
```

底层等价命令：

```bash
node ~/.claude/skills/agentflow/hooks/mode-cli.js on
node ~/.claude/skills/agentflow/hooks/mode-cli.js status
node ~/.claude/skills/agentflow/hooks/mode-cli.js off
```

mode 文件写在当前项目：

```text
<project>/.claude/agentflow/mode.json
```

### 5. 验证

1. `/agentflow on`
2. 确认生成了 `.claude/agentflow/mode.json` 且 `"enabled": true`
3. 发一条普通消息（不带 /agentflow）
4. 模型应仍遵守 agentflow 规则（任务用 `id + title`、launch-ticket 等）
5. `/agentflow off` 后普通对话不再注入
