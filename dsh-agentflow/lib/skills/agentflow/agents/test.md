# Agentflow Test Agent

当你收到 `/skill:agentflow test` 时，执行以下测试流程：

## 测试步骤

### 1. 检查 MCP 工具可用
```
mcp__agentflow__flow_ping
```

预期返回类似：
```json
{"status":"ok"}
```

### 2. 创建命名空间
```
mcp__agentflow__namespace_create name: "test-{timestamp}"
```

### 3. 创建任务
```
mcp__agentflow__task_create
  namespace: "test-{timestamp}"
  title: "测试任务"
  description: "这是一个测试"
  assignee: "leader"
```

预期返回包含 `available_transitions`。

### 4. 列出任务
```
mcp__agentflow__task_list namespace: "test-{timestamp}"
```

### 5. 获取任务详情
```
mcp__agentflow__task_get
  namespace: "test-{timestamp}"
  task_id: "<上一步返回的 id>"
```

### 6. 状态转换测试
```
mcp__agentflow__task_transition
  namespace: "test-{timestamp}"
  task_id: "<task_id>"
  transition: "start"
  actor_role: "leader"
```

### 7. 查看历史
```
mcp__agentflow__task_history
  namespace: "test-{timestamp}"
  task_id: "<task_id>"
```

## 验证标准

- 所有 7 个 MCP 工具都能调用成功
- 任务创建后返回 `id` 和 `available_transitions`
- 状态转换能正常流转（assigned → executing → ...）
- 历史记录正确显示事件

完成后汇报测试结果。
