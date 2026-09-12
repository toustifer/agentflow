# AgentFlow · 内核自举开发预设 (agentflow-dev)

专门用于在隔离工作区（worktree、WSL2、Docker 或独立沙箱目录）内开发、重构和测试 Agentflow 自身。

## 适用场景
- 开发 Agentflow Go 核心引擎 (`pkg/`, `cmd/agentflow/`)
- 改进 Python BT 行为树 sidecar (`bt_service/`, `trees/`)
- 扩展 DSH 客户端与宿主插件 (`dsh-agentflow/`, 独立前端扩展)
- 编写和完善系统级架构文档与 Spec

## 核心隔离铁律（防自杀与状态撕裂）
1. **禁止覆写宿主进程**：严禁把新编译的二进制直接输出覆盖宿主使用的 `agentflow.exe`。
2. **强制隔离测试数据库**：执行任何单元测试或集成测试时，必须显式指定 `AGENTFLOW_DB_PATH`（如临时内存库或任务隔离目录的 `.db` 文件），严禁触碰 `%TEMP%\agentflow.db`。
3. **Python 标准库约束**：`bt_service` 必须保持零 pip 第三方依赖（仅限标准库）。
4. **资源同步必须闭环**：修改根目录 `bt_service` 或 `trees` 后，必须执行 `scripts/sync-skill.ps1`（或 `sync-skill.sh`）。
5. **严禁越界编排**：作为底层实现 Worker，不进行任务拆解与子代理衍生，专注通过测试代码与真实验证证据交付。
