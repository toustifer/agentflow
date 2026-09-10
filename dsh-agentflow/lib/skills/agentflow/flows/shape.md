# Shape Flow

这是 `agentflow` bundle 内部的 **shape flow** 文档。

用途：维护稳定的产品形态书，并把每轮需求收敛成可继续拆 DAG / task 的范围。

## 边界

这个 flow 只在 `intake` 已接受需求后进入。

这个 flow 可以做完整设计讨论，不需要刻意轻量化；但它与通用正式 spec 流的边界必须非常清楚：

- 允许：探索上下文、逐步提问、比较方案、呈现设计、迭代修改
- 默认：已有产品形态书时只读它，并为当前 DAG 收敛本轮范围
- 只有 `create`、用户确认的 `amend` 或 `rewrite` 才能修改产品形态书
- 禁止：写 `docs/superpowers/specs/*`、commit spec、自动转 `writing-plans`
- 结束方式：让用户确认本轮范围，然后把控制权交回 goal flow

## Shape mode

先检查 `.claude/PROJECT_FINAL_SHAPE.md`：

- `create`：文件不存在，完成完整产品形态书。
- `reuse`：文件已存在且本轮不改变产品边界；产品书只读，当前范围写入 DAG title/description/tasks；复杂 DAG 可写 `.claude/dags/<dag_id>_SHAPE.md`。
- `amend`：只增加小范围能力、worker 或依赖；以 diff 方式修改产品书，必须让用户确认。
- `rewrite`：产品类型、主技术栈或核心用户流发生重大变化；必须先取得用户明确确认，才能重写。

默认规则：已有产品形态书即 `reuse`。单次 bugfix、polish、小功能不得静默进入 `rewrite`。

DAG shape 只写本轮的 Goal、In Scope、Out of Scope、相对产品书的增量和成功标准；不得复制整本产品形态书。


shape flow 的目标不是输出 formal spec，而是：
- 锁定后续拆 DAG / task 必须依赖的 Rigid 决策
- 明确 in-scope / out-of-scope
- 明确技术方向和外部依赖
- 明确需要哪些 worker
- 保证 Leader / Worker 后续不会乱猜产品边界

## Brainstorm -> 出形态 -> 拆 DAG

保持这个顺序：

```text
用户目标
-> 反问关键问题
-> 收敛 shape
-> 写 .claude/PROJECT_FINAL_SHAPE.md
-> 返回 goal flow
-> worker_register
-> dag_create
-> task_create_batch
```

## 只把 Rigid 写进形态书

需要明确写进 `.claude/PROJECT_FINAL_SHAPE.md` 的内容：
- 项目类型
- 目标用户
- 核心功能清单
- 明确不做的事
- 技术栈方向
- 外部依赖
- 数据模型方向
- 需要哪些 worker
- 成功标准

不需要写死的内容：
- 具体函数命名
- 内部目录细节
- 测试用例细节
- 组件内部实现
- worker 级别的灵活发挥空间

## 流程

### Step 1. Explore project context

先看当前目录状态：
- 有没有现有代码
- 有没有 README / docs / 调研文档
- 最近提交和当前 repo 状态
- 是否已经有 `.claude/PROJECT_FINAL_SHAPE.md`
- `intake` 刚刚确认的约束和范围边界是什么

### Step 2. 必要时反问关键问题

一次只问一个问题，优先锁定这些：
- 这是给谁用的
- 如果只能保留 1 个能力，是什么
- 明确不做什么
- 技术/区域/平台边界
- 真实外部依赖有哪些
- worker 应按什么角色拆

### Step 3. 提出 2-3 个合理切法

保留完整设计思考能力：
- 可以比较不同产品/技术切法
- 要说明 tradeoff
- 要给推荐

但目标始终是帮助 shape 收敛，而不是升级成 formal spec 文档流。

### Step 4. 分段呈现设计并确认

覆盖这些最关键内容：
- 核心用户流
- MVP 边界（in / out of scope）
- 系统结构/主决策单元
- 外部工具/API 边界
- 数据/状态模型方向
- 待确认风险项
- 实现顺序的大方向

### Step 5. 写入 `.claude/PROJECT_FINAL_SHAPE.md`

这是本 flow 的唯一正式落点。

禁止写入：
- `docs/superpowers/specs/*`
- `docs/specs/*`
- 任何 formal design doc 默认目录

### Step 6. 自检

写完后快速自检：
1. 有没有 TBD / TODO / 占位项
2. 有没有自相矛盾
3. 有没有超出当前项目边界的发散内容
4. 有没有把 Flexible 内容误写成 Rigid
5. 后续能不能直接据此拆 worker / DAG / task

### Step 7. 让用户 review 形态书

建议提示：

> 已将 shape 写入 `.claude/PROJECT_FINAL_SHAPE.md`。请先 review；如果边界没问题，就回到 goal flow 继续推进 worker / DAG / task。

## 产物模板

`.claude/PROJECT_FINAL_SHAPE.md` 至少应包含：

```md
# PROJECT_FINAL_SHAPE

## Goal
## Target Users
## In Scope
## Out of Scope
## Core User Flow
## Tech Direction
## External Dependencies
## Data / State Model Direction
## Workers Needed
## Success Criteria
## Rigid Decisions
## Flexible Decisions
## Open Risks
```

## 结束条件

当且仅当以下条件满足时，本 flow 结束：
- `.claude/PROJECT_FINAL_SHAPE.md` 已写好
- 用户已确认 shape 没问题，或仅剩小修改
- 内容已经足够 goal flow 继续 `worker_register -> dag_create -> task_create_batch`

结束后，不要继续扩展到 formal spec 流。把控制权交回 goal flow。
