# Codex 原生版

本适配复用 agentflow Go MCP 引擎和 Python 行为树服务，提供 Codex 技能及独立安装器。DSH、Claude 的安装配置和数据库不参与迁移。

## 安装与使用

准备 Python 3.11+、Git、Codex CLI，以及构建引擎所需的 Go（版本以 go.mod 为准）。在可信源码目录运行：

```text
python scripts/install-codex.py
```

已有可信构建可指定 `--binary <路径>`。Windows 使用原生 codex.exe，必要时指定 `--codex-command <codex.exe路径>`。不调用 PowerShell/cmd 包装脚本。安装器仅使用标准库，不下载额外依赖。

安装后重新打开 Codex 会话，输入：

```text
$agentflow setup
$agentflow 帮我检查当前项目，并按确认的目标组织开发和独立审查
```

技能入口是 `$agentflow`。没有安装永久模式钩子或注册自定义 `/agentflow` 命令。任务执行需要宿主提供真实子代理工具；不支持子代理的宿主只能检查、规划，不能伪造执行成功。

派工先准备 worktree 和票据，再创建真实 worker，写入实际身份和模型，启动成功后才允许编辑。交付还需测试、提交、日记和独立 reviewer。已有引擎生成的 `.claude/` 项目元数据路径保留兼容，不表示调用 Claude。

## 文件与保护

- 技能：`~/.agents/skills/agentflow`。
- 配置：`$CODEX_HOME/config.toml`，默认 `~/.codex/config.toml`，由 Codex CLI 注册 MCP。
- 数据库：`$CODEX_HOME/agentflow/data/agentflow.db`，重装保留。
- 引擎及 sidecar：`$CODEX_HOME/agentflow/runtime/<安装ID>`。
- 配置快照：`$CODEX_HOME/agentflow/backups/<安装ID>`。
- 技能备份：技能旁的隐藏目录 `.agentflow-backup-<安装ID>`，避免跨磁盘重命名失败。

默认拒绝覆盖来源不明的同名技能/MCP。`--replace-skill` / `--replace-mcp` 仅在人工核对后使用。失败会恢复技能；配置仅在其他设置未变时恢复，发现冲突或损坏则保留当前文件并提示检查。旧运行目录、备份和数据库不会自动删除。

回退时关闭受影响会话，只恢复备份中 agentflow 的 MCP 条目与技能，保留其他设置及数据库。安装器不会降级数据库格式。

## 验证

```text
python -m unittest discover -s tests/codex -v
go test ./cmd/agentflow ./pkg/server ./pkg/engine
python scripts/install-codex.py --binary <引擎路径> --codex-home <临时配置目录> --skill-dir <另一个临时技能目录>
python scripts/check-codex-mcp.py --codex-home <临时配置目录>
```

诊断脚本只测试 initialize、tools/list、flow_ping，并使用临时数据库。协议通过不等于模型能调用工具，更不等于完成了 worker/reviewer 全流程。

2026-10-23 Windows 验证记录：安装器 9 项测试通过（包含 C/D 盘之间的更新、回滚）；`go test ./cmd/agentflow ./pkg/server ./pkg/engine` 全部通过，其中服务层耗时约 131 秒。另一次人为设为 90 秒的限时运行超时，不影响前述默认超时运行的通过结果。真实 Codex CLI 在含空格的隔离路径中注册成功；MCP 暴露 66 个工具，协议级 flow_ping 通过；本机安装及覆盖更新完成。Codex CLI 规范化了另一 MCP 的空 `args=[]` 为省略，其余配置语义保留。

新 Codex CLI 会话成功发现 flow_ping，但调用被审批策略拒绝（工具需要批准、该会话 approval policy 为 never）。未绕过审批，因此尚未验收完整派工闭环。在允许正常工具批准的交互会话中完成 `$agentflow setup` 后再运行真实任务。

官方入口说明：[Codex skills](https://learn.chatgpt.com/docs/build-skills)、[Codex MCP](https://learn.chatgpt.com/docs/extend/mcp?surface=cli)。
