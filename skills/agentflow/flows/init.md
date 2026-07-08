# Init Flow

这是 `agentflow` bundle 内部的 **init flow** 文档。

用途：当用户执行 `/agentflow init` 时，把一个**已有内容的现有项目 / 现有 repo** 正式接入 agentflow。

这层不是：
- 安装 MCP（那是 `setup`）
- 恢复已接管项目（那是 `resume`）
- 推新目标（那是 `goal`）

这层只回答：

```text
当前这个已有内容项目
是否已经被 agentflow 接管？
如果没有，如何正式绑定？
绑定后应该去 resume，还是去 intake/goal？
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
- 绑定后的 repo / branch 事实

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

## Step 5. 读取 `project_next_steps`

如果 repo 已经有 HEAD：

```text
mcp__agentflow__project_next_steps(namespace_id)
```

用它判断当前项目现在处于：
- `setup`
- `shape`
- `plan`
- `execute`
- `stuck`
- `done`

## Step 6. 决定后续去向

推荐规则：

### 已经有 phase / DAG / task 上下文
动作：
- 切去 `resume`
- 让 `resume` 接手项目 snapshot、DAG 列表、推荐继续项

### 刚绑定成功，但还没有形成业务执行面
动作：
- 切去 `intake`
- 再由 `intake` 决定是否进入 `goal`

也就是：
- `init` 只负责把已有内容项目接入 agentflow
- 接进来后是 `resume` 还是 `intake/goal`，由当前项目状态决定

## 与 Setup / Resume / Goal 的关系

- `setup`：修 MCP 环境，不负责 repo/bootstrap
- `init`：把现有 repo 正式接进 agentflow
- `resume`：恢复已经接管的项目
- `goal`：推进新目标

一句话：

```text
setup = 装工具
init = 接现有项目
resume = 继续已接管项目
goal = 推新目标
```

## 结束条件

当且仅当以下条件满足时，本 flow 才算完成：
- 已确认 MCP 可用
- 已判断当前 cwd 是否已绑定 namespace
- 如未绑定，已成功调用 `project_init`
- 已根据 `has_head_commit` 和 `project_next_steps` 判断后续去向
- 已切去 `resume` 或 `intake/goal`，或明确停在 first-commit gate
