# agentflow Setup Guide

> Canonical public mirror: https://hub.stifer.xyz/agentflow-setup.md  
> **Default install = download Release (no Go, no git clone).**  
> Updated: 2026-07-24 · Release **v0.2.1**

## 概述

agentflow 本地侧是 **三件套**（缺一不可）：

| 组件 | 作用 |
|------|------|
| **Skill** `~/.claude/skills/agentflow/` | `/agentflow`、flows、hooks（含 MCP GATE） |
| **MCP 二进制** + Host stdio | `mcp__agentflow__*` 工具 |
| **Sticky hooks** | `/agentflow on` 跨轮注入规则 |

**本地 MCP 服务器 = Go 二进制 `agentflow stdio`（预编译下载）。**  
不要再走 `agent-company` + Node `agentflow-mcp.mjs` 主路径（已废弃）。

## 验收三层（不要混）

| 层 | 检查 | 通过才算 |
|----|------|----------|
| 配置 | `~/.claude.json` 有 `mcpServers.agentflow` | 仅「写过配置」 |
| 进程/UI | `/mcp` 列出 agentflow 且 **非 failed** | 用户侧必过 |
| **会话工具** | 模型本轮能调用 `mcp__agentflow__flow_ping` | **唯一业务验收** |

`claude mcp list` Connected、`agentflow:on` statusline、Bash 调 stdio 写库 —— **都不算** MCP 可用。

MCP 未通过时：agent 必须停并让用户修 MCP，**禁止** JSON-RPC / sqlite 旁路继续 goal。

## 推荐安装：一键下载（macOS / Linux）

需要：`curl`、`tar`、Node 18+（hooks）。**不需要 Go / git。**

```bash
curl -fsSL https://raw.githubusercontent.com/toustifer/agentflow/master/scripts/install.sh | bash
```

指定版本 / 自动写入 MCP 配置：

```bash
curl -fsSL https://raw.githubusercontent.com/toustifer/agentflow/master/scripts/install.sh \
  | VERSION=v0.2.1 bash -s -- --write-config
```

脚本会：

1. 下载 `skill.tgz` + 本机 arch 的预编译二进制（GitHub Release）  
2. 安装到 `~/.claude/skills/agentflow/`（含 `bin/agentflow`）  
3. 校验 `MCP GATE` 存在  
4. 打印（或 `--write-config` 写入）`mcpServers.agentflow` 与 sticky hooks 片段  

然后：

1. 若未用 `--write-config`：把脚本打印的 JSON 合并进 `~/.claude.json`  
2. 把 sticky hooks 合并进 `~/.claude/settings.json`（**不要整文件覆盖**）  
3. **完全退出并重启** Claude Code  
4. 按下方「验证」清单过一遍  

## Windows（PowerShell）

```powershell
irm https://raw.githubusercontent.com/toustifer/agentflow/master/scripts/install.ps1 | iex
# 或:
# $env:VERSION='v0.2.1'; irm ... | iex
# .\install.ps1 -WriteConfig
```

装到 `%USERPROFILE%\.claude\skills\agentflow\`，二进制为 `bin\agentflow.exe`。

## 手动下载（不用 install 脚本）

Release：https://github.com/toustifer/agentflow/releases/tag/v0.2.1

| 资产 | 用途 |
|------|------|
| `skill.tgz` | skill + hooks + flows（**必下**） |
| `agentflow-darwin-arm64` | Apple Silicon |
| `agentflow-darwin-amd64` | Intel Mac |
| `agentflow-linux-amd64` | Linux x64 |
| `agentflow-windows-amd64.exe` | Windows x64 |

```bash
VERSION=v0.2.1
BASE=https://github.com/toustifer/agentflow/releases/download/$VERSION
DEST=~/.claude/skills/agentflow
mkdir -p "$DEST/bin"
curl -fsSL "$BASE/skill.tgz" | tar -xz -C /tmp
rsync -a /tmp/agentflow/ "$DEST/"
# pick arch:
curl -fsSL "$BASE/agentflow-darwin-arm64" -o "$DEST/bin/agentflow"
chmod +x "$DEST/bin/agentflow"
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

`~/.claude/settings.json` sticky（合并）：

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

## 验证（全部过才算装好）

1. **完全退出并重启** Claude Code  
2. `/mcp` → 有 `agentflow` 且 **不是 failed**  
3. 本会话能调用 `mcp__agentflow__flow_ping`（**唯一业务验收**）  
4. `grep -n "MCP GATE" ~/.claude/skills/agentflow/hooks/mode-lib.js` 有命中  
5. `/agentflow on`；statusline 可出现 `MCP:cfg|missing|broken`  
6. MCP 不可用时 agent **必须停**，禁止 Bash/JSON-RPC/sqlite 旁路  

| Symptom | Fix |
|---------|-----|
| No `/agentflow` | skill 未装到 `~/.claude/skills/agentflow` |
| `/mcp` 无 agentflow / failed | 二进制路径错、缺 `args:["stdio"]`、需重启 |
| `MCP GATE` grep 无 | 仍是旧 skill；重跑 install 或下新 `skill.tgz` |
| 模型 Bash 调 stdio | **无效**；修 MCP，不要接受旁路 |

## 升级

```bash
curl -fsSL https://raw.githubusercontent.com/toustifer/agentflow/master/scripts/install.sh \
  | VERSION=v0.2.1 bash
```

重启 Claude Code。

## Sticky 使用

```text
/agentflow on
/agentflow status
/agentflow off
```

`agentflow:on` **只表示 mode 开了**，不表示 MCP 可用。

## 附录：开发者从源码构建（非默认）

仅当你要改引擎或无 Release 时：

```bash
git clone https://github.com/toustifer/agentflow.git && cd agentflow
rsync -a skills/agentflow/ ~/.claude/skills/agentflow/
mkdir -p ~/.claude/skills/agentflow/bin
go build -o ~/.claude/skills/agentflow/bin/agentflow ./cmd/agentflow/
# 发布者：
# VERSION=v0.2.1 bash scripts/build-release.sh
# gh release create v0.2.1 dist/agentflow-* dist/skill.tgz
```

## Codex CLI（可选，同一二进制）

```bash
codex mcp add agentflow -- "$HOME/.claude/skills/agentflow/bin/agentflow" stdio
```

Hub：https://hub.stifer.xyz/codex-setup.md

## 已废弃（勿再教）

- 默认路径要求用户 `git clone` + `go build`  
- `agent-company` + `agentflow-mcp.mjs` + npm SDK  
- 无 `args: ["stdio"]` 的裸 command  
- 把 `claude mcp list` Connected 当会话可用  
- MCP 失败时 Bash JSON-RPC / sqlite 旁路  

## 可选：Hub 团队 MCP

```json
"hub": { "type": "http", "url": "https://hub.stifer.xyz/mcp" }
```

见 https://hub.stifer.xyz/agent-setup.md
