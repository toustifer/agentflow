# Agentflow 自举开发架构与实战指南 (Self-Hosting & Bootstrap Development Guide)

> **核心哲学**：将 Agentflow 作为宿主调度引擎，去开发、重构和演进 Agentflow 自身。  
> **核心架构**：**Host-Target 交叉隔离** —— 外部宿主控制面负责拆解与调度，内部隔离靶机（WSL2/Docker/独立目录）负责开发与测试，Git 作为唯一事实同步桥梁。

---

## 一、 为什么不能在宿主机上“裸跑”自举？

在操作系统与编译器设计中，用自身迭代自身（如 GCC 编译 GCC，Linux 内核开发）必须遵循自举隔离原则。如果直接让宿主 Agentflow 在其自身运行环境内修改并测试代码，会导致 5 个**结构性自指故障**：

```
                           【自指重入故障模型】
                           
   宿主机 Agentflow (正在运行) ───[锁定]───> agentflow.exe 二进制
          │                                        ▲
          │ (尝试覆写/测试)                          │ (Windows 文件锁拒绝:
          └──────────> go build -o agentflow.exe ──┘  ERROR_SHARING_VIOLATION)
          
   宿主机 Agentflow ──────[读写]──────> %TEMP%\agentflow.db (状态机撕裂)
                                                   ▲
   Worker 运行单元测试 ────[清库/独占写]───────────────┘
```

1. **二进制进程锁死（Process Lock / Suicide Trap）**：  
   Windows 下运行中的可执行文件受内核独占文件锁保护。正在调度任务的 `agentflow.exe` 无法被正在编译的 Go 二进制覆盖，测试清理进程也会引发共享冲突。
2. **状态数据库撕裂（Metastate Collision）**：  
   默认的数据库路径通常为系统临时目录的 `agentflow.db`。若 Worker 执行 `go test ./pkg/...` 或烟测脚本未重定向数据库，测试用例的重置表结构和并发写入会瞬间毁坏宿主当前任务的状态机。
3. **Git 索引锁冲突（Worktree Lock Contention）**：  
   同仓库的所有 `git worktree` 共享同一个底层 `.git/`（对象池、分支引用及 `.git/index.lock`）。Worker 在 worktree 内执行 commit/rebase 与宿主调度器的 Git 操作并发碰撞时，会引发 Git 锁拒绝。
4. **Python BT Sidecar 内存陈旧与端口冲突**：  
   宿主守护的 Python 行为树 sidecar（`python -m bt_service`）加载的是启动时刻的代码。修改 Python 核心逻辑后，宿主无法热重载；若测试拉起新 sidecar，还可能引发端口或 stdio 冲突。
5. **元认知认知重叠（Prompt Re-entrancy）**：  
   大模型既是“工具的调度者”，又是“工具代码的编写者”。缺乏物理界限时，模型极易将“当前对话的参数（如临时 namespace_id）”硬编码到被开发的源码中。

---

## 二、 架构设计：三层 Host-Target 隔离模型

```
┌────────────────────────────────────────────────────────────────────────┐
│                   Layer 1: 宿主机控制面 (Host Control Plane)            │
│  - DeepSeek Harness (DSH) Web UI / CLI                                 │
│  - 生产稳定版 Agentflow (如 v0.2.7) 负责目标规划与 DAG 状态追踪           │
│  - 宿主机专属工作区，保持绝对稳定，不运行正在开发的实验二进制                 │
└────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ (通过 SSH / 独立环境隔离隔离下发)
┌────────────────────────────────────────────────────────────────────────┐
│                  Layer 2: 隔离开发靶机 (Target Execution Sandbox)        │
│  推荐形态：WSL2 子系统 / Docker 容器 / 独立虚拟环境工作区               │
│  - 独立的 Python 3.8+ 纯净环境 (独立 PYTHONPATH)                       │
│  - 强制重定向开发环境变量:                                               │
│      export AGENTFLOW_DB_PATH=/tmp/sandbox-agentflow.db                 │
│      export AGENTFLOW_BT_DIR=/target/agentflow                          │
│  - 独立开发代码副本，运行编译、修改、单元测试、MCP stdio 烟测             │
│  - 产出物验证通过后执行 Git Commit & Push                               │
└────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼ (Git 事实流转)
┌────────────────────────────────────────────────────────────────────────┐
│                   Layer 3: 远程代码与 CI 门禁 (Truth & Merge Plane)      │
│  - GitHub 特性分支 (feat/xxx)                                          │
│  - GitHub Actions CI 跨平台自动化测试 (Ubuntu / macOS / Windows)       │
│  - 合并至 master 并触发版本翻代 (Flip Generation)                       │
└────────────────────────────────────────────────────────────────────────┘
```

---

## 三、 隔离靶机环境配置规范

### 1. 靶机环境变量隔离矩阵

在隔离靶机（WSL2 或独立沙箱）中启动 Worker 前，必须预设以下隔离环境变量，严禁穿透：

| 环境变量 | 默认值 | 靶机隔离值示例 | 目的 |
|---|---|---|---|
| `AGENTFLOW_DB_PATH` | `%TEMP%\agentflow.db` | `/tmp/target-dev.db` | 避免污染宿主状态机数据库 |
| `AGENTFLOW_BT_DIR` | 自动向上探测 | `/home/dev/agentflow` | 锁定靶机内部的行为树与 Python sidecar |
| `AGENTFLOW_PYTHON` | `python` | `/usr/bin/python3` | 避免误用宿主机器的 Python 路径 |
| `HUB_SYNC` | `1` | `0` | 单元测试与本地自举期间禁用云端心跳同步 |

### 2. 靶机本地快速验证流程

靶机内代码编写完毕后，必须依次完成以下“四重门禁”方可判定开发通过：

```bash
# 门禁 1: Python BT 引擎单元测试 (142 项测试必须全绿)
python3 -m pytest bt_service/tests/ -q

# 门禁 2: Go 核心包单元测试
go test -v ./pkg/engine/...
go test -v ./pkg/bt/...

# 门禁 3: MCP 协议通讯与状态流转烟测
go build -o bin/agentflow ./cmd/agentflow/
go build -o bin/mcp_comm_check ./smoke/mcp_comm_check.go
AGENTFLOW_BIN=./bin/agentflow ./bin/mcp_comm_check

# 门禁 4: 发布打包完整性校验
bash scripts/sync-skill.sh
bash scripts/pack-skill.sh dist/
```

---

## 四、 内核开发重要红线

在修改 Agentflow 内核代码时，靶机 Worker 必须严格遵循以下系统规范：

1. **MCP 协议帧必须严格遵守 Content-Length 头规范**：
   - 官方 MCP SDK（Claude / DSH）均采用 `Content-Length: <size>\r\n\r\n<JSON>` 封包。
   - 核心通信循环必须位于 `cmd/agentflow/main.go` 中的 `serveMCP`，不可回退为裸换行 JSON。
2. **Python BT 引擎保持零第三方 pip 依赖**：
   - `bt_service` 是随二进制打包分发的独立 sidecar，严禁引入 `requests`, `fastapi`, `pydantic` 等额外 pip 包。
   - 必须使用标准库 `urllib.request`, `json`, `threading`, `abc`, `importlib`。
3. **资源同步闭环**：
   - 修改根目录下 `bt_service/` 或 `trees/` 后，必须执行 `scripts/sync-skill.sh`（或 `.ps1`），确保 `skills/agentflow/` 中的同步副本保持最新。

---

## 五、 自举更新与翻代流程 (Generation Flip)

当靶机完成任务并将代码推送到远程分支、GitHub Actions CI 验证通过后，宿主机按照以下安全翻代步骤更新自身：

```
[Target] 提交 PR / Push 分支 ➔ [GitHub Actions] 跨平台 CI 绿灯
                                         │
                                         ▼
[Host] 宿主机安全更新流程:
  1. git checkout master && git pull origin master
  2. powershell scripts/build-release.ps1 (在隔离输出目录 dist/ 编译新二进制)
  3. 暂时退出宿主 DSH 会话 / 停止 agentflow 进程
  4. 覆盖安装 ~/.dsh/skills/agentflow/bin/agentflow.exe
  5. 重新拉起 DSH ── 翻代完成 (Stage N ➔ Stage N+1)
```
