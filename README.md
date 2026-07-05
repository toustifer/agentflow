# agentflow

<p align="center">
  <strong>AI Agent 团队的软件项目生命周期编排引擎</strong>
  <br>
  <em>A local lifecycle orchestrator for AI agent teams</em>
</p>

---

<p align="center">
  <a href="#中文概览">中文</a> ·
  <a href="#english-summary">English</a>
</p>

---

## 中文概览

agentflow 是一个**本地优先、MCP 原生、git/worktree-aware** 的 AI agent 项目生命周期编排引擎。

它把项目初始化、任务拆解、分支执行、review handoff 和项目记忆串成一条真实工作流：
- `project_init` 把本地代码仓库绑定到 namespace
- DAG 表达 branch-scoped 工作流，task 表达可依赖、可审查、可恢复的工作单元
- leader / worker / reviewer 三个角色按默认 behavior tree 推进主链
- docs / handbooks / diaries 持久化项目知识与交付记录

运行形态：
- **Go MCP server**：系统事实源、状态机、工具注册、SQLite 持久化
- **Python BT sidecar**：leader / worker / reviewer 默认行为树执行层
- **Git + worktree**：每个 DAG 绑定分支、每个 task 绑定 worktree

## 核心模型

| 概念 | 含义 |
|------|------|
| `namespace` | 一个项目的隔离边界 |
| `DAG` | 一条 branch-scoped 的工作流，通常对应一个功能分支 |
| `task` | 一个有状态、有依赖、有审查流转的工作单元 |
| `worker` | 执行任务的角色，跨 DAG 共享 |
| `reviewer` | 读取提交元数据并做 `pass / rework` 决策的角色 |
| `leader` | 负责 phase 判断、派发、监控、阻塞汇报、完成收口 |

除了任务状态，agentflow 还内建项目记忆面：
- `doc_*`：项目文档
- `worker_handbook_*` / `find_knowledge` / `find_pitfalls`：Worker 经验库
- `worker_diary_*`：Worker 工作日记
- `leader_diary_*`：Leader 项目日记

## 生命周期总览

项目不是直接从“建 task”开始，而是按 phase 推进：

```text
setup -> shape -> plan -> execute -> stuck -> done
```

| Phase | 含义 |
|------|------|
| `setup` | 还没有完成项目初始化 |
| `shape` | 正在确认最终形态、范围和角色分工 |
| `plan` | 已有 worker / namespace，但还没拆出 DAG / task |
| `execute` | 已有任务主链，正在 dispatch / 实现 / review |
| `stuck` | 当前没有可派发任务，也没有活跃任务，需要人工处理阻塞 |
| `done` | 当前 DAG / 项目任务已完成 |

高层入口：
- `project_next_steps`：看项目当前在哪个 phase、下一步该做什么
- `leader_tick`：让 leader 默认 BT 按 phase 做一次调度
- `lifecycle_tick`：在一条调用里串 leader -> worker -> reviewer 的完整主链

## 执行模型：Git / Branch / Worktree

这是当前系统最重要的运行约束之一。

### 1. `project_init` 是推荐入口

`project_init` 会：
- 创建或绑定 namespace
- 校验 / 初始化 git 仓库
- 设置主分支信息
- 记录 workdir / worktree root
- 写入 `.claude/agentflow-git.md`

`.claude/agentflow-git.md` 是 repo-local 的执行规则文件，约束 worker 如何在 worktree 中工作、如何提交、哪些动作被禁止。

### 2. 一 DAG 一分支

每个 DAG 绑定一个 feature branch。DAG 不是纯逻辑分组，而是和 git 分支直接关联的执行单元。

### 3. 一 task 一 worktree

task 在自己的 worktree 中执行，而不是直接在 repo root 改文件。

典型约束：
- worker 只修改自己的 `worktree_path`
- task 的 git branch 必须和 DAG branch 一致
- `start` / `resume` 会准备 task 的 git runtime
- `git_status` / `worktree_get` 用来检查当前 git/worktree 状态

### 4. `submit` 是带交付契约的

`submit` 不只是一次状态转换。对 git-backed task，提交前需要满足：
- clean worktree
- 已有 worker diary
- 能记录 `review.commit`
- 能记录 `review.diff`

reviewer 围绕这些 review metadata 做 pass / rework，而不是脱离 git 上下文推进状态。

## 默认 Behavior Tree 角色流

### Leader

`trees/leader-default.json` 的主线语义：

```text
refresh_phase
  -> setup_actions | shape_actions | plan_actions
  -> execute: dispatch_task | monitor_tasks
  -> stuck: report_stuck
  -> done: report_done
```

leader 负责判断项目处于哪个 phase，并按 phase 决定下一步动作。

### Worker

`trees/worker-default.json` 的默认链路：

```text
doc_search_prepare
-> task_get_confirm
-> enter_worktree
-> implement_code
-> git_commit_changes
-> doc_write_record
-> diary_write_entry
-> task_submit_for_review
```

这条链路明确表达：worker 的交付不是“改完代码就算结束”，而是要连同 commit、文档、日记和 review handoff 一起完成。

### Reviewer

`trees/reviewer-default.json` 的默认链路：

```text
fetch_work_diff
-> review_decide
-> task_review_pass | task_review_rework
```

reviewer 基于 `review.commit` / `review.diff` 决策，而不是脱离 git 上下文做抽象状态推进。

## MCP 能力面

README 不再硬编码工具数量；当前工具面请以 `pkg/server/mcp.go` 为准。

更适合按能力域理解：

### Bootstrap / Project Setup
- `project_init`
- `project_next_steps`
- `namespace_create`
- `namespace_get`
- `namespace_list`
- `namespace_delete`

### DAG / Task / Worker State
- `dag_create`, `dag_get`, `dag_list`, `dag_update`, `dag_report`, `dag_flowchart`
- `task_create`, `task_get`, `task_list`, `task_query`, `task_history`, `task_create_batch`, `task_transition`
- `worker_register`, `worker_get`, `worker_list`, `worker_update`, `worker_status`, `worker_prompt_get`

### Lifecycle / Behavior Trees
- `leader_tick`
- `lifecycle_tick`
- `bt_list_trees`
- `bt_show_tree`
- `bt_validate_tree`
- `bt_tick`

### Git / Worktree / Review Handoff
- `git_status`
- `worktree_get`
- task metadata 中的 `git.*`
- `review.commit` / `review.diff`

### Docs / Handbooks / Diaries
- `doc_write`, `doc_get`, `doc_list`, `doc_search`, `doc_delete`
- `worker_handbook_write`, `worker_handbook_get`, `worker_handbook_list`
- `find_knowledge`, `find_pitfalls`
- `worker_diary_write`, `worker_diary_get`, `worker_diary_list`
- `leader_diary_write`, `leader_diary_get`, `leader_diary_list`

### Reporting / Project Queries
- `project_next_tasks`
- `project_blockers`
- `project_report`
- `flow_ping`

## Quick Start

### 1. Build

```bash
go build -o agentflow ./cmd/agentflow/
```

### 2. Run as an MCP stdio server

```bash
./agentflow stdio
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

### 3. Bootstrap a real project

推荐的 happy path 不是先手动零散创建对象，而是：

1. `project_init`
2. `worker_register`
3. `dag_create`
4. `task_create` / `task_create_batch`
5. `project_next_steps`
6. `leader_tick`

### 4. Minimal lifecycle sketch

```text
project_init
-> worker_register
-> dag_create
-> task_create_batch
-> leader_tick
-> worker-default
-> reviewer-default
-> leader_tick(done)
```

## Runtime Notes

### Primary runtime mode

主路径是 MCP stdio：

```bash
./agentflow stdio
```

### Other modes

还支持：
- `agentflow file <path>`：读取单个 JSON-RPC 文件请求
- 默认 HTTP 启动：提供 `127.0.0.1:9600` 的健康接口和基础运行壳

但对 Claude Code / MCP 集成来说，**stdio 才是核心运行方式**。

### Python BT sidecar

BT-backed lifecycle 依赖 Python sidecar。Go server 会在需要时拉起 `python -m bt_service`。

没有 Python 时：
- 基础 MCP / 状态存储仍可工作
- 但 leader / worker / reviewer 默认 BT 主链能力会退化或不可用

### Git prerequisite

完整项目工作流强依赖 git：
- `project_init` 会处理 repo 绑定
- worktree / branch / review metadata 都依赖 git runtime
- 没有 git 时，无法使用完整的交付 / review 主链

### Database path

可通过环境变量覆盖数据库位置：

```bash
AGENTFLOW_DB_PATH=/path/to/agentflow.db
```

如果未设置，默认会在系统临时目录下使用 `agentflow.db`。

## Validation

### Unit / integration tests

```bash
go test ./pkg/...
python -m pytest bt_service/tests/ -x -vv
```

### End-to-end smoke

```bash
go run ./smoke/mcp_comm_check.go
```

这个 smoke 会真实验证：
- namespace / worker / DAG / task 创建
- task transition 权限
- worktree 准备与 git commit
- worker diary 前置
- submit / review / rework / cancel
- backward compatibility

## Repository Layout

```text
cmd/
  agentflow/              Go entrypoint (`stdio`, `file`, HTTP)

pkg/
  engine/                 状态机、DAG、Worker、SQLite、docs/diaries/handbooks
  server/                 MCP handler、lifecycle、git/worktree、BT provider bridge

bt_service/
  server/                 Python BT action / transport / clients
  tests/                  Python BT sidecar tests

smoke/
  mcp_comm_check.go       端到端协议 smoke

trees/
  leader-default.json
  worker-default.json
  reviewer-default.json
```

## Where To Go Deeper

如果你想看更深一层的定义，不要把 README 当成唯一真相来源：

- `SPEC.md`
  - worker handbook / diary / leader diary 的数据与存储模型
- `REPORT_STUCK_SPEC.md`
  - `report_stuck` 行为设计与 blackboard 契约
- `smoke/mcp_comm_check.go`
  - 端到端使用路径
- `pkg/server/mcp.go`
  - 当前 MCP 工具面的权威来源
- `pkg/server/project_init.go`
  - `.claude/agentflow-git.md` 规则模板来源
- `trees/*.json`
  - 默认 leader / worker / reviewer 行为树定义

## Caveats

- agentflow 是本地优先系统，不是 SaaS 控制平面。
- README 负责解释系统心智模型，不会复刻全部 spec 细节。
- BT 默认流是 opinionated 的；如果你要自定义策略，应直接看 tree 定义和 BT 工具。
- review handoff 的完整体验依赖 git metadata，而不是仅靠 task state。

---

## English Summary

agentflow is a **local-first MCP orchestration engine for AI agent teams**.

It now combines:
- project bootstrap with `project_init`
- phase-driven orchestration: `setup -> shape -> plan -> execute -> stuck -> done`
- Go MCP server + SQLite as the source of truth
- Python BT sidecar for the default leader / worker / reviewer flows
- git-native execution with one branch per DAG and one worktree per task
- explicit review handoff through `review.commit` and `review.diff`
- persistent project memory via docs, worker handbooks, and diaries

### Main runtime

```bash
go build -o agentflow ./cmd/agentflow/
./agentflow stdio
```

### Recommended bootstrap path

```text
project_init
-> worker_register
-> dag_create
-> task_create_batch
-> project_next_steps
-> leader_tick
```

### Default role flows

```text
Leader:   refresh phase -> dispatch / monitor / stuck / done
Worker:   confirm -> worktree -> implement -> commit -> doc -> diary -> submit
Reviewer: fetch diff -> decide -> pass / rework
```

### Deep references

- `SPEC.md` for handbook / diary data model
- `REPORT_STUCK_SPEC.md` for stuck-path contract
- `smoke/mcp_comm_check.go` for end-to-end smoke
- `pkg/server/mcp.go` for the authoritative MCP tool surface
- `trees/*.json` for the shipped behavior trees

---

## License

MIT
