# Baseline Schema

`/agentflow init` 产出的 baseline 至少应包含这些信息。

## Required Sections

### 1. Project Summary
- 项目名称
- 一句话目标
- 当前已实现的核心能力

### 2. Tech Stack
- 主要语言
- 主要框架
- 构建/运行方式
- 测试/部署相关关键信息

### 3. Architecture Layers
至少把项目粗分成：
- 业务域层
- 基础设施层
- 共享组件/工具层

### 4. Module / Directory Map
- 关键目录
- 每个目录的大致职责
- 哪些目录是主业务入口

### 5. Domain / Worker Candidates
对每个候选域至少给：
- `id`
- `title`
- `scope`
- `kind`（业务域 or 基础设施）
- 代表性目录或文件

### 6. Design Decisions
- 当前可观察到的关键边界
- 已经存在的架构约束
- 需要后续 shape 明确的灰区

### 7. Repo Maturity Flags
- 是否已有首个 commit
- docs 覆盖度如何
- 是否已有可恢复的 agentflow phase / DAG / task 上下文

### 8. Handoff Decision
baseline 最终必须收敛到一个明确去向：
- `resume`
- `intake -> goal`
- `stop_on_first_commit_gate`

## Important Constraint

baseline 是盘面，不是执行计划。

它不应该：
- 直接产出 task DAG
- 直接开始 worker dispatch
- 越过 intake / shape 的边界
