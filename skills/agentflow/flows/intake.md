# Intake Flow

这是 `agentflow` bundle 内部的 **intake flow** 文档。

用途：在进入 `shape` 之前，先由 leader 判断一个新需求 **要不要现在进入系统**，而不是默认直接开始产品设计和拆 DAG。

## 这层回答什么

`intake` 只回答这些问题：
- 这个需求值不值得做
- 这个需求是不是现在做
- 它应该并入现有 DAG，还是新开 DAG
- 它会不会抢占关键 worker / 打断当前执行面
- 下一步应该进入 `shape`，还是先停住

一句话区分：
- `intake` 回答：**要不要做**
- `shape` 回答：**做成什么样**

## 适用范围

下面两类情况都先走 `intake`：
- 从 0 到 1 的新 demo / 新项目
- 已有项目上的新功能 / 新需求 / 范围调整

区别不是先靠“需求分类”硬分，而是靠 intake 结论收敛：
- `scope_target = new_dag`
- `scope_target = existing_dag`
- `scope_target = unknown`

## 默认输入

leader 做 intake 时，默认先看这些事实：

### 1. 当前项目状态

优先读取：
- `project_next_steps`
- `project_report`
- `project_blockers`

要回答：
- 当前项目在哪个阶段
- 有没有明显 blocker
- 现在适不适合插入新需求

### 2. 当前执行面

优先读取：
- `dag_list`
- 必要时 `dag_get(..., with=graph)`
- `task_query`
- 必要时 `task_history`

要回答：
- 更适合并入现有 DAG 还是新开 DAG
- 是否和已有 task 重叠、冲突、或其实是续做

### 3. 已有决策痕迹

优先读取：
- `doc_search`
- 必要时 `doc_list`
- `leader_diary_list`
- 必要时 `leader_diary_get`

要回答：
- 以前有没有做过类似判断
- 之前有没有明确说过“先不做”
- 最近 leader 有没有定过优先级和边界

### 4. worker 复用与负载

优先读取：
- `worker_list`
- 必要时 `worker_status`
- 可选：`worker_handbook_list`
- 可选：`find_knowledge`
- 可选：`find_pitfalls`

要回答：
- 需要的 worker 是否存在
- 是否已经过载
- 有没有现成经验可以复用

## 必要时补问

如果事实还不够，一次只补一个关键问题，优先问：
- 这个需求如果只能保留 1 个能力，是什么
- 这次明确不做什么
- 时间优先级是不是高于当前执行面
- 这是补现有项目，还是希望拉出一条独立线

不要一上来长篇产品设计；先把“做不做、现在做不做”判断清楚。

## 最小决策输出

intake 至少要收敛出这些结果：
- `decision`: `accepted | deferred | rejected`
- `rationale`: 为什么这样决定
- `scope_target`: `existing_dag | new_dag | unknown`
- `blocking_conditions`: defer/reject 时的阻塞原因
- `next_action`: `enter_shape | stop | wait | ask_user`
- `constraints_to_carry_forward`: 如果 accepted，进入 shape 时必须继续携带的约束

## 决策规则

### Accepted

满足以下情况时可接受：
- 需求目标清楚到足以进入 shape
- 当前执行面允许插入
- 相关 worker 能承接，或值得为它们安排新 DAG

动作：
- 写一条简短 `leader_diary_write`
- 进入 `shape`

### Deferred

适用于：
- 方向可能对，但现在插入成本过高
- 当前有 blocker / 资源冲突 / 优先级冲突
- 还缺一个关键前提事实

动作：
- 写一条简短 `leader_diary_write`
- 停止 goal flow
- 明确“什么条件满足后再重新考虑”

### Rejected

适用于：
- 明显不符合当前项目目标
- 与既定边界冲突
- 成本/收益明显不划算

动作：
- 写一条简短 `leader_diary_write`
- 停止 goal flow
- 明确为什么不做

## 写入边界

第一版 intake 默认：
- **不写正式 project doc**
- **不默认调用 `doc_write`**
- 只保留轻量决策痕迹

建议写法：

```text
leader_diary_write(
  type="decision",
  title="Intake accepted: ..." | "Intake deferred: ..." | "Intake rejected: ...",
  tags=["intake"],
  content=<需求 / 决策 / 原因 / 下一步>
)
```

内容只要能回答：
- 提了什么需求
- 决策是什么
- 为什么
- 下一步是什么

## 与 Shape 的边界

`intake` 不负责：
- 写 `.claude/PROJECT_FINAL_SHAPE.md`
- 做完整产品设计
- 产出 formal spec
- 直接拆 DAG / task

`intake` 通过后，才把控制权交给 `shape`。

## 结束条件

当且仅当以下条件满足时，本 flow 结束：
- 已明确给出 `accepted / deferred / rejected`
- 已给出简短理由和下一步
- 如需要，已写一条 intake 决策 diary

如果是 `accepted`：返回 goal flow，进入 `shape`。
如果是 `deferred / rejected`：停止推进，不要假装已经进入 shape。
