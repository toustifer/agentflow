# agentflow on DeepSeek Harness (DSH) — Setup Guide

> Branch: `deepseek/dsh-support` · Base: master (`v0.2.8+` dual-mode framing setup)
> Updated: 2026-09-12
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
| stdio 传输帧 | `Content-Length` | `Content-Length` | **NDJSON（换行分隔 JSON）**（需 agentflow v0.2.8+ 原生双模支持） |

DSH 的 skill 系统见 `@deepseek-ai/dsh-skill-filesystem`：只认
`<root>/<name>/SKILL.md` 或 `<root>/<name>.md`，frontmatter 必填
`name`（kebab-case）与 `description`；**没有 frontmatter 的 SKILL.md 会被静默跳过**。

## 二、DSH 三件套（对应 Claude 三件套）

| 组件 | DSH 位置 | 说明 |
|------|----------|------|
| Skill | `~/.dsh/skills/agentflow/` | 本仓库 `skills/agentflow/` 内容 + frontmatter 包装（含 `bt_service` 与 `trees`） |
| MCP | `~/.dsh/profiles/web/cordis.patch.yml` | `dsh-mcp-client` 插件实例，stdio 启动 `agentflow stdio` |
| CLI（替代 hooks） | `hooks/mode-cli.js` 等 | DSH 无每轮注入；mode/status 走 `node hooks/mode-cli.js` |

环境要求：Node 18+、Python 3.8+（执行 `python -m bt_service` 行为树引擎，纯标准库无第三方 pip 依赖）。

## 三、安装步骤

### 1. 安装 Skill

```bash
# 若从源码仓库安装，先确保同步最新 bt_service 与 trees
bash scripts/sync-skill.sh
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
        command: /Users/YOU/.dsh/agentflow/bin/agentflow
        # Windows 示例：'C:\Users\YOU\.dsh\agentflow\bin\agentflow.exe'
        args: ['stdio']
        dbPath: /Users/YOU/.dsh/agentflow/agentflow.db
        # Windows 示例：'C:\Users\YOU\.dsh\agentflow\agentflow.db'
```

- `serverName` 决定模型侧工具名：`mcp__agentflow__*`
- 支持 `stdio` 与 `streamable-http`；重连/退避/HMR 由 dsh-mcp-client 内置
- 编辑后 HMR 热替换或重启 DSH 生效

#### 目录隔离与 Windows 文件锁避坑

- **推荐独立目录**：统一推荐将二进制放在独立目录 `~/.dsh/agentflow/bin/agentflow`（Windows 为 `.exe`），`dbPath` 对应为 `~/.dsh/agentflow/agentflow.db`。**严禁/不再推荐放入技能目录**（如 `~/.dsh/skills/agentflow/bin/`）。
- **Windows 平台文件锁风险**：Windows 下运行中的进程与其打开的文件会被系统内核施加强制排他文件锁（Exclusive File Lock）。若将二进制或 `.db` 放入技能目录 `~/.dsh/skills/agentflow/`，当 DSH 正在运行或打开会话时，执行技能更新、`sync-skill` 脚本、`pack-skill` 打包或二次构建时都会因无法写入/替换被锁定的文件而崩溃（报错 `EBUSY: resource busy or locked` 或 `Access is denied`）。将二进制与数据库放置在独立的 `~/.dsh/agentflow/` 即可实现技能文档资源与运行期程序/数据的彻底解耦。
- **TypeScript MCP SDK stdio 默认帧为 NDJSON**：官方 TypeScript MCP SDK（`@modelcontextprotocol/sdk`，即 DSH 底层 `dsh-mcp-client` 所用）在 stdio 传输上**默认采用换行分隔 JSON (NDJSON)** 帧，而非 HTTP 风格的 `Content-Length: ...\r\n\r\n` 头部封包。Agentflow 自 **v0.2.8** 起原生支持自适应双模帧（自动检测并兼容 NDJSON 和 Content-Length 帧）。若使用 v0.2.7 及更早构建，会因为无法解析 NDJSON 帧导致进程挂起超时无响应，请确保升级到 v0.2.8+。

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
# 服务器侧冒烟（不经 DSH；支持 NDJSON 与 Content-Length 双模帧）
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"probe","version":"1.0"}}}' \
  | ~/.dsh/agentflow/bin/agentflow stdio
# Windows 下使用 C:\Users\YOU\.dsh\agentflow\bin\agentflow.exe stdio
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

## 六、DSH agent 预设（agentflow-leader / agentflow-worker）

本仓库在 `skills/agentflow/agents/` 下提供两套**项目无关**的 DSH agent preset，
把 agentflow 的 Leader/Worker 协议深度注入任意项目的 Agent：

| Preset | 目录 | 角色 | 核心边界 |
|--------|------|------|----------|
| `agentflow-leader` | `skills/agentflow/agents/agentflow-leader/` | **Leader / 编排** | 不代做（不写产品代码、不自己 commit/submit、不亲自调试/查库） |
| `agentflow-worker` | `skills/agentflow/agents/agentflow-worker/` | **Worker / 实现** | 不 orchestrate（不拆 DAG、不派发、不 spawn 子代理） |

两者均基于 DSH 的 `standard` 预设拷贝，保留完整编码能力，仅改写人格与工具边界，
并各携带一份可加载的深度参考技能（`agentflow-leader` / `agentflow-worker`）。
预设**不内置任何业务域**；项目自身的 Worker 名册以仓库 `AGENTS.md` / `CLAUDE.md` 为准。

### 安装到本地 `~/.dsh`

```bash
mkdir -p ~/.dsh/.agent-presets
# 把两个预设目录复制到 DSH 本地预设根
cp -a skills/agentflow/agents/agentflow-leader ~/.dsh/.agent-presets/
cp -a skills/agentflow/agents/agentflow-worker ~/.dsh/.agent-presets/
```

创建 DSH 会话时选择对应预设（显示名「AgentFlow · Leader」/「AgentFlow · Worker」）。

> ⚠️ **装后校验：`cp -a` 之后必须比对 sha256，不一致一律视为安装失败。**
>
> preset 镜像是**人工维护**的：`scripts/sync-skill.ps1` 只同步 `bt_service` /
> `trees` / `requirements.txt`，**完全不碰 preset**。也就是说仓库里的
> `agent.cordis.yml` 与 `~/.dsh/.agent-presets/` 下的 live 副本之间
> **目前没有任何自动同步机制**，脱钩了不会有任何报错——只会静默生效一份错误的预设。
> 所以每次安装/更新后都要逐 preset 校验（在仓库根执行）：
>
> ```bash
> for p in agentflow-leader agentflow-worker agentflow-dev agentflow-dev-leader; do
>   repo=$(sha256sum "skills/agentflow/agents/$p/agent.cordis.yml" | cut -c1-64)
>   live=$(sha256sum "$HOME/.dsh/.agent-presets/$p/agent.cordis.yml" | cut -c1-64)
>   if [ "$repo" = "$live" ]; then echo "OK   $p $repo"
>   else echo "FAIL $p repo=$repo live=$live"; fi
> done
> ```
>
> Windows（PowerShell）等价写法：
>
> ```powershell
> foreach ($p in 'agentflow-leader','agentflow-worker','agentflow-dev','agentflow-dev-leader') {
>   $repo = (Get-FileHash "skills\agentflow\agents\$p\agent.cordis.yml" -Algorithm SHA256).Hash
>   $live = (Get-FileHash "$env:USERPROFILE\.dsh\.agent-presets\$p\agent.cordis.yml" -Algorithm SHA256).Hash
>   if ($repo -eq $live) { "OK   $p" } else { "FAIL $p repo=$repo live=$live" }
> }
> ```
>
> 出现任何 `FAIL` 即为安装失败：**不要用那份预设创建会话**，先让仓库镜像与 live
> 对齐（或反过来把 live 收敛回仓库），再重跑校验。

### 关键工具：`spawn_worker`

DSH 的子 Agent 通过 `composeFrom` 固定 join 父预设，**无按调用换 preset 的机制**。
因此 `agentflow-leader` 内配一个专属 `spawn_worker` 工具实例，用固定
`persona` + `toolFilter` 达成「Worker 人格 + 无编排工具」的边界。

`toolFilter.deny` 的**实际值恰为 5 项**，一项不多一项不少：

```yaml
toolFilter:
  deny:
    - workflow
    - ralph
    - send_message
    - interrupt_agent
    - list_agents
```

- 生成领域 Worker → 用 `spawn_worker`；调研/一般子任务 → 用 `subagent`/`subagent_fork`。

### 🚫 严禁在 `toolFilter.deny` 里点名本 preset 自己贡献的工具

> **`subagent` / `subagent_fork`（以及任何本 preset 自己提供的工具名）
> 绝对不能出现在 `toolFilter.deny` 里。**
>
> 这不是"多禁一个工具"的降级行为，而是**整条委派通道直接宕掉**。

**机理**：DSH 的工具限制由 `@deepseek-ai/dsh-tools` 的 `tools.restrict()` 执行。
它**只接受"可限制集合"（`restrictableNames`）内的名字**，名字不在集合里就**抛错**，
而不是静默忽略。源码注释原文（`@deepseek-ai/dsh-tools/lib/index.js`，
`restrict()` 上方，约 2783–2789 行）：

> Restrict global tools for the calling agent scope. Empty filters, unknown
> names, scope-local names, and reserved transport names fail. Restrictions
> intersect; scoped registrations remain visible.

同文件 `restrict()` 体内的抛错点（约 2801–2803 行）：

```js
const known = this.view(scope).restrictableNames;
const unknown = [...allow ?? [], ...deny ?? []].filter((name) => !known.has(name));
if (unknown.length > 0) throw new Error(`tools.restrict() names unknown global tool${...} ...`);
```

**为什么 `subagent` / `subagent_fork` 会踩中这个 `throw`**：
它们**正是本 preset 自己贡献的工具**（由 `agentflow-leader` 里那几个
`@deepseek-ai/dsh-tool-subagent` 行注册），因而不在"可限制集合"里。
更隐蔽的是，`@deepseek-ai/dsh-tool-subagent` 对这两个工具**只在
`subagent/provider-added` 事件上注册**：

```js
runtimeCtx.on("subagent/provider-added", (subagentProvider) => {
  if (subagentProvider.name === config.provider && mounted === void 0) mount(subagentProvider);
});
const present = runtimeCtx.subagents.getProvider(config.provider);
if (present !== void 0) mount(present);
else runtimeCtx.logger.info(`subagent provider "..." not registered yet; the "..." tool will register when it appears`);
```

即 provider 未就绪时**静默不注册**，只打一条 `info` 日志——所以工具名可能
**连存在都不存在**。而真实注册路径 `mount()` 内部的
`runtimeCtx.tools.register(...)` 一旦拿到与 deny 冲突的名字，就会在
`restrict()` 处**抛错 → 工具实例构造失败 → 整个委派通道全废**
（Leader 无法派发任何子代理；是报错，不是降级）。

**正确做法**：deny 里只写**编排类全局工具名**，绝不写本 preset 自己的贡献物。
上面那 5 项就是当前 live 与仓库镜像的唯一正确取值。

完整根因、复现与排查记录见
[`KERNEL_PRESET_TOOLFILTER.md`](./KERNEL_PRESET_TOOLFILTER.md)。

> ⚠️ **`agentOptions` 里钉的 `provider` / `model` 必须真实存在于目标机器的
> `~/.dsh/settings.yaml`，否则该通道每次用都失败。**
> 这不是"回退到默认路由"，而是**每次调用都失败**——preset 内的原注释写得很直白：
>
> > The provider/model names here MUST exist in this deployment's
> > `~/.dsh/settings.yaml` (llm-pi-ai.providers). If the names differ, rename
> > them; if the deployment has no second provider/model at all, DELETE this
> > whole `- id: ...-worker-alt` block — a spawn pinned to a
> > missing provider/model **fails on every use**.
>
> **本仓库曾有一份现成反例**：仓库镜像把 alt 通道钉成
> `provider: opencode2` / `model: glm-5.3-flash`，而该机器的 `settings.yaml`
> 里只有 `opencodego2` / `opencodego1`，**根本不存在 `opencode2` 这个 provider**
> —— 那条 alt 通道形同报废。正确取值示例：`cliproxy-google` /
> `kr/deepseek-v4.1-flash`。改 preset 前请先确认 `settings.yaml` 里存在该
> provider 与该 model。

> ⚠️ **stamp 陷阱：`dsh-agent-presets` 的代际 stamp 只以组装文件为键。**
> 预设"代际"仅由组装产物 **`agent.cordis.yml`** 决定，依据是它的
> `{ mtimeMs, size }`（`dsh-agent-presets` 中
> `COMPOSITION_FILE = "agent.cordis.yml"`，
> `compositionStamp()` 对 `preset.path` 取 stat，`sameStamp()` 只比这两项）。
> 因此**旁边顺手改的 `SKILL.md` / `preset.yml` / `README.md` 不会让新会话感知到
> 变化**（`preset.yml` 只是 `METADATA_FILE`，不参与 stamp），必须等到
> `agent.cordis.yml` **本身变动**（mtime 或 size 改变）**或进程重启**为止。
> 修 preset 时请直接改组装文件本身，不要指望改旁边文件能热生效。

### 完整说明

每个预设目录与 `skills/agentflow/agents/` 均带 `README.md`，含角色/边界/使用方式。

## 七、Hub MCP（`mcp__hub__*`）—— 试验性多人同步上下文

除 agentflow 自带的四个 hub 工具外，DSH 还可挂载**独立的 Hub MCP server**，
把 hub 平台（https://hub.stifer.xyz）的完整能力暴露给会话。两者分工不同：

| 工具面 | 来源 | 职责 |
|--------|------|------|
| `mcp__agentflow__hub_login` / `hub_list_teams` | agentflow MCP server 内置 | **设备码登录**（两段式拿 JWT，落 `~/.agent-hub/config.json`）+ 列出自己的团队码（需 JWT） |
| `mcp__agentflow__hub_status` / `hub_bind_team` | agentflow MCP server 内置 | 用已有 JWT 查询/绑定 namespace ↔ 团队码 |
| `mcp__hub__*` | 独立 Hub MCP server（`hub-mcp/index.js`） | 云端协作操作（登录能力与内置 `hub_login` 重叠，二选一即可） |

### 挂载方式（`~/.dsh/profiles/web/cordis.patch.yml`）

```yaml
- insert:
    - id: mcp-hub
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        serverName: hub
        transport: stdio
        command: node
        args: [<path>/hub-mcp/index.js]
        env:
          HUB_API_URL: https://hub.stifer.xyz
```

### 工具清单

- `hub_login`：设备授权登录（唯一合法 JWT 来源；agentflow 不做登录）
- `hub_heartbeat` / `hub_list_workers`：Worker 在线/离线/stale 监控
- `hub_acquire_lock` / `hub_release_lock` / `hub_renew_lock`：分布式锁（跨机器防冲突）
- `hub_create_playbook` / `hub_search_playbooks`：**跨用户共享的经验库**（全文搜索）
- `hub_append_event` / `hub_list_events`：团队事件流
- `hub_sync_dag` / `hub_get_dag`：DAG 任务状态软同步到云端看板
- `hub_add_repo`：绑定 GitHub 仓库

### ⚠️ 试验性定位（如实说明）

- 共享的是**沉淀产物**（经验/锁/事件/看板/心跳），**不是**实时会话上下文——
  各自会话的进行中对话与工作现场仍在各自本地，互不可见。
- 同团队多用户通过同一个 4 位 `business_code` 协作；绑定关系见 `HUB_SOFT_SYNC.md`。
- JWT 有效期约 3 天；`invalid token` 时重新 `hub_login` 即可（token 存
  `~/.agent-hub/config.json`，只存 JWT，不存团队码）。

## 八、版本与发布

- 分支 `deepseek/dsh-support` 与 master 并行维护；master 的引擎修复会定期合入
- Release 建议使用 `v0.2.8+` 系列，以获得 stdio 双模自适应帧原生支持
- 构建与打包沿用 `scripts/build-release.sh`（同一 Go 二进制，无宿主差异）
