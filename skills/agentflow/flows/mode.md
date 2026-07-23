# agentflow sticky mode

让 `/agentflow` 在会话里“保持开启”，接近 goal 插件的 always-on 体验。

## 重要限制

Claude Code **不能**在输入框 UI 上挂住 `agentflow` 文本前缀/chip。

本 flow 实现的是等价能力：

```text
/agentflow on  -> 写 .claude/agentflow/mode.json
每一轮 prompt  -> UserPromptSubmit hook 注入 agentflow 规则
statusline     -> 读取 .claude/agentflow/status.json 并显示 agentflow 摘要
resume/inspect -> 通过 hooks/render-inspect.js 刷新 status.json 并输出树状视图
/agentflow off -> 关闭 mode 文件
```

## 命令

### `/agentflow on`

1. 确认当前项目目录（`CLAUDE_PROJECT_DIR` 或 cwd）
2. 运行：

```bash
node <skill>/hooks/mode-cli.js on
```

可选参数：

```bash
node <skill>/hooks/mode-cli.js on --namespace <ns> --dag <dag_id> --note "..."
```

3. 向用户确认：
   - mode 已开启
   - mode 文件路径
   - 若 hook 未安装，提示按 `SETUP.md` 的 sticky mode 段配置 `UserPromptSubmit`
   - **MCP 探测结果**（`mode-cli` 输出的 `mcp` + `warnings`）：
     - `configured: false` → 明确说：mode 开了但 **MCP 未配置**，先装/配 agentflow，打开 `/mcp` 直到出现 agentflow 且非 failed；**禁止** Bash/JSON-RPC 旁路干活
     - `paths_ok: false` → 配置路径坏了，先修 `~/.claude.json` 再重启
     - 配置看起来 OK → 仍提醒：必须在本会话看到 `mcp__agentflow__*` 工具；`claude mcp list` Connected 不够
4. 不要在 on 时强行进入 goal；只打开模式。
5. 若 MCP 未就绪：下一步只能是 setup/修 MCP，**不要**自动 `/agentflow resume` 或 goal。

### `/agentflow off`

```bash
node <skill>/hooks/mode-cli.js off
```

确认 mode 已关闭。关闭后 hook 不再注入上下文。

### `/agentflow status`（可选）

```bash
node <skill>/hooks/mode-cli.js status
```

显示 enabled / path / namespace_id / dag_id。

## mode 文件格式

路径：`<project>/.claude/agentflow/mode.json`

```json
{
  "enabled": true,
  "enabled_at": "2026-07-13T00:00:00.000Z",
  "updated_at": "2026-07-13T00:00:00.000Z",
  "project_dir": "D:/myprogram/foo",
  "namespace_id": "optional",
  "dag_id": "optional",
  "note": "optional"
}
```

## hook 注入内容（概要）

当 `enabled=true` 时，`hooks/mode-inject.js` 每轮注入：

- **MCP 硬门禁（Rule 0）**：无本会话 `mcp__agentflow__*` 时立即停止业务；禁止 Bash/stdio JSON-RPC/sqlite 旁路；要求用户先 `/mcp` 修好
- 配置探测 banner：`MCP:missing` / 路径坏掉时注入 CONFIG ALERT
- MCP 通过后才：只用 `mcp__agentflow__*` 工具
- 当前会话应把普通自然语言请求也默认视为 agentflow 工作，除非用户明确要求离开 agentflow mode
- 每个新对话轮都应视为同一 agentflow working session，优先路由到 `resume` / `inspect` / `goal` / 当前激活 flow，而不是先走自由回答
- 新的项目目标应直接落回 agentflow 的 goal/intake 主链，而不是脱离工作流单独处理
- 任务引用必须 `task_id + title`
- start worker 必须走 launch-ticket 协议：`prepare_start -> spawn Agent -> start(ticket, worker_agent_id) -> sync`
- **Leader 禁止代写产品代码**；prepare/start 失败只能修 worktree/分支占用或 escalate
- 主仓留在 `base_branch`；`execution_branch` 只在 DAG worktree
- DAG 拥有 shared worktree lease，task 只持有阶段性租约
- **`agentflow:on` ≠ MCP OK**

## 安装检查清单

Leader/用户第一次用 sticky mode 时确认：

1. `hooks/mode-inject.js` 已在 `UserPromptSubmit` 注册
2. `node hooks/mode-cli.js on` 能写出 mode 文件，且输出含 `mcp` 探测与 `warnings`
3. **`/mcp` 列出 `agentflow` 且非 failed**；本会话能调用 `mcp__agentflow__flow_ping`
4. 下一轮普通用户消息后，模型行为仍保持 agentflow 约束；MCP 缺失时会停在 setup 话术而不是旁路
5. statusline：`agentflow:on` 旁应有 MCP badge
   - `MCP:missing` / `MCP:broken`（红）= 先修配置
   - `MCP:cfg`（黄）= 配置在，仍须会话内工具可用
6. 若项目里已有 `.claude/agentflow/status.json`，statusline 应升级成两级摘要：
   - 第 1 行：`agentflow:on · MCP:… · dag:<title|id> · working:<n> · ready:<n> · blocked:<n>`
   - 第 2 行：`<phase> <progress> · workers:<busy>/<total> · busy:<top workers>`（MCP 坏时追加 `fix MCP before work`）
   其中 `busy:<top workers>` 最多显示 2 个 busy worker，超出部分折叠成 `+N`
   若没有 snapshot，则回退到静态 badge

详细配置见 `SETUP.md` 的 Sticky Mode 段。

## 与业务 flow 的关系

| 命令 | 作用 |
|---|---|
| `on` / `off` / `status` | 只切换会话模式，不创建 DAG/task |
| `resume` / `goal` / `init` | 业务 flow；可在 mode on 期间使用 |
| 普通自然语言 | mode on 时仍自动带 agentflow 规则 |

推荐日常用法：

```text
/agentflow on
... 正常对话推进项目 ...
/agentflow resume   # 需要明确恢复快照时
/agentflow off      # 离开 agentflow 工作面
```
