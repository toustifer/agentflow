# agentflow

<p align="center">
  <strong>轻量级项目编排引擎 · 为 AI Agent 团队设计</strong>
  <br>
  <em>Lightweight orchestration engine — built for AI Agent teams</em>
</p>

---

<p align="center">
  <a href="#项目背景">中文</a> ·
  <a href="#background">English</a>
</p>

---

## 项目背景

### 难题场景

当 AI Agent（如 Claude Code）作为 Leader 管理一个软件项目时，会遇到几个核心痛点：

1. **状态失控** — 多个子 Agent 并行执行，谁在做什么、做到哪一步了、谁在阻塞谁，完全靠人脑记忆
2. **角色混乱** — Leader、Worker、Reviewer 职责不清，Leader 不自觉写代码，Worker 不提交就卡住
3. **依赖未知** — Task A 依赖 Task B，但 B 还没完成时 A 就启动了，引发连锁失败
4. **跨 DAG 冲突** — 同一个 Worker 在多个功能分支上同时忙，但没有任何感知
5. **进度不可视** — 项目做到 50% 还是 80%，全靠估算

### 需求分析

我们需要的是一个 **轻量、内聚、MCP 原生** 的状态引擎，不是又一个 Jira 或 ClickUp。核心需求：

| 需求 | 说明 |
|------|------|
| 角色驱动状态机 | Leader/Worker/Reviewer 各有权限，非法操作服务端拒绝 |
| DAG 表达 | Task 之间的依赖关系自动推导，循环依赖提前检测 |
| Worker 注册表 | Worker 是全局实体，跨 DAG 共享，状态自动派生 |
| 批量操作 | Leader 拆解任务时一口气创建，减少 MCP 往返 |
| 可视化 | Mermaid 流程图，Claude Code 原生渲染 |
| 向后兼容 | 旧工具（7 个）全部可用，新字段可选 |

### 为什么不做成 SaaS

agentflow 就是一个 Go 二进制文件 + SQLite，没有服务器、没有数据库、没有订阅。**Leader Agent 通过 MCP stdio 协议直接连接**，数据存在本地文件，开箱即用。

---

## 产品功能

### 24 个 MCP 工具

```
项目/命名空间            task_create, task_get, task_list, task_transition,
├── DAG 管理             task_history, task_query, task_create_batch
│   ├── dag_create
│   ├── dag_get / with=graph
│   ├── dag_list
│   ├── dag_update
│   ├── dag_report           ← 进度报告（完成率 + Worker 负载）
│   └── dag_flowchart        ← Mermaid 流程图
│
├── Worker 注册表       worker_register, worker_get, worker_list,
│                       worker_update, worker_status（状态自动派生）
│
├── 项目级查询          project_next_tasks（依赖感知 + Worker 感知）
│                       project_blockers（显式阻塞 + 隐式阻塞）
│                       project_report（跨 DAG 聚合报告）
│
└── 健康检查            flow_ping, namespace_create, namespace_list
```

### 状态机

```
assigned ──start──→ executing ──submit──→ review_pending ──pass──→ done
                     ↑                      ↑
                resume──┐              reassign
                     │              │
                rework_needed ←─rework─┘
                     │
                reassign → assigned
                cancel → cancelled
```

每个角色只能执行自己的转换：

| 角色 | 允许的操作 |
|------|-----------|
| **Leader** | `start`, `resume`, `reassign`, `cancel` |
| **Worker** | `submit` |
| **Reviewer** | `pass`, `rework` |

非法操作（如 Worker 试图 `pass` 自己的任务）会被服务端拒绝。

### DAG + Worker 模型

```
Namespace（项目）
  ├── DAG-1（"添加登录功能", branch: "feat/login"）
  │     T1(worker-auth) ──→ T2(worker-fe) ──→ T3(worker-qa)
  │     T4(worker-auth, 并行)
  │
  ├── DAG-2（"暗黑模式", branch: "feat/dark-mode"）
  │     T5(worker-fe) ──→ T6(worker-qa)
  │
  └── Worker 注册表
        ├── worker-auth — idle（DAG-1 已完成）
        ├── worker-fe   — busy（DAG-2 执行中）
        └── worker-qa   — idle
```

关键原则：
- **DAG 节点 = Task**，依赖通过 `depends_on` 表达
- **Worker 全局共享**，不因 DAG 复制
- **Worker 状态自动派生**：有任一 Task 处于 executing/review_pending → busy
- **跨 DAG 隐式阻塞**：`project_next_tasks` 自动标记 Worker 忙导致的阻塞

### 技术调研原则

Worker 遇到技术障碍时，**谁的问题谁解决**——Worker Agent 自带 WebSearch/WebFetch：
1. Worker 自研搜索，找到方案后继续实现
2. 确实无法解决 → Leader `reassign` 换人
3. 不引入独立的调研 Worker，避免责任转移

---

## Quick Start

```bash
go build -o agentflow ./cmd/agentflow/
./agentflow stdio   # MCP 模式
```

接入 Claude Code 的 `.claude.json`：

```json
{
  "mcpServers": {
    "agentflow": {
      "command": "./agentflow",
      "args": ["stdio"],
      "type": "stdio"
    }
  }
}
```

### 测试

```bash
go test ./pkg/...
go run ./smoke/mcp_comm_check.go
```

---

## 工程结构

```
pkg/
  engine/           ← 核心引擎（状态机、DAG、Worker、SQLite）
    engine.go        — 状态机 + CRUD
    dag.go           — DAG 类型 + CRUD + 报告
    worker.go        — Worker 类型 + CRUD + 状态
    project.go       — 项目级查询（next_tasks, blockers）
    batch.go         — 批量创建 Task
    flowchart.go     — Mermaid 流程图
    store.go         — SQLite 持久化
    store_dag_worker.go — DAG/Worker 持久化
  server/           ← MCP 协议层
    mcp.go           — 工具注册 + 请求分发
    handlers_ext.go  — 所有 MCP handler
    sync.go          — Hub 同步接口
```

---

## 效果

在一个模拟项目（日记 CLI）的测试中：

| 指标 | 结果 |
|------|------|
| 创建 DAG | 1 次调用 |
| 注册 Worker | 3 次调用 |
| 创建 Task（含依赖） | 1 次 batch 调用 / 3 个 task |
| 执行 + review + pass 完整周期 | ~30 秒 |
| 跨 DAG Worker 冲突感知 | 自动 |
| Mermaid 可视化 | 一次 dag_flowchart 调用 |

---

## Background

### The Problem

When an AI Agent leads a software project, chaos emerges fast:

1. **State drift** — multiple sub-agents executing, no one knows who is doing what
2. **Role confusion** — Leader writes code, Worker doesn't submit, Reviewer skips
3. **Hidden dependencies** — Task B starts before Task A finishes → cascade failure
4. **Cross-DAG conflicts** — same Worker busy on two branches, no coordination
5. **Invisible progress** — is it 50% or 80%? Nobody knows.

### Solution

agentflow is a **lightweight, MCP-native, embeddable orchestration engine**. It is not a SaaS — just a Go binary + SQLite.

### 24 MCP Tools

```
Namespace/DAG/Task management    task_create, task_get, task_list,
                                 task_transition, task_history,
                                 task_query, task_create_batch
DAG lifecycle                    dag_create, dag_get, dag_list,
                                 dag_update, dag_report, dag_flowchart
Worker registry                  worker_register, worker_get,
                                 worker_list, worker_update, worker_status
Project queries                  project_next_tasks, project_blockers,
                                 project_report
Health                           flow_ping, namespace_create,
                                 namespace_list
```

### State Machine

```
assigned ──start──→ executing ──submit──→ review_pending ──pass──→ done
                     ↑                      ↑
                resume──┐              reassign
                     │              │
                rework_needed ←─rework─┘
                     │
                reassign → assigned
                cancel → cancelled
```

| Role | Allowed Transitions |
|------|-------------------|
| **Leader** | `start`, `resume`, `reassign`, `cancel` |
| **Worker** | `submit` |
| **Reviewer** | `pass`, `rework` |

### Architecture

```
Claude Code ──MCP stdio──→ agentflow (Go binary)
                                │
                           SQLite (file)
```

### Getting Started

```bash
go build -o agentflow ./cmd/agentflow/
./agentflow stdio
```

---

## License

MIT
