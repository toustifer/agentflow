# agentflow Setup Guide

> Canonical public mirror: https://hub.stifer.xyz/agentflow-setup.md  
> Updated: 2026-07-24

## 概述

agentflow 本地侧是 **三件套**（缺一不可）：

| 组件 | 作用 |
|------|------|
| **Skill** `~/.claude/skills/agentflow/` | `/agentflow`、flows、hooks |
| **MCP 二进制** + Host stdio | `mcp__agentflow__*` 工具 |
| **Sticky hooks** | `/agentflow on` 跨轮注入规则 |

**本地 MCP 服务器 = Go 二进制 `agentflow stdio`。**  
不要再走 `agent-company` + Node `agentflow-mcp.mjs` 主路径（已废弃）。

## 验收三层（不要混）

| 层 | 检查 | 通过才算 |
|----|------|----------|
| 配置 | `~/.claude.json` 有 `mcpServers.agentflow` | 仅「写过配置」 |
| 进程/UI | `/mcp` 列出 agentflow 且 **非 failed** | 用户侧必过 |
| **会话工具** | 模型本轮能调用 `mcp__agentflow__flow_ping` | **唯一业务验收** |

`claude mcp list` Connected、`agentflow:on` statusline、Bash 调 stdio 写库 —— **都不算** MCP 可用。

MCP 未通过时：agent 必须停并让用户修 MCP，**禁止** JSON-RPC / sqlite 旁路继续 goal。

## 前置条件

- **Go 1.22+**（能 `go build ./cmd/agentflow` 即可；版本以仓库 `go.mod` 为准）
- **Node.js 18+**（仅 sticky hooks / statusline，**不是** MCP 主路径）
- **Git**

## 快速安装

### Windows

```powershell
git clone https://github.com/toustifer/agentflow.git
cd agentflow

# 1) Skill（含 hooks / flows / MCP GATE）
$dst = "$env:USERPROFILE\.claude\skills\agentflow"
New-Item -ItemType Directory -Force -Path $dst | Out-Null
Copy-Item -Recurse -Force .\skills\agentflow\* $dst

# 2) 本地 MCP 二进制（放 skill/bin）
$bin = "$dst\bin"
New-Item -ItemType Directory -Force -Path $bin | Out-Null
go build -o "$bin\agentflow.exe" .\cmd\agentflow\
```

`~/.claude.json`（用户级；**不是** `~/.claude/.mcp.json`）：

```json
{
  "mcpServers": {
    "agentflow": {
      "command": "C:\\Users\\YOU\\.claude\\skills\\agentflow\\bin\\agentflow.exe",
      "args": ["stdio"],
      "type": "stdio"
    }
  }
}
```

`~/.claude/settings.json` sticky hooks（**合并**，勿整文件覆盖）：

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "node C:\\Users\\YOU\\.claude\\skills\\agentflow\\hooks\\mode-inject.js",
            "timeout": 5
          }
        ]
      }
    ]
  },
  "statusLine": {
    "type": "command",
    "command": "node C:\\Users\\YOU\\.claude\\skills\\agentflow\\hooks\\statusline.js",
    "refreshInterval": 5
  }
}
```

把 `YOU` 换成真实用户名；路径用绝对路径。

### macOS / Linux

```bash
git clone https://github.com/toustifer/agentflow.git
cd agentflow

mkdir -p ~/.claude/skills/agentflow
rsync -a skills/agentflow/ ~/.claude/skills/agentflow/
# 或: cp -R skills/agentflow/. ~/.claude/skills/agentflow/

mkdir -p ~/.claude/skills/agentflow/bin
go build -o ~/.claude/skills/agentflow/bin/agentflow ./cmd/agentflow/
```

`~/.claude.json`：

```json
{
  "mcpServers": {
    "agentflow": {
      "command": "/Users/YOU/.claude/skills/agentflow/bin/agentflow",
      "args": ["stdio"],
      "type": "stdio"
    }
  }
}
```

`~/.claude/settings.json`：

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "node /Users/YOU/.claude/skills/agentflow/hooks/mode-inject.js",
            "timeout": 5
          }
        ]
      }
    ]
  },
  "statusLine": {
    "type": "command",
    "command": "node /Users/YOU/.claude/skills/agentflow/hooks/statusline.js",
    "refreshInterval": 5
  }
}
```

Linux 把 `/Users/YOU` 换成 `$HOME` 展开后的绝对路径。

### Codex CLI（可选，同一二进制）

```bash
codex mcp add agentflow -- "$HOME/.claude/skills/agentflow/bin/agentflow" stdio
```

Hub 团队协作另见：https://hub.stifer.xyz/codex-setup.md

## 验证（全部过才算装好）

1. **完全退出并重启** Claude Code  
2. `/mcp` → 有 `agentflow` 且 **不是 failed**  
3. 本会话能调用 `mcp__agentflow__flow_ping`（**唯一业务验收**；仅 `claude mcp list` Connected 不够）  
4. `grep -n "MCP GATE" ~/.claude/skills/agentflow/hooks/mode-lib.js` 有命中  
5. `/agentflow on` → mode 文件生成；`mode-cli` / status 可带 `mcp` 探测与 warnings  
6. statusline：`agentflow:on · MCP:cfg|missing|broken`（有 MCP badge 说明新 statusline 已生效）  
7. MCP 不可用时 agent **必须停**，禁止 Bash/JSON-RPC/sqlite 旁路  

| Symptom | Fix |
|---------|-----|
| No `/agentflow` | Skill 未拷到 `~/.claude/skills/agentflow` |
| `/mcp` 无 agentflow / failed | 二进制路径错、缺 `args:["stdio"]`、需重启 |
| `MCP GATE` grep 无 | skill 未更新到含门禁的版本；`git pull` + 重拷 skill |
| `on` 但不 sticky | settings.json hooks 路径错或未装 Node |
| 模型 Bash 调 stdio | **无效**；修 MCP，不要接受旁路 |

## 升级 skill + 二进制

```bash
cd /path/to/agentflow
git pull
rsync -a skills/agentflow/ ~/.claude/skills/agentflow/
go build -o ~/.claude/skills/agentflow/bin/agentflow ./cmd/agentflow/   # Windows: agentflow.exe
```

重启 Claude Code。确认：

```bash
grep -n "MCP GATE" ~/.claude/skills/agentflow/hooks/mode-lib.js
```

## Sticky Mode 使用

```text
/agentflow on
/agentflow on --namespace my-ns --dag feature-x
/agentflow status
/agentflow off
```

底层：

```bash
node ~/.claude/skills/agentflow/hooks/mode-cli.js on
node ~/.claude/skills/agentflow/hooks/mode-cli.js status
node ~/.claude/skills/agentflow/hooks/mode-cli.js off
```

mode 文件：`<project>/.claude/agentflow/mode.json`  
`agentflow:on` **只表示 mode 开了**，不表示 MCP 可用。

inspect 渲染会刷新 `status.json` 供 statusline 使用：

```text
mcp__agentflow__project_inspect(...)
-> node ~/.claude/skills/agentflow/hooks/render-inspect.js
```

## 已废弃（勿再教）

- `~/.claude/skills/agent-company/bin/agentflow-mcp.mjs` + `@modelcontextprotocol/sdk` 作为主 MCP 路径  
- 无 `args: ["stdio"]` 的裸 `command`（会挂死 / 超时）  
- 把 `claude mcp list` Connected 当成会话可用  
- MCP 失败时用 Bash JSON-RPC / 直接 sqlite 继续 goal  

## 可选：Hub 团队 MCP

```json
"hub": { "type": "http", "url": "https://hub.stifer.xyz/mcp" }
```

soft-sync：`~/.agent-hub/config.json` — 见 https://hub.stifer.xyz/agent-setup.md
