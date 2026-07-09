# Init Flow

这是 `agentflow` bundle 内部的 **重初始化 init flow**。

用途：当用户执行 `/agentflow init` 时，把一个**已有内容的现有项目 / 现有 repo** 正式接入 agentflow，并完成首次 baseline 建立。

这层不是：
- 安装 MCP（那是 `setup`）
- 恢复已接管项目（那是 `resume`）
- 推新目标（那是 `goal`）

这层要完成的是：

```text
Bind
-> Scan
-> Baseline
-> Handoff
```

## Step 1. 先确认 MCP 健康

先判断：
- 当前会话里是否已有 `mcp__agentflow__*`
- `mcp__agentflow__flow_ping` 是否成功

如果失败：
- 转去 `flows/setup.md`
- 不要假装可以继续 `init`

## Step 2. 判断当前 cwd 是否已绑定 namespace

先做：

```text
mcp__agentflow__namespace_list(workdir_contains=<当前 cwd>)
```

分流：
- 找到已有 namespace：
  - 说明当前项目已经被接管
  - 不重复 init
  - 直接切去 `resume`
- 没找到：
  - 继续 `project_init`

## Step 3. 调用 `project_init`

对当前现有 repo/workdir 调：

```text
mcp__agentflow__project_init(...)
```

至少关注这些返回值：
- `namespace_id`
- `workdir`
- `rules_file_path`
- `has_head_commit`
- 绑定后的 repo / default base branch 事实

这一步的目标是：
- 让当前项目进入 agentflow namespace
- 写入 `.claude/agentflow-git.md`
- 建立 repo/workdir 到 namespace 的正式绑定

## Step 4. 处理“还没有首个 commit”

如果 `has_head_commit = false`：
- 明确告诉用户当前 repo 还缺 bootstrap baseline
- 可以完成绑定，但不能假装已经能正常 resume / dispatch
- 指向现有 first-commit 约束

也就是说：
- `init` 可以把 repo 接进来
- 但没有 HEAD 的 repo 还不能进入正常 worktree 执行面

## Step 5. 扫描现有代码库

按 `references/init-analysis-checklist.md` 扫：
- `README.md`
- `docs/*`
- 关键 config
- 源码结构
- 代表性模块文件

目标：
- 总结项目做什么
- 找出模块边界
- 区分业务域与基础设施层
- 提炼候选 worker/domain 盘面

注意：
- 这一步输出 baseline，不直接生成执行任务
- 不要跳过 intake / shape 的边界

## Step 6. 建立 baseline

按 `references/baseline-schema.md` 生成 baseline。

推荐写入 agentflow 项目文档 / metadata：
- project summary
- tech stack
- architecture layers
- design decisions
- key directories / module map
- domain / worker candidates
- repo maturity flags

baseline 的职责是建盘面，不是直接开始 dispatch。

## Step 7. 决定后续去向

推荐规则：

### 已经有 phase / DAG / task 上下文
动作：
- 切去 `resume`
- 让 `resume` 接手项目 snapshot、DAG 列表、推荐继续项

### 刚绑定成功，并完成 baseline，但还没有形成业务执行面
动作：
- 切去 `intake`
- 再由 `intake` 决定是否进入 `goal`

也就是：
- `init` 负责把已有内容项目接入 agentflow，并建立首次盘面
- 后续是 `resume` 还是 `intake/goal`，由当前项目状态决定

## 与 Setup / Resume / Goal 的关系

- `setup`：修 MCP 环境，不负责 repo/bootstrap
- `init`：把现有 repo 接进 agentflow，并做 baseline
- `resume`：恢复已经接管的项目
- `goal`：推进新目标

一句话：

```text
setup = 装工具
init = 接现有项目 + 出 baseline
resume = 继续已接管项目
goal = 推新目标
```

## 结束条件

当且仅当以下条件满足时，本 flow 才算完成：
- 已确认 MCP 可用
- 已判断当前 cwd 是否已绑定 namespace
- 如未绑定，已成功调用 `project_init`
- 如有 HEAD，已完成最小 baseline 扫描与建盘
- 已切去 `resume` 或 `intake/goal`，或明确停在 first-commit gate
