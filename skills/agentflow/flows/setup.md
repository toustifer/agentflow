# Setup Flow

这是 `agentflow` bundle 内部的 **setup flow** 文档。

用途：当 `/agentflow` 发现以下任一情况时，优先进入本 flow，而不是直接推进 goal / resume：
- 当前没有 `mcp__agentflow__*` 工具可用
- `mcp__agentflow__flow_ping` 调用失败
- agentflow MCP 桥接脚本或二进制未安装
- 本地 MCP 配置缺失或失效

## 目标

setup flow 的目标不是直接做产品，而是：
- 确认本机是否具备 agentflow MCP 运行条件
- 指导用户安装或修复 agentflow MCP
- 在确认 MCP 恢复可用后，把控制权交回 `/agentflow` 主流程

## 诊断顺序

### Step 1. 先判断是不是“完全没有 agentflow MCP”

症状：
- 看不到 `mcp__agentflow__*` 工具
- `/mcp` 中没有 `agentflow`

优先检查：
- `~/.claude.json` 里是否配置了 `agentflow` MCP server
- `agent-company/bin/agentflow-mcp.mjs` 是否存在
- agentflow 二进制是否存在于预期位置

### Step 2. 如果 MCP 存在，再测健康

调用：

```text
mcp__agentflow__flow_ping
```

结果分流：
- 成功：退出 setup flow，回到主流程
- 失败：继续检查桥接脚本 / 二进制 / 依赖

### Step 3. 检查本地关键文件

优先检查这些路径：
- `agent-company/bin/agentflow-mcp.mjs`
- `agent-company/bin/agentflow.exe`（Windows）
- `agent-company/bin/agentflow`（macOS/Linux）
- `~/.claude.json`

### Step 4. 检查运行前置条件

至少确认：
- Node.js 可用（桥接脚本依赖）
- MCP SDK 已装好
- 二进制存在且可运行
- `~/.claude.json` 中的 agentflow MCP 条目路径正确

### Step 5. 修复路径

本 flow 可以指导用户做这些动作：
- 从源码仓库编译二进制
- 或从 GitHub release / 已发布产物下载二进制
- 安装/修复 `agentflow-mcp.mjs`
- 更新 `~/.claude.json`
- 重启 Claude Code 或重新加载 MCP

## 安装信息来源

优先参考：
- `SETUP.md`
- `agent-company/bin/agentflow-mcp.mjs`

不要在 setup flow 里重新发明一套和 `SETUP.md` 冲突的安装说明。

## 自动化边界

setup flow 可以：
- 诊断
- 解释缺什么
- 给出明确命令或步骤
- 在用户确认后帮助执行可逆操作

setup flow 不应该：
- 静默下载并执行未知二进制
- 在未说明风险的情况下偷偷修改本地配置
- 假装 MCP 已经恢复而跳过验证

## 结束条件

当且仅当以下条件满足时，setup flow 才结束：
- `mcp__agentflow__flow_ping` 成功
- 或者用户明确决定暂时不继续安装/修复

成功后：
- 如果当前是新项目，回到 goal flow
- 如果当前是已有项目，回到 resume flow
