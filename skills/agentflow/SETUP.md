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
