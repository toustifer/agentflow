# Update Flow

这是 `agentflow` bundle 内部的 **update flow**。

用途：用户执行 `/agentflow update`（或 `upgrade` / `version`）时，**同时检查 skill 与 MCP 二进制版本**，并告诉对方去哪里核对、如何升级。

## 目标

1. 读本地 **skill** 版本（`~/.claude/skills/agentflow/VERSION`）
2. 读本地 **MCP 二进制** 版本（`agentflow version-json` / `version`，路径来自 `~/.claude.json` 的 `mcpServers.agentflow`）
3. 拉 GitHub **latest release** 对比
4. 若落后或 skill/MCP 版本不一致 → 给出同一 `VERSION=` 的一键升级命令
5. 明确「检查位置」：skill 文件、binary 路径、Release 页、文档

**不要求** MCP 会话工具已注入即可跑检查（CLI 直接读磁盘 + 可选调 binary）；但升级后必须重启并用 `flow_ping` 验收。

## 执行步骤（agent 必做）

### Step 1. 跑检查脚本

在 skill 安装目录下（通常 `~/.claude/skills/agentflow`）：

```bash
node hooks/version-check.js
# 只要 JSON：
node hooks/version-check.js --json
# 不访问网络：
node hooks/version-check.js --offline
```

Windows 同理，用 skill 的绝对路径：

```powershell
node $env:USERPROFILE\.claude\skills\agentflow\hooks\version-check.js
```

### Step 2. 同时读会话侧 MCP 版本（若工具可用）

调用：

```text
mcp__agentflow__flow_ping
```

期望字段（v0.2.2+）：

```json
{ "ok": true, "version": "v0.2.2", "commit": "...", "date": "..." }
```

把 `flow_ping.version` 与 `version-check.js` 的 `mcp.version` 对照；若会话仍是旧进程，提示 **完全退出并重启** Claude Code。

### Step 3. 向用户展示（固定版式）

必须同时展示 **skill** 与 **mcp** 两行，不可只报一个：

```text
agentflow update check
======================
skill:  vX.Y.Z  [up_to_date|outdated|...]  (path/to/VERSION)
mcp:    vX.Y.Z  [up_to_date|outdated|...]  (path/to/binary)
latest: vA.B.C  https://github.com/toustifer/agentflow/releases/...
needs_update: YES|no

Where to check:
  - skill VERSION: ...
  - MCP binary:    ...
  - releases:      https://github.com/toustifer/agentflow/releases
  - docs:          https://hub.stifer.xyz/agent-setup.md

Next:
  - ...
```

`status` 含义：

| status | 含义 |
|--------|------|
| `up_to_date` | 本地 ≥ latest（同版本） |
| `outdated` | 本地 < latest，应升级 |
| `ahead` | 本地 > latest（自构建/未发布） |
| `unknown_local` / `unknown_remote` | 读不到本地或网络失败 |
| `unparsable` | 版本字符串无法比较 |

### Step 4. 需要升级时给出命令（skill + MCP 一起装）

**macOS / Linux：**

```bash
curl -fsSL https://raw.githubusercontent.com/toustifer/agentflow/master/scripts/install.sh \
  | VERSION=<latest> bash -s -- --write-config
```

**Windows PowerShell：**

```powershell
$env:VERSION = '<latest>'
irm https://raw.githubusercontent.com/toustifer/agentflow/master/scripts/install.ps1 | iex
```

然后：

1. 合并 sticky hooks（若脚本只打印了片段）  
2. **完全退出并重启** Claude Code  
3. 再跑 `/agentflow update` 与 `flow_ping`，确认 skill 与 mcp 版本一致且等于 latest  

### Step 5. skill / MCP 不一致

若 `mismatch_skill_mcp: true`：

```text
Skill 与 MCP 二进制版本不一致 — 必须用同一 VERSION 重装两件套（install.sh 会同时更新 skill.tgz + binary）。
不要只换其中一个。
```

## 检查位置（产品话术，照抄）

| 组件 | 查哪里 |
|------|--------|
| Skill 版本 | `~/.claude/skills/agentflow/VERSION` |
| MCP 二进制版本 | `agentflow version` / `version-json`；路径在 `~/.claude.json` → `mcpServers.agentflow.command` |
| 会话是否已加载新 MCP | 本会话 `flow_ping.version`（旧进程会仍报旧版） |
| 最新版 | https://github.com/toustifer/agentflow/releases/latest |
| 安装说明 | https://hub.stifer.xyz/agent-setup.md |

## 边界

- **不要**在 update flow 里推进 goal / 写产品代码  
- **不要**用 Bash JSON-RPC 旁路替代 MCP 业务调用；检查版本用 `version-check.js` + `flow_ping` 即可  
- 网络失败时仍要打印本地 skill/mcp 版本与手动 Release 链接  
- `on`/`off` 不替代 update；`status` 可附带一句「完整版本检查请 /agentflow update」

## 验收

- `/agentflow update` 输出含 skill **和** mcp 两行版本  
- outdated 时含可复制 install 命令  
- 升级重启后 `flow_ping.version` == skill `VERSION` == latest tag  
