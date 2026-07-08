# /agentflow

项目编排引擎调度器。`/agentflow` 是唯一公开入口。

`setup` / `init` / `intake` / `goal` / `resume` / `shape` 现在都作为本 bundle 内部 flow 持有：

```text
agentflow/
  SKILL.md
  flows/
    setup.md
    init.md
    intake.md
    goal.md
    resume.md
    shape.md
  references/
    using-superpowers-adapter.md
```

## 总原则

先确认 `agentflow` MCP 是否可用，再决定进入哪个业务 flow。

## MCP 前置检查

在进入 `goal` 或 `resume` 之前，先做两层判断：

1. 当前会话里是否已经有 `mcp__agentflow__*` 工具
2. 如果有，`mcp__agentflow__flow_ping` 是否成功

如果任一失败：
- 读取 `flows/setup.md`
- 按 setup flow 做安装/修复/验证
- 不要假装业务 flow 可以继续推进

## 调度逻辑

```text
如果 agentflow MCP 不可用      -> 读取 flows/setup.md
/agentflow init [项目名]        -> 读取 flows/init.md
/agentflow goal [目标]         -> 先读取 flows/intake.md，再读取 flows/goal.md
/agentflow resume              -> 读取 flows/resume.md 并按 resume flow 推进
/agentflow [无]                -> 默认读取 flows/resume.md
其他                             -> 全部当作 goal，先读取 flows/intake.md，再读取 flows/goal.md
```

## 路由规则

收到 `/agentflow` 后，按下面顺序判断：

1. 先确认 agentflow MCP 是否可用
   - 如果不可用：读取 `flows/setup.md`

2. 如果 args 以 `init` 开头
   - 读取 `flows/init.md`
   - 把它当作已有内容项目首次接入请求
   - 在 `/agentflow` 内完成 repo 绑定、扫描、baseline 建立与后续去向判断

3. 如果 args 以 `goal` 开头
   - 先读取 `flows/intake.md`
   - 再读取 `flows/goal.md`
   - 只有 intake 接受后，才按 goal flow 进入 shape / plan / execute

4. 如果 args 以 `resume` 开头
   - 读取 `flows/resume.md`
   - 先恢复项目 snapshot 和 DAG 列表
   - 再按 resume flow 决定继续哪条线

5. 如果 args 为空
   - 默认读取 `flows/resume.md`
   - 默认走同一套项目恢复流程

6. 其他
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
