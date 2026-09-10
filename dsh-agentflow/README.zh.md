# @toustifer/dsh-agentflow

为 DeepSeek Harness 提供 agentflow。安装本插件后：挂载
[agentflow](https://github.com/toustifer/agentflow) MCP 服务器（暴露
`mcp__agentflow__*` 共 61 个工具：project_init、dag_create、task_create、
leader_tick、flow_ping……），并把 `agentflow` 技能同步进 DSH 技能根目录。

## 前置要求

- 已初始化 DSH（任意 profile）。
- agentflow 二进制需为**标准 MCP Content-Length 帧**版本（`feat/dsh-mcp-wiring`
  修复）。旧构建收到首条帧即崩溃：
  `invalid character 'C' looking for beginning of value`。验证：

  ```bash
  agentflow version    # 应打印版本，而不是启动横幅
  ```

## 安装

```bash
dsh plugin --profile web add @toustifer/dsh-agentflow
```

在 `<dshHome>/profiles/web/cordis.patch.yml` 加一行：

```yaml
- insert:
    - id: agentflow
      name: '@toustifer/dsh-agentflow'
```

重启 DSH（或新开会话）。技能目录出现 `agentflow`，工具列表出现 `mcp__agentflow__*`。

## 配置

| 字段 | 默认 | 说明 |
|---|---|---|
| `serverName` | `agentflow` | 工具名前缀 `mcp__agentflow__*` |
| `command` | `''` → `$AGENTFLOW_BIN` → `agentflow` | 二进制路径 |
| `args` | `['stdio']` | 传给二进制的参数 |
| `dbPath` | `''` | 非空时设置 `AGENTFLOW_DB_PATH` |
| `syncSkill` | `true` | 同步内置技能（带版本保护） |
| `toolCallTimeoutMs` | `60000` | 单次工具调用超时 |
| `failOnStartupError` | `false` | 初始连接失败时是否让激活失败 |
| `reconnect` | mcp-client 默认 | 重连策略 |

示例：

```yaml
- insert:
    - id: agentflow
      name: '@toustifer/dsh-agentflow'
      config:
        command: 'D:\myprogram\agentflow\bin\agentflow.exe'
        dbPath: 'C:\Users\me\.dsh\agentflow\agentflow.db'
```

## 原理

`apply()` 解析二进制路径 → 尽力同步内置技能到 `$DSH_AGENTS_HOME`
（默认 `~/.agents`）→ 以 `transport: 'stdio'`、`args: ['stdio']` 挂载
`@deepseek-ai/dsh-mcp-client`。

## 验证

- `skill` 可加载 `agentflow`。
- 工具列表出现 `mcp__agentflow__flow_ping`。
- 调用 `mcp__agentflow__flow_ping` 返回 `ok: true`。
