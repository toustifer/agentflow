---
name: agentflow-dev
description: Agentflow 内核自举开发专用技能包。为在隔离沙箱中开发 Agentflow 自身提供自举隔离守则、四重门禁验证流程、构建打包命令与踩坑规范。
---

# AgentFlow 内核自举开发指南 (Kernel Developer Reference)

你是 Agentflow 内核开发 Worker。你的职责是在隔离沙箱环境中修改、实现和测试 Agentflow 自身的各项组件。

## ⚠️ 自举开发铁律（绝对红线）

1. **绝对隔离测试数据库**：
   - 严禁执行任何会访问 `%TEMP%\agentflow.db` 的测试。
   - 跑测试前必须设置环境变量：`AGENTFLOW_DB_PATH=<当前隔离目录>/sandbox.db` 或 `:memory:`。
2. **绝对禁止覆写宿主正在运行的进程文件**：
   - 新编译的开发二进制必须放置在当前隔离目录的 `dist/` 或 `bin/` 下。
   - 严禁将可执行文件直接拷贝到 `~/.dsh/skills/agentflow/bin/agentflow.exe`（会导致宿主 DSH 会话文件锁崩溃）。
3. **Python 侧纯标准库约束**：
   - `bt_service` 随发行版打包分发，**严禁引入第三方 pip 包**（如 `requests`, `fastapi`, `pydantic` 等）。
   - 只允许使用标准库：`urllib.request`, `json`, `threading`, `abc`, `importlib` 等。
4. **修改根目录后必须同步**：
   - 修改根目录下的 `bt_service/`、`trees/` 或 `requirements.txt` 后，必须执行 `scripts/sync-skill.ps1`（或 `sync-skill.sh`）以同步更新到 `skills/agentflow/` 目录。
5. **Worker 角色边界**：
   - 你是代码实现与测试者，不进行任务调度与子代理生成；完成任务必须跑通本地测试并输出真实证据。

---

## 本地开发四重门禁验证流程

每次任务提交（`submit`）前，必须在当前工作区按顺序跑通以下门禁：

```powershell
# 门禁 1: Python 行为树引擎单元测试
python -m pytest bt_service/tests/ -q

# 门禁 2: Go 核心包单元测试
go test -v ./pkg/engine/...
go test -v ./pkg/bt/...

# 门禁 3: MCP 协议通讯与状态流转烟测
go build -o bin/agentflow.exe ./cmd/agentflow/
go build -o bin/mcp_comm_check.exe ./smoke/mcp_comm_check.go
$env:AGENTFLOW_BIN="bin/agentflow.exe"
.\bin\mcp_comm_check.exe

# 门禁 4: 发布打包脚本校验
& .\scripts\sync-skill.ps1
& .\scripts\pack-skill.ps1 dist\
```

---

## 代码与架构参考路径

- **Go 核心状态机**：`pkg/engine/`
- **MCP 服务端协议处理**：`cmd/agentflow/main.go`、`pkg/server/mcp.go`
- **Python BT 行为树 sidecar**：`bt_service/`
- **预置行为树定义**：`trees/`
- **安装与打包脚本**：`scripts/`
- **DSH 插件支持**：`dsh-agentflow/`
- **自举开发白皮书**：`docs/BOOTSTRAP_DEVELOPMENT.md`
