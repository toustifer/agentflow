# agentflow Setup Guide

## 概述

agentflow 是一个 Go 语言的 MCP 服务，通过 stdio 协议与 Claude Code 通信，提供 24 个 `mcp__agentflow__*` 工具。

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
