# Init Flow

这是 `/agentflow` 主 skill 里的 **轻桥接 init flow**。

它的职责已经收窄：
- 接住用户仍然会输入的 `/agentflow init`
- 判断这是不是“已有内容项目第一次接入 agentflow”
- 然后把重初始化工作交给独立的 `/agentflow-init`

也就是说：
- `/agentflow` 继续做日常主路由
- `/agentflow-init` 负责重扫描 / baseline / domain 盘面生成

## 当前语义

当用户输入：

```text
/agentflow init
```

主 skill 应理解为：

```text
这是一个已有内容项目的首次接入请求
-> 不在这里做重分析
-> 转交独立 `/agentflow-init`
```

## 最小判断

在桥接前，只做最少判断：
- 当前会话里是否已有 `mcp__agentflow__*`
- `mcp__agentflow__flow_ping` 是否成功

如果失败：
- 转去 `flows/setup.md`

如果成功：
- 明确告诉用户：接下来进入独立 `/agentflow-init`
- 由 `/agentflow-init` 负责：
  - `project_init`
  - README / docs / config / source tree 扫描
  - baseline 生成
  - 决定去 `resume` 还是 `intake -> goal`

## 与独立 `/agentflow-init` 的边界

`/agentflow init` 不再负责：
- 直接做完整 repo 绑定细节
- 重扫描代码库
- 生成 baseline
- 建立 domain / worker 候选盘面

这些统一交给：

```text
/agentflow-init
```

## 结束条件

当且仅当以下条件满足时，本 flow 才算完成：
- 已确认 MCP 可用，或已转去 `setup`
- 已明确把控制权交给 `/agentflow-init`
