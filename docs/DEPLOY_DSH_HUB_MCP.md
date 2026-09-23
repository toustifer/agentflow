# 把 agent-hub 的 MCP 服务器接进 DeepSeek Harness (DSH)

> DAG: `dag-hub-login-and-mcp` · Task: `task-3-dsh-hub-mcp-wiring`
> 目标文件（**在仓外**）：`%USERPROFILE%\.dsh\profiles\web\cordis.patch.yml`
> 接线日期：2026-09-23

## 一、这份文档解决什么问题

`deepseek/dsh-support` 分支的 agentflow 侧只暴露了两个 hub 工具
（`hub_login`、`hub_list_teams`，见 `task-1` / `pkg/server/hub_login_tools.go`）。
而 **agent-hub 自己**就带一台完整的 MCP 服务器
（`D:\myprogram\agent-hub\mcp-server`），暴露 **24 个 `hub_*` 工具**：
设备登录、我的团队、Worker 心跳、分布式锁（acquire/release/renew/list）、
事件（append/list）、Playbook（create/search）、分支索引（report/bind/list/refresh）、
DAG 同步、仓库绑定、邀请与链接审批等。

本任务把**那台服务器**以 stdio 方式注册进 DSH 的 `web` profile，
使模型侧拿到 `mcp__hub__<toolName>` 形态的完整工具面。

## 二、前置依赖（缺失就不要接）

| 依赖 | 必需性 | 检查命令 |
|------|--------|----------|
| `node` 可执行文件 | 必需 | `node --version`（实测 `v22.21.1`，路径 `C:\nvm4w\nodejs\node.exe`） |
| `agent-hub\mcp-server\index.js` | 必需 | `Test-Path 'D:\myprogram\agent-hub\mcp-server\index.js'` |
| `agent-hub\mcp-server\node_modules`（含 `@modelcontextprotocol/sdk`） | **必需** | `Test-Path 'D:\myprogram\agent-hub\mcp-server\node_modules'` |

`mcp-server\package.json` 声明 `"type": "module"`，`index.js` 是 ESM +
顶层 `await`，因此**必须**用 Node 18+ 启动；用 CommonJS 包裹或旧版 Node 会直接语法失败。

> ⚠️ `node_modules` 不存在时**不要接线**——服务器起不来，而
> `failOnStartupError: false` 会让 DSH 静默降级成「没有这些工具」，很难排查。
> 先 `cd D:\myprogram\agent-hub\mcp-server; npm install` 再回到这里。

## 三、接线步骤

### 1. 备份（**不可跳过**）

```powershell
$ts  = Get-Date -Format 'yyyyMMddHHmmss'
$src = "$env:USERPROFILE\.dsh\profiles\web\cordis.patch.yml"
Copy-Item -LiteralPath $src -Destination "$src.bak-$ts"
(Get-Item "$src.bak-$ts").Length          # 记录字节数
(Get-FileHash -LiteralPath $src).Hash     # 记录 SHA256
```

### 2. 编辑：**只做按行插入，不要反序列化再序列化**

`cordis.patch.yml` 是别人的 profile 层，里面有大量中文注释。
用 YAML 库 `parse` → `stringify` 会把注释全部吃掉，**禁止**这种做法。

在 `mcp-reverse-flow` 之后、`mcp-zotero` 之前插入：

```yaml
# agent-hub MCP — Hub 自身的 MCP 服务器，暴露完整 hub_* 工具面
# （hub_login / hub_list_my_businesses / 分布式锁 / Playbook / 事件 / 分支上报等 24 个工具）。
# 由 dag-hub-login-and-mcp 的 task-3 接线；接线步骤/验证/回滚见 docs/DEPLOY_DSH_HUB_MCP.md。
# 依赖：D:\myprogram\agent-hub\mcp-server\node_modules 必须存在（@modelcontextprotocol/sdk）。
- insert:
    - id: mcp-hub
      name: '@deepseek-ai/dsh-mcp-client'
      config:
        serverName: hub
        transport: stdio
        command: 'C:\nvm4w\nodejs\node.exe'
        args:
          - 'D:\myprogram\agent-hub\mcp-server\index.js'
        failOnStartupError: false
        env:
          HUB_API_URL: 'https://hub.stifer.xyz'
```

**为什么用 `failOnStartupError: false`**：与 `mcp-ldxp` / `mcp-reverse-flow` 一致。
这些是**按需**能力，起不来不应连带把整个 web profile 拖垮。
代价是启动失败不会报错——所以下面的「验证」步骤必须真的跑。

**为什么显式给 `HUB_API_URL`**：`lib/config.js` 的取值顺序是
`process.env.HUB_API_URL` → `~/.agent-hub/config.json: hub_url` → 硬编码
`https://hub.stifer.xyz`。显式写死可以和本机配置文件解耦，避免
本地 config 被改到别处时静默连错后端。

### 3. 校验 YAML 仍合法

```powershell
node -e "const Y=require('yaml');const fs=require('fs');const d=Y.parseDocument(fs.readFileSync(process.env.USERPROFILE+'/.dsh/profiles/web/cordis.patch.yml','utf8'),{strict:true});if(d.errors.length){console.error(d.errors);process.exit(2)}console.log('OK top-level entries =',d.toJS().length)"
```

解析失败 → **立即回滚**（见第五节）。

### 4. 校验 profile 仍能加载

`--dump-config` 会合成整棵 profile 树然后退出，**不启动任何 app、不碰任何进程**，
是这里最安全的加载性检查：

```powershell
dsh --dump-config --profile web | Select-String 'mcp-hub' -Context 0,12
```

再加一条更贴近真实解析路径的检查：

```powershell
dsh plugin --profile web list
```

### 5. 实测 stdio 服务器（端到端，最有说服力）

用最小 MCP 客户端向服务器发 `initialize` + `tools/list`：
脚本是仓内的 `smoke/hub_mcp_probe.mjs`（NDJSON 帧，与
`@modelcontextprotocol/sdk` 的 `StdioServerTransport` 一致，零依赖）。

```powershell
cd D:\myprogram\agentflow
node smoke\hub_mcp_probe.mjs 'D:\myprogram\agent-hub\mcp-server\index.js' 'C:\nvm4w\nodejs\node.exe'
```

期望：`HAS_hub_login=true`，工具数为 **24**，退出码 `0`。

### 6. 让 DSH 生效

`cordis.patch.yml` 属于 profile 层，DSH 的 HMR 会感知 loader 变化；
**但新 MCP 服务器实例的连接与工具注册发生在会话/插件激活期**。
保守做法：**开一个新会话**（或重载窗口）再确认 `mcp__hub__*` 工具出现。

> 本节是**未确证项**，详见第六节。

## 四、验证清单（可复制粘贴）

```powershell
$src = "$env:USERPROFILE\.dsh\profiles\web\cordis.patch.yml"

# AC1 备份存在
Get-ChildItem "$src.bak-*" | Select-Object FullName,Length

# AC2 条目形状
Select-String -Path $src -Pattern 'mcp-hub' -Context 2,12

# AC3 YAML 合法（节点数）
node -e "const Y=require('yaml');const d=Y.parseDocument(require('fs').readFileSync(process.argv[1],'utf8'));console.log('nodes',Y.visit(d,()=>{}))" $src

# AC4 profile 可加载
dsh --dump-config --profile web | Select-String 'mcp-hub'
dsh plugin --profile web list

# AC5 stdio 实测
node smoke\hub_mcp_probe.mjs
```

## 五、回滚预案（一条命令）

```powershell
Copy-Item -LiteralPath "$env:USERPROFILE\.dsh\profiles\web\cordis.patch.yml.bak-<yyyyMMddHHmmss>" -Destination "$env:USERPROFILE\.dsh\profiles\web\cordis.patch.yml" -Force
```

回滚后验证：

```powershell
Select-String -Path "$env:USERPROFILE\.dsh\profiles\web\cordis.patch.yml" -Pattern 'mcp-hub'   # 应无输出
dsh --dump-config --profile web                                                                # 应 EXIT=0
```

备份是**逐字节副本**，`-Force` 覆盖即可完全恢复原状；已在接线时做过演练
（复制到临时路径后 SHA256 与备份一致）。

**回滚是纯仓外操作，不需要动 git。**

## 六、未解未知点（如实记录，不要当成已解决）

1. **新工具是否对「当前已打开的会话」立即可见 —— 不确定。**
   已确证的是：配置文件合法、`--dump-config` 能合成出 `mcp-hub` 条目、
   服务器本体用配置里的确切命令能起并列出 24 个工具。
   **未确证**的是：DSH 的 HMR 是否会重建这个 mcp-client 实例并把新工具
   注入一个**已经存在**的会话。建议按「开新会话」使用；不要断言当前会话已可用。

2. **「备选方案：远程 Streamable HTTP」的字段名 —— 已查清（不再是未知点）。**
   从实现里读到权威定义
   （`@deepseek-ai/dsh\node_modules\@deepseek-ai\dsh-mcp-client\lib\types\index.d.ts`）：

   ```ts
   export interface StreamableHttpConfig {
       transport: 'streamable-http';   // 连字符，不是 streamable_http
       serverName: string;
       url: string;                    // 字段名就是 url（不是 endpoint / baseUrl）
       headers: Record<string, string>;
       toolCallTimeoutMs: number;
       failOnStartupError: boolean;
       reconnect?: ReconnectConfig;
   }
   ```

   也就是说**首选方案之外的路确实走得通**，形状与 profile 里已有的
   `mcp-zotero` 条目完全一致。`agent-hub` 另有 `http-server.js`
   （裸 JSON-RPC，`GET` 返回工具清单、`POST` 处理 `tools/list` 与 `tools/call`），
   可自托管后用 `transport: streamable-http` + `url:` 接入。
   本次**没有采用**这条路：它是自建的精简 HTTP 层，不是标准
   Streamable HTTP/SSE 端点，与官方 SDK 客户端握手不保证兼容；
   stdio 直连官方 SDK 服务器更稳。

3. **`serverName` 的取值影响工具公开名**：注册名是
   `mcp__<serverName>__<rawName>`，即 `serverName: hub` →
   `mcp__hub__hub_login`。改 `serverName` 等于改全部工具名，
   会打断任何已写好的提示词/脚本。要改就得整体改。

## 七、变更记录

| 日期 | 变更 | 执行者 |
|------|------|--------|
| 2026-09-23 | 首次接线：新增 `mcp-hub`（stdio，24 工具）；备份 `cordis.patch.yml.bak-20260923110719`（4142 B） | `task-3-dsh-hub-mcp-wiring` |
