---
name: agentflow-init
description: >
  Use when the user wants to adopt an existing repo or existing codebase into agentflow for the first time.
  Handles heavy initialization: bind repo/workdir, scan docs and source structure, generate baseline project metadata,
  propose domain/worker roster, then hand off to resume or intake/goal.
  Triggers on: /agentflow-init, 初始化现有项目, 接入已有 repo, baseline, 项目基线分析, 现有代码初始化.
---

# /agentflow-init

把一个**已有内容的现有项目 / 现有 repo** 正式接入 agentflow。

这个 skill 是重初始化器，不是日常路由器。

它负责：
- 现有 repo 首次接入
- README / docs / config / source tree 扫描
- baseline 生成
- 初始 domain / worker 候选盘面
- 决定后续进入 `/agentflow resume` 还是 `/agentflow goal`

它不负责：
- MCP 安装（交给 `/agentflow` -> `setup`）
- 日常 resume / goal 调度
- 直接开始 task 执行

## Recommended Pipeline

```text
Pre-flight
-> Bind
-> Scan
-> Baseline
-> Handoff
```

## Phase 1. Pre-flight

先确认：
- 当前会话里是否已有 `mcp__agentflow__*`
- `mcp__agentflow__flow_ping` 是否成功
- 当前 cwd 是否已绑定 namespace

如果 MCP 不可用：
- 返回 `/agentflow` 主 skill 的 setup 逻辑

如果当前 cwd 已经绑定 namespace：
- 不重复做重初始化
- 默认建议直接转入 `/agentflow resume`

## Phase 2. Bind

如果当前 cwd 尚未绑定 namespace：
- 调 `mcp__agentflow__project_init(...)`
- 记录：
  - `namespace_id`
  - `rules_file_path`
  - `has_head_commit`
  - repo/workdir 绑定事实

如果没有首个 commit：
- 停在 first-commit gate
- 不继续进入 baseline / execution handoff

## Phase 3. Scan

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

## Phase 4. Baseline

按 `references/baseline-schema.md` 生成 baseline。

推荐写入 agentflow 项目文档 / metadata：
- project summary
- tech stack
- architecture layers
- design decisions
- key directories / module map
- domain / worker candidates
- repo maturity flags

baseline 的职责是建盘面，不是直接生成执行任务。

## Phase 5. Handoff

根据 baseline 和当前项目状态决定：
- 已有 phase / DAG / task 上下文 -> `/agentflow resume`
- 只有 baseline，还没有 shape / DAG -> `/agentflow goal`，但先走 intake

## Notes

这个 skill 参考了 `agent-company init` 的分析纪律，但不复用：
- `.mycompany/`
- `leader.json`
- hub/dashboard 协议
- worker diary / session / playbook

这里只保留：
- 读 docs
- 扫代码
- 生成 baseline
- 建立 domain 盘面
- 再交回 agentflow 主链
