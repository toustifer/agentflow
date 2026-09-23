# Leader preset `toolFilter.deny` 与 `tools.restrict()` 的判定规则

> 事故/任务：`task-1-toolfilter-deny-fix`（DAG `dag-fix-leader-preset-toolfilter`，P0）
> 影响面：`agentflow-leader` preset 的三个委派通道（`spawn_worker` / `spawn_worker_alt`，以及 Worker 子代理自身）
> 改动文件（**仓外**，不在本仓库）：`C:\Users\15775\.dsh\.agent-presets\agentflow-leader\agent.cordis.yml`

---

## 1. `restrict()` 的权威判定规则

来源：`@deepseek-ai/dsh-tools`，`lib/index.js` 第 2783–2789 行（**源码注释原文，逐字引用**）：

```js
	/**
	* Restrict global tools for the calling agent scope. Empty filters, unknown
	* names, scope-local names, and reserved transport names fail. Restrictions
	* intersect; scoped registrations remain visible.
	* @param filter - global-tool mask: `allow` (keep only) and/or `deny` (remove).
	* @returns the exact disposer that lifts this restriction.
	*/
	restrict(filter) {
```

即：**空过滤器、未知名字、作用域本地（scope-local）名字、保留传输名，全部直接抛错。**

抛错点在同文件第 2801–2803 行：

```js
		const known = this.view(scope).restrictableNames;
		const unknown = [...allow ?? [], ...deny ?? []].filter((name) => !known.has(name));
		if (unknown.length > 0) throw new Error(`tools.restrict() names unknown global tool${unknown.length > 1 ? "s" : ""} ${unknown.map((n) => `"${n}"`).join(", ")}; known global tools: ${[...known].sort().join(", ") || "(none)"}`);
```

这个错误信息里的措辞是 **`unknown global tool(s)`** —— 这正是各种转述走样的源头。

### 1.1 `restrictableNames` 到底是什么集合（关键，易误读）

同文件第 2839–2850 行的注释说得很清楚（原文引用）：

```js
	* A restriction filters what a scope inherits — the global layer and every
	* ancestor layer on its chain — and never what its OWN layer registers.
	* That exemption is what a per-child capability filter has to keep intact:
	* the delegation runtime registers a child's structured-output tool into the
	* child's own layer, and a filter naming the capabilities the child may use
	* must not strip the machinery it answers through.
	*
	* Reading the exempt set as "the global layer" instead of "not mine" held
	* only while every model-facing tool sat in the host composition. Once
	* presets moved them onto the agent plane they became an ANCESTOR
	* contribution, so a child's filter silently stopped constraining anything
	* it was given.
```

对应实现（第 2854–2869 行）：

```js
	view(scope) {
		const layers = this.layers.chainLayers(scope);
		const own = this.layers.peek(scope);
		const inherited = new Map(this.layers.global.tools.entries());
		for (const layer of layers) {
			if (layer === own) continue;
			for (const [name, definition] of layer.tools.entries()) inherited.set(name, definition);
		}
		...
		for (const [name, definition] of inherited) {
			knownNames.add(name);
			restrictableNames.add(name);
			...
```

**结论**：`restrictableNames` = 「global 层 + 该 scope 链上所有 **ancestor** 层」的可继承表面，**不含该 scope 自己那一层**（自己的层被显式 `continue` 跳过）。

所以 `deny` 能点名的前提是：**该名字此刻确实作为「继承面」被注册出来了**。名字没注册出来（或注册晚了），就落到第 2802–2803 行的 `unknown` 分支 → `restrict()` 抛错 → 调用方（preset 里的工具实例构造）失败。

---

## 2. 真实根因

### 2.1 事实链

1. `agentflow-leader` preset 在 `delegation` 组里挂了两个实例，贡献 `subagent` / `subagent_fork`（preset 第 282–298 行）：

   ```yaml
       - id: tool-subagent
         name: '@deepseek-ai/dsh-tool-subagent'
         config:
           provider: spawn
           toolName: subagent
           ...
       - id: tool-subagent-fork
         name: '@deepseek-ai/dsh-tool-subagent'
         config:
           provider: fork
           toolName: subagent_fork
           ...
   ```

2. `@deepseek-ai/dsh-tool-subagent` 里，这两个工具名**不是**无条件注册的。`lib/index.js` 第 565–575 行：

   ```js
   		runtimeCtx.on("subagent/provider-added", (subagentProvider) => {
   			if (subagentProvider.name === config.provider && mounted === void 0) mount(subagentProvider);
   		});
   		runtimeCtx.on("subagent/provider-removed", (name) => {
   			if (name !== config.provider || mounted === void 0) return;
   			mounted.disposeTool();
   			mounted = void 0;
   		});
   		const present = runtimeCtx.subagents.getProvider(config.provider);
   		if (present !== void 0) mount(present);
   		else runtimeCtx.logger.info(`subagent provider "${config.provider}" not registered yet; the "${config.toolName ?? "subagent"}" tool will register when it appears`);
   ```

   工具名由 `runtimeCtx.tools.register(...)`（第 396–399 行）在 `mount()` 里注册，而 `mount()` 只在对应 `provider`（`spawn` / `fork`）**出现时**才被调用；provider 尚未出现时只打一条 **info 日志**，**不报错**。

3. 于是：`provider` 注册失败 / 尚未注册（例如同时挂载两个都声明 `provider: spawn` 的 Leader preset，或组合与时序不满足）→ `subagent` / `subagent_fork` **没进注册表** → 它们不在 `restrictableNames` 里。

4. 而 preset 第 353–361 行（`spawn_worker`）与第 415–423 行（`spawn_worker_alt`）的 `toolFilter.deny` **点名**了这两个名字 → `restrict()` 命中 `unknown` 分支抛错 → 该工具实例**构造失败** → **委派通道整条死掉**。

### 2.2 为什么报错只点了这两个名字

报错只点 `subagent` / `subagent_fork`，**没有点** `workflow` / `ralph` / `send_message` / `interrupt_agent` / `list_agents`。原因是这两类名字来源不同：

| 名字 | 来源 | 稳定性 |
| --- | --- | --- |
| `subagent`, `subagent_fork` | **本 preset 自产**（`tool-subagent` / `tool-subagent-fork` 两个实例，第 282–298 行），且**条件 + 异步**注册 | 脆：依赖 provider 出现时序 |
| `workflow`, `ralph`, `send_message`, `interrupt_agent`, `list_agents` | host 全局面（`dsh-tool-subagent-control` 等稳定挂载） | 稳 |

这条「只点前两个」的分布，本身就是「前两个是 preset 自产、后五个是 host 全局」的现场证据。

### 2.3 对一处错误转述的明确更正

> **原转述的「这两个工具根本不存在，deny 它们没有任何实际效果」是错的。**

错在两点：

1. **它们不是「不存在」的工具。** 它们由**同一个 preset 的第 282–298 行**贡献（`tool-subagent` 与 `tool-subagent-fork` 两个实例），是**定义好、被点名、带完整 `toolName` 配置**的一等工具。它们只是**注册是条件式的**（依赖 `spawn` / `fork` provider 出现）。
2. **deny 它们不是「没有实际效果」，而是「有灾难性效果」。** 因为校验发生在 `restrict()` 里，名字缺失不是被静默忽略，而是**抛错**，进而让整个工具实例构造失败。也就是说：这两个名字一旦缺位，deny 列表里写它们不是「多写一行废话」，而是**把整条委派通道炸掉**。

「unknown global tool」里的 `global` 措辞容易让人误读成「这是 host 全局工具、preset 里没有」，但按第 2839–2850 行的定义，它实际指的是「**该 scope 的继承面**（global + ancestor 层）」，preset 注册到祖先层的工具也算。**错误转述正是踩在这个词上。**

---

## 3. 本次改动

`C:\Users\15775\.dsh\.agent-presets\agentflow-leader\agent.cordis.yml`，**只删 4 行**：

- 第 355–356 行：`- subagent` / `- subagent_fork`
- 第 417–418 行：`- subagent` / `- subagent_fork`

两个 `deny` 列表各从 7 项变 5 项，保留的 5 项（`workflow` / `ralph` / `send_message` / `interrupt_agent` / `list_agents`）**原样未动**。文件其余字节**逐字节不变**。

- 字节数：`33355` → `33253`（`-102`，正好是 `2 × (23 + 28)`，即两组各 12 空格缩进的 `- subagent` + `- subagent_fork`）
- 行数：`477` → `473`（`-4`）
- 注释行数：`152` → `152`（不变）
- `git diff --no-index --numstat`：`0  4`（**+0 / −4**，纯减法）

### 3.1 本次改动的代价（必须知情）

**这不是「删掉几条无效配置」，而是「用机械强制换可用性」。**

- 在**健康会话**里，`subagent` / `subagent_fork` 确实存在于 `restrictableNames` 中，`deny` 它们**是被真正执行的** —— `restrict()` 校验能过，本身就是「它们当时确实被拦住了」的证明。也就是说，**改动前，Worker 通道在这两个工具上是「机械强制禁止」，不是形同虚设。**
- **改动后**，Worker 只剩 **人格层禁令**：preset 里 `persona` 文本中的「你不 spawn 子代理、不跑 workflow、不驱动 Ralph 循环来把自己的活外包出去。」（第 340 行 / 第 402 行）。这是**软约束**——靠模型自觉遵守提示词。
- 因此本改动**真实降低了 Worker 越权 spawn 的机械保障**，换来的是「provider 未就绪时委派通道不再被 deny 校验一次性炸掉」。
- 对照形态：**另一个 preset 文件** `C:\Users\15775\.dsh\.agent-presets\agentflow-dev-leader\agent.cordis.yml`（共 202 行）定义了同样的委派工具，而 **`toolFilter` 出现次数为 0**（实测：`Select-String -Pattern 'toolFilter'` 计数 = 0）→ 这本来就是**已知可用的无拦截形态**。本次改动把 `spawn_worker` / `spawn_worker_alt` 的拦截强度**降到了与 `agentflow-dev-leader` 同级**（就这两个名字而言）。
- 更深一层的取舍：`workflow` / `ralph` / `send_message` / `interrupt_agent` / `list_agents` 这五项仍被机械拦截，**Worker 依旧无法跑 workflow、无法驱动 Ralph、无法反向操控父代理**。被放弃的只是「不能 spawn 子代理」这一条的机械强制。

**若要让机制恢复完整**，正确修法**不是**把这两个名字加回来（那会重演崩溃），而是**先确保 `subagent` / `subagent_fork` 的 provider 一定在 `restrict()` 之前注册成功**（消除「同时挂载两个 `provider: spawn`」之类的组合冲突），再恢复 deny。本次任务范围仅限「止血」，不扩大到组合与时序修复。

---

## 4. 验证证据（本次交付实测）

| 检查 | 命令 | 结果 |
| --- | --- | --- |
| YAML 解析（**Node `yaml`**，AC#4 指定） | `node %TEMP%\preset_yaml_probe.mjs <bak> <live>`，`yaml` 从 DSH 自己的 `node_modules/yaml` 解析；因 preset 含 3 处 `!!js` 自定义标签，显式传入 `customTags` 声明 `tag:yaml.org,2002:js` | 备份与 live 均 `YAML_PARSE_ERRORS=0`；`LIVE_AC4_ALL_PASS=true`（exit=0） |
| YAML 解析（交叉验证，tag-tolerant `yaml.safe_load`） | `python %TEMP%\ac_validate.py <bak> <live>` | 两边均 `YAML_PARSE_ERRORS=0` |
| `toolFilter:` 计数 | 文本正则 + 结构化遍历 双重计数 | 文本 `2` / 结构化 `2` |
| 两个 `deny` 列表 | 结构化读取（路径 `[12]/config/[4]/config/toolFilter` 与 `[12]/config/[5]/config/toolFilter`） | 各 `5` 项，且 == 保留的 5 项（`workflow`/`ralph`/`send_message`/`interrupt_agent`/`list_agents`） |
| 注释行数（**注释零丢失**，AC#5） | `^\s*#` 计数 | `152` → `152`，相等（`comment_lines_equal=true`） |
| 纯减法（AC#3） | `git diff --no-index --numstat` | `0  4`（+0 / −4） |
| 尾部换行 | 字节检查 | `True`（保持） |
| 行尾风格 | 字节扫描 | `CRLF=0 LF_ONLY=473`（原为 `LF_ONLY=477`，风格未变） |
| 字节数 | `Buffer.byteLength` | `33355` → `33253`（`delta=-102`） |

> ⚠️ 注意 `dsh` **没有**校验 agent preset 的子命令。因此「改动是否生效」**只能**靠 YAML 解析 + 结构 diff + **重启后开新会话确认**。本次交付**未重启任何进程**（按任务要求，生效由用户开新会话验证）。

---

## 5. 回滚步骤

备份：`C:\Users\15775\.dsh\.agent-presets\agentflow-leader\agent.cordis.yml.bak-20260923132530`
备份 sha256：`AF9D33E62AF861D801E9A3AD96DBA8F6CC389BBAE3878B7191222D23E9628456`（33355 字节）

**回滚命令（PowerShell，一条即恢复）：**

```powershell
Copy-Item -LiteralPath "C:\Users\15775\.dsh\.agent-presets\agentflow-leader\agent.cordis.yml.bak-20260923132530" -Destination "C:\Users\15775\.dsh\.agent-presets\agentflow-leader\agent.cordis.yml" -Force
```

**回滚后校验：**

```powershell
(Get-FileHash -LiteralPath "C:\Users\15775\.dsh\.agent-presets\agentflow-leader\agent.cordis.yml" -Algorithm SHA256).Hash
# 期望：AF9D33E62AF861D801E9A3AD96DBA8F6CC389BBAE3878B7191222D23E9628456
(Get-Item -LiteralPath "C:\Users\15775\.dsh\.agent-presets\agentflow-leader\agent.cordis.yml").Length
# 期望：33355
```

回滚演练已实测：把备份复制到临时路径比对 sha256，与备份一致（`ROLLBACK_DRILL_MATCH=True`），证明该命令可用且备份完好。

---

## 6. 相关但不属于本次改动

- `agentflow-dev-leader`（`agentflow-dev-leader\agent.cordis.yml`，共 202 行）：同样三个委派工具、**无 `toolFilter`**（实测计数 0）—— 已知可用形态，**本次未改动**，也**不应**改动。
- `spawn_worker_alt` 的 `agentOptions: { provider: a6api, model: gemini-3.8-flash }`：已在 `~/.dsh/settings.yaml` 只读核实 —— provider `a6api` 定义在第 **74** 行，model `gemini-3.8-flash` 定义在第 **90** 行 —— **两者都存在，该块保留，未删**。
- `~/.dsh/settings.yaml`：**本次未改动**。
