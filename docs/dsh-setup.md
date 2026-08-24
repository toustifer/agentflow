# agentflow on DeepSeek Harness (DSH) — Setup Guide

> Branch: `deepseek/dsh-support` · Base: master (`v0.2.6` Claude/Codex setup)
> Updated: 2026-08-14
>
> 本分支把 agentflow 的宿主支持从 Claude Code / Codex 扩展到
> **DeepSeek Harness (DSH, `@deepseek-ai/dsh`)**。同一个 Go 二进制、同一套
> namespace/DAG/task/worker 模型，宿主差异只体现在 Skill 装载方式和 MCP 注册方式。

## 一、DSH 与 Claude 的宿主差异

| 能力 | Claude Code | Codex CLI | DeepSeek Harness (DSH) |
|------|-------------|-----------|------------------------|
| Skill 装载 | `~/.claude/skills/<name>/SKILL.md` | `~/.codex` 同格式 | `~/.dsh/skills/<name>/SKILL.md`，**必须带 frontmatter** |
| MCP 注册 | `~/.claude.json` → `mcpServers` | `codex mcp add` | profile patch `cordis.patch.yml` → `@deepseek-ai/dsh-mcp-client` |
| Sticky hooks | `UserPromptSubmit` 注入 | `~/.codex/hooks` | **不支持**（无 hook 机制；用 CLI 脚本替代） |
| UI 检查 | `/mcp` | `codex mcp list` | 无 `/mcp`；以**会话工具存在**为准 |
| statusline | 支持 | 支持 | **不支持** |

DSH 的 skill 系统见 `@deepseek-ai/dsh-skill-filesystem`：只认
`<root>/<name>/SKILL.md` 或 `<root>/<name>.md`，frontmatter 必填
`name`（kebab-case）与 `description`；**没有 frontmatter 的 SKILL.md 会被静默跳过**。

## 二、DSH 三件套（对应 Claude 三件套）

| 组件 | DSH 位置 | 说明 |
|------|----------|------|
| Skill | `~/.dsh/skills/agentflow/` | 本仓库 `skills/agentflow/` 内容 + frontmatter 包装 |
| MCP | `~/.dsh/profiles/web/cordis.patch.yml` | `dsh-mcp-client` 插件实例，stdio 启动 `agentflow stdio` |
| CLI（替代 hooks） | `hooks/mode-cli.js` 等 | DSH 无每轮注入；mode/status 走 `node hooks/mode-cli.js` |

## 三、安装步骤

### 1. 安装 Skill

```bash
mkdir -p ~/.dsh/skills/agentflow
rsync -a skills/agentflow/ ~/.dsh/skills/agentflow/
```

**DSH 兼容 frontmatter**（追加在 `~/.dsh/skills/agentflow/SKILL.md` 顶部，
原正文保持不变）：

```markdown
---
name: agentflow
description: 项目编排引擎调度器。/agentflow 是唯一公开入口，提供 setup/init/intake/goal/resume/inspect/shape/mode/update 等 flow；先确认 agentflow MCP 可用（mcp__agentflow__*）再进入业务 flow。
---
```

DSH 发现根（按 rank 合并）：项目 `.dsh/skills`(100) → `.agents/skills`(200) →
`customSkillDirs`(300) → `~/.dsh/skills`(400) → `~/.agents/skills`(500)。
用户级安装放 `~/.dsh/skills` 即可；watcher 检测到变更后会话内热刷新
（`<available_skills>` 目录）。

### 2. 注册 MCP（cordis.patch.yml）

编辑 `~/.dsh/profiles/web/cordis.patch.yml`（web profile 用户 patch 层）：

```yaml
- insert:
    - id: mcp-agentflow
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        serverName: agentflow
        transport: stdio
        command: /Users/YOU/.dsh/skills/agentflow/bin/agentflow
        args: ['stdio']
```

- `serverName` 决定模型侧工具名：`mcp__agentflow__*`
- 支持 `stdio` 与 `streamable-http`；重连/退避/HMR 由 dsh-mcp-client 内置
- 编辑后 HMR 热替换或重启 DSH 生效

### 3. 可选：Hub 团队 MCP

Hub 是独立的 Node stdio MCP（仓库外，`hub-mcp` 项目），token 自动加载
（`HUB_TOKEN` 环境变量 > `~/.agent-hub/config.json`）：

```yaml
    - id: mcp-hub
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        serverName: hub
        transport: stdio
        command: node
        args: ['/Users/YOU/innox/hub-mcp/index.js']
        env:
          HUB_API_URL: https://hub.stifer.xyz
```

hub-mcp 修复记录（DeepSeek 分支配套）：
- 缺 `@modelcontextprotocol/sdk` → `npm install @modelcontextprotocol/sdk`
- `npm init` 生成的 package.json 缺 `"type": "module"` → 补上，否则 ESM import 报错
- token 仅内存 → 启动自动加载 `~/.agent-hub/config.json`（或 `HUB_TOKEN`）

## 四、验证清单（全部过才算装好）

| 层 | 检查 | 通过才算 |
|----|------|----------|
| Skill | 会话 `<available_skills>` 里有 `agentflow` | 模型目录可见 |
| Skill 加载 | `skill("agentflow")` 返回正文 | 指令可用 |
| MCP 进程 | `agentflow stdio` initialize 握手成功 | 服务器活着 |
| **会话工具** | 本轮能调 `mcp__agentflow__flow_ping` | **唯一业务验收** |
| Hub（可选） | 本轮能调 `mcp__hub__hub_get_dag`（传 `business_code`） | 团队通道通 |

```bash
# 服务器侧冒烟（不经 DSH）
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}' \
  | ~/.dsh/skills/agentflow/bin/agentflow stdio
```

预期返回 `serverInfo: agentflow`。MCP 不可用时禁止 Bash 旁路跑
`agentflow`/JSON-RPC/sqlite 当业务验收（同 Claude 侧 MCP GATE 纪律）。

## 五、已知限制（DeepSeek 分支）

- **无 sticky hooks**：`mode-inject.js` 的每轮注入在 DSH 不存在；`/agentflow on`
  语义退化为「写 mode.json + 打印状态」，跨轮保持靠对话上下文而非注入
- **无 `/mcp` UI**：MCP 状态以会话工具列表为准，出错看 DSH 日志
  （`dsh-mcp-client` reconnect 会打 warn/error）
- **hub-mcp 默认 business_code 是 `"ai-medbox"`**：调用时显式传
  `business_code`（如 `aryd`）覆盖，或改 hub-mcp 默认值
- **skill 正文里的 Claude 专有引用**（`/mcp`、`claude mcp list`）在 DSH 下
  按「会话工具存在性」等价理解

## 六、版本与发布

- 分支 `deepseek/dsh-support` 与 master 并行维护；master 的引擎修复会定期合入
- Release 命名建议 `v0.2.6-dsh` 系列，与 Claude/Codex 版 `v0.2.6` 区分
- 构建与打包沿用 `scripts/build-release.sh`（同一 Go 二进制，无宿主差异）
