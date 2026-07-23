# /agentflow

项目编排引擎调度器。`/agentflow` 是唯一公开入口。

`setup` / `init` / `intake` / `goal` / `resume` / `inspect` / `shape` / `mode` 现在都作为本 bundle 内部 flow 持有：

```text
agentflow/
  SKILL.md
  flows/
    setup.md
    init.md
    intake.md
    goal.md
    resume.md
    inspect.md
    shape.md
    mode.md
  hooks/
    mode-lib.js
    mode-cli.js
    mode-inject.js
    statusline.js
    render-inspect.js
  references/
    using-superpowers-adapter.md
```

## 总原则

先确认 `agentflow` MCP 是否可用，再决定进入哪个业务 flow。

## MCP 硬门禁（禁止旁路）

**有 sticky `agentflow:on` ≠ MCP 可用。** `on` 只写本地 `mode.json`；`/mcp` 里没有 `agentflow` 或显示 failed 时，**禁止**继续推进业务。

| 层 | 含义 | 能否当验收 |
|----|------|------------|
| 配置 | `~/.claude.json` / 项目 mcp 里有 `agentflow` | 否（只说明写过配置） |
| 进程 | `claude mcp list` → Connected | **否**（不等于本会话工具已注入） |
| **会话工具** | 当前模型工具列表里有 `mcp__agentflow__*` | **是** |
| UI | `/mcp` 列出 agentflow 且非 failed | 用户侧必查 |

**MCP 不可用时（无 `mcp__agentflow__*`、调用失败、`/mcp` 无 agentflow 或 failed）：**

1. **立刻停止** goal / resume / prepare / start / submit / 写产品代码。
2. **禁止**用 Bash/shell 跑 `agentflow`、`agentflow stdio`、手写 JSON-RPC `tools/call`、直接 sqlite 改 agentflow DB，当作 MCP 替代。
3. **禁止**谎报 lifecycle 成功或「已在用 agentflow」。
4. **明确告诉用户**先修好 MCP：打开 `/mcp` → 修好 `agentflow`（必要时一并修 `hub`）→ 重启 Claude Code → 确认本会话能调用 `mcp__agentflow__flow_ping`。
5. 只进入 `flows/setup.md` 做安装/修复指导，修好前不回到业务 flow。

sticky 注入（`hooks/mode-inject.js`）每轮重复上述门禁；statusline 在配置缺失时显示 `MCP:missing` / `MCP:broken`。

## Leader / Worker 执行边界

`/agentflow` 打开后，**主会话默认是 leader，不是实现工人**。

- Leader：intake / shape / plan / prepare_start / spawn Agent / transition / sync / review 协调
- Worker Agent：在 DAG `worktree_path` 内写代码、测试、commit、submit
- 主仓保持 `base_branch`；禁止在主仓 checkout `execution_branch` 后由 leader 手写交付
- prepare/start 失败时只修 git/worktree 或 escalate，**禁止**主会话代做 task

### Skill-primary 派工（唯一真相）

```text
leader_tick(namespace_id, dag_id)     # 只读 phase/next；BT dispatch = prepare-only
task_prepare_start                    # ticket + worktree；state 仍 assigned
spawn 真实 Agent
task_transition(start)                # launch.ticket + real worker_agent_id
                                      # + runtime.provider + runtime.status=started
```

- **禁止**把 `leader_tick` / BT `dispatch_task` 当成已 start
- **禁止**合成 `worker_agent_id`（如 `bt:...`）
- 多 DAG 时必须显式 `dag_id`；单 DAG 可由 server `single_auto`
- `lifecycle_tick` 仅测试/诊断 glue，不是生产 execute 主循环
- Worker BT 树的 `implement_code` / `git_commit_changes` 是 briefing / 录 metadata，不代替 Worker Agent 写代码/commit

完整派工协议见 `flows/goal.md` 的 Execute 段。

## Sticky Mode（会话保持）

Claude Code **不能**在输入框里挂住 `agentflow` 文本前缀。  
等价能力是 sticky mode：

```text
/agentflow on  -> 写 .claude/agentflow/mode.json
每一轮 prompt  -> UserPromptSubmit hook 注入 agentflow 规则
statusline     -> 可选显示 agentflow:on
/agentflow off -> 关闭
```

细节见 `flows/mode.md` 与 `SETUP.md` Sticky Mode 段。

## MCP 前置检查

在进入 `goal` 或 `resume` 之前（以及 sticky 开启后的**每一轮**业务意图），做判断：

1. 当前会话工具列表是否已有 `mcp__agentflow__*`（不是 `claude mcp list`）
2. 若有，`mcp__agentflow__flow_ping` 是否成功
3. 用户侧 `/mcp`：是否列出 `agentflow` 且非 failed

如果任一失败：
- 读取 `flows/setup.md`
- 按 setup flow 做安装/修复/验证
- **停止一切旁路**（见上节「MCP 硬门禁」）
- 不要假装业务 flow 可以继续推进

`on` / `off` / `status` **不要求** MCP 已连通（它们只读写 mode 文件），但：
- `on` 必须向用户打印 MCP 配置探测结果与 **「先修好 MCP 再干活」** 警告
- sticky 开启后若 MCP 仍不可用，每轮注入仍强制停在 setup，而不是静默干活

## 调度逻辑

```text
如果 agentflow MCP 不可用      -> 读取 flows/setup.md（on/off/status 除外）
/agentflow on [opts]           -> 读取 flows/mode.md，开启 sticky mode
/agentflow off                 -> 读取 flows/mode.md，关闭 sticky mode
/agentflow status              -> 读取 flows/mode.md，显示 mode 状态
/agentflow inspect ...         -> 读取 flows/inspect.md，查看项目/DAG/task 树状进度
/agentflow init [项目名]        -> 读取 flows/init.md
/agentflow goal [目标]         -> 先读取 flows/intake.md，再读取 flows/goal.md
/agentflow resume              -> 读取 flows/resume.md 并按 resume flow 推进
/agentflow resume dag <dag_id> -> 恢复项目现场，但强制聚焦指定 DAG
/agentflow [无]                -> 默认读取 flows/resume.md
其他                             -> 全部当作 goal，先读取 flows/intake.md，再读取 flows/goal.md
```

## 路由规则

收到 `/agentflow` 后，按下面顺序判断：

1. 如果 args 以 `on` / `off` / `status` 开头
   - 读取 `flows/mode.md`
   - 用 `hooks/mode-cli.js` 写/读 `.claude/agentflow/mode.json`
   - 不进入业务 DAG flow
   - 若 hook 未安装，提醒用户按 `SETUP.md` 配置 `UserPromptSubmit`

2. 先确认 agentflow MCP 是否可用
   - 如果不可用：读取 `flows/setup.md`

3. 如果 args 以 `init` 开头
   - 读取 `flows/init.md`
   - 把它当作已有内容项目首次接入请求
   - 在 `/agentflow` 内完成 repo 绑定、扫描、baseline 建立与后续去向判断

4. 如果 args 以 `goal` 开头
   - 先读取 `flows/intake.md`
   - 再读取 `flows/goal.md`
   - 只有 intake 接受后，才按 goal flow 进入 shape / plan / execute

5. 如果 args 以 `resume` 开头
   - 读取 `flows/resume.md`
   - 先恢复项目 snapshot 和 DAG 列表
   - 再按 resume flow 决定继续哪条线

6. 如果 args 以 `inspect` 开头
   - 读取 `flows/inspect.md`
   - 优先使用 `project_inspect(namespace_id, focus?, dag_id?, task_id?)`
   - 把返回 snapshot 通过 `node hooks/render-inspect.js` 渲染并刷新 `.claude/agentflow/status.json`
   - 输出树状项目 / DAG / task / worker / blocker 视图

7. 如果 args 为空
   - 默认读取 `flows/resume.md`
   - 默认走同一套项目恢复流程

8. 其他
   - 全部当作 goal
   - 先读取 `flows/intake.md`
   - 再读取 `flows/goal.md`

## Shape 约束

当 goal flow 进入 `shape` 阶段时：
- 前提是 `intake` 已给出 accepted / enter_shape
- 必须读取 `flows/shape.md`
- 不得调用通用 `brainstorming`
- 正式产物必须写入 `.claude/PROJECT_FINAL_SHAPE.md`
- 默认不得写 `docs/superpowers/specs/*`
- `shape` 完成后应继续 `worker_register -> dag_create -> task_create_batch`

## 额外说明

- `references/using-superpowers-adapter.md` 是 shape 阶段的参考材料
- `flows/*.md` 是本 bundle 的主实现定义，不是附属说明文档
- `SETUP.md` 是 setup flow 的安装说明来源，不是业务 flow 文档
- `hooks/*` 是 sticky mode 的运行时脚本；skill 本身不会每轮自动重注入，必须靠 `UserPromptSubmit` hook
