# Init Analysis Checklist

`/agentflow-init` 在做现有代码库初始化时，至少按下面顺序收集事实。

## 1. Project Overview

优先读：
- `README.md`
- 项目根目录说明文件
- 入口配置文件

要回答：
- 项目是什么
- 面向谁
- 当前主要能力是什么

## 2. Docs and Specs

优先读：
- `docs/*`
- 模块说明
- 架构说明
- 接口说明

要回答：
- 现有边界是否已经被文档明确
- 有没有现成术语和模块分层

## 3. Config and Tooling

优先读：
- `package.json` / `go.mod` / `pyproject.toml` / `Cargo.toml`
- 构建、测试、部署相关配置
- app/router/framework 配置

要回答：
- 技术栈是什么
- 项目成熟度如何
- 是否已有清晰的运行/构建路径

## 4. Source Tree Structure

至少扫描：
- 顶层目录
- 业务模块目录
- 共享组件/基础设施目录
- 服务层/数据层/路由层

要回答：
- 哪些目录像业务域
- 哪些目录像基础设施层
- 哪些目录只是共享支撑

## 5. Representative Files

每个候选业务域至少抽读少量代表性文件，避免只按目录名猜。

要回答：
- 该域真正负责什么
- 与其他域的边界是什么
- 是否值得成为候选 worker/domain

## 6. Output Discipline

初始化阶段输出的是 baseline，不是任务拆解。

因此：
- 不直接生成执行 task
- 不假装已经完成产品 shape
- 先建立项目盘面，再决定交给 resume 还是 intake/goal
