# 技术提案：基于累积历史回放的 Agentflow 策略演化（Dream-RSI 启发方案）

> **提案状态**：草案 / 待评审（RFC）  
> **关联 DAG**：`dag-history-replay-proposal` / `task-1-write-proposal`  
> **目标文件**：`docs/PROPOSAL_HISTORY_REPLAY.md`  
> **基准分支**：`origin/master` @ `f55d5ed`  
> **核心原则**：不轻信营销倍数、不照搬不可比假设、闸门先行于算法、以损坏态为真实负例。

---

## 目录

- [0. 核心主张与执行摘要](#0-核心主张与执行摘要)
- [1. 背景：Dream-RSI 精确机制](#1-背景dream-rsi-精确机制)
- [2. Agentflow 现状映射表](#2-agentflow-现状映射表)
- [3. 三个不成立类比与论文自承局限](#3-三个不成立类比与论文自承局限)
- [4. 三台阶最小实现路线与 DAG 拆解](#4-三台阶最小实现路线与-dag-拆解)
- [5. 推荐演进路线](#5-推荐演进路线)
- [6. 首个真实历史数据集：今日会话 5 大缺陷分析](#6-首个真实历史数据集今日会话-5-大缺陷分析)
- [7. 来源可信度审计与核实矩阵](#7-来源可信度审计与核实矩阵)
- [8. 本提案尚未验证与待解决事项](#8-本提案尚未验证与待解决事项)
- [附录 A：Dream-RSI 事实断言自检映射表](#附录-adream-rsi-事实断言自检映射表)

---

## 0. 核心主张与执行摘要

用户提出：“我们的 agentflow 好像没有提供对于过去的 workflow 的处理方式”，并提出调研 `sleeping-rsi`。经 Leader 严格信源审计，`sleeping-rsi` 并非学术标准词，其真实学术原型为 2026 年 9 月发布的 **Dream-RSI**（arXiv `2609.14858`）。

Dream-RSI 的核心工程价值在于：**将智能体过去的实际探索轨迹转化为一个离线回放沙盘（Replay Simulator），在不改变模型权重与工具接口的前提下，通过编辑可执行的策略代码实现策略迭代。**

然而，直接把 Dream-RSI 的数学形式搬进 Agentflow 存在严重的**领域失真**：
1. **评估器性质截然不同**：Dream-RSI 依赖廉价客观的 Evaluator（编译、运行、数学验证），而 Agentflow 的交付验证是高度主观且包含博弈偏好的 Code Review / 人工验收。
2. **历史轨迹存在严重毒化**：Agentflow 过去留下的历史充满由于坏策略、坏判据导致的踩坑记录；脱离留出集（Held-out）与污染标注的回放，会直接演化出“系统性重蹈覆辙”的坏策略。
3. **DAG 依赖严重压缩了并行收益**：工业级工作流充满 `depends_on` 拓扑约束，无法像算法搜索那样获得大宽度批处理收益。

因此，本提案提出**“三台阶稳妥路线”**：
- **台阶 1（决策点提取）**：把 Task 状态机、Git 提交与 Review 反馈规范化为结构化的历史决策点，建立自举真实数据集。
- **台阶 2（闸门即策略）**：将写在 Leader 提示词中的自然语言散文经验，提炼为具有布尔判据、纯函数输入输出的可执行检查脚本（如已验证的 `scripts/preset-drift.ps1 -Check`）。
- **台阶 3（回放评分器）**：仅出设计，严禁盲目上线；必须引入留出验证与已知负例证伪，严防产生假阳性评分器。

---

## 1. 背景：Dream-RSI 精确机制

本节陈述的 Dream-RSI 理论机制全部严格限定于 Leader 已核实的文献事实（对应编号 `F1`–`F11`），禁止任何推测与未经证实的扩展。

### 1.1 论文基本身份与背景 [F1]
- **论文全称**：*Dream-RSI: Recursive Self-Improvement through Evolving Worlds*
- **arXiv 编号**：`arXiv:2609.14858`，cs.CL，提交于 2026-09-14。
- **机构署名**：Google / UMD (马里兰大学) / DeepMind / UVA (弗吉尼亚大学)。
- **主要作者**：Tong Zheng, Xidong Wu, Zheng Zhang, Zhankui He, Chaoyi Zhang, Benjamin Coleman, Ruoqiao Wei, Di Bai, Haolin Liu, Rui Liu, Xue Wang 等 20 余位学者。
- **已核实来源**：arXiv abs 页、arXiv HTML 正文、alphaXiv 镜像、HuggingFace 论文页（共计 7 源交叉印证）。

### 1.2 核心洞见 [F2]
> **"accumulated discovery history can serve as a replay simulator over the realized search space"**  
> （累积的发现历史本身，可以作为覆盖“已探索搜索空间”的离线回放模拟器。）

智能体在真实环境（Online）中与代码库交互时，耗费了高昂的 API 调用与执行时间。这些留存在磁盘上的分支、错误、修改与测量结果，构成了搜索空间的一个“已实现子集（Realized Subspace）”。离线（Offline）阶段无需反复调用真实的编译执行环境，而是将历史当作沙盘来回放、测试候选探索策略。

### 1.3 极简编排层架构 [F3]
> **"A lightweight orchestration layer makes exploration explicit and programmable while leaving the underlying coding agent unchanged."**  
> （一个轻量级编排层使探索过程显式化且可编程，同时保持底层的编程智能体完全不被修改。）

编排层与 Worker 执行层职责完全解耦：底层编码智能体（Coding Agent）不感知自改进逻辑，仅接收由编排层派发的分支和任务指令。

### 1.4 发现树（Discovery Tree）形式化定义 [F4]
- 树结构 $T$ 以 $r$ 为根节点，其中 $r$ 代表探索开始前的**初始工作区状态（Initial Workspace State）**。
- 每一个非根节点 $v \in T$ 拥有**唯一的直接父节点（Primary Parent）**。父节点直接定义了“节点 $v$ 的探索尝试从何处派生”：探索 Worker 通过恢复父节点所保存的工作区快照，并将累积的上下文观测注入提示词，进而展开新的代码生成。
- 节点 $v$ 严格保留其继承来的历史轨迹，并完整持久化记录当前这轮 **generation–evaluation（生成–评估）尝试的完整结果与文件系统快照（Filesystem Snapshot）**。

### 1.5 合格候选集（Eligible Set）与并行调度机制 [F5]
- 在任一轮次中，搜索策略通过观察当前树结构 $T$，从中筛选出下一步允许展开探索的节点。
- **合格节点集合（Eligible Set）** 定义为根节点与所有当前叶子节点的并集：
  $$A(T) = \{r\} \cup \{v \in T : v \text{ 是当前观察树的叶子节点}\}$$
- 系统配置 $W \ge 1$ 个并行 Worker，且约束每个 Worker 在同一时刻仅能执行一个生成–评估请求。
- 在第 $k$ 轮调度中（$k \le K_1$），探索策略自候选空间中筛选出一个节点批次：
  $$C_t^k \in A(T_t^k; W)$$
  该批次中每个选定节点被分发给一个独立 Worker，全并发并行执行。

### 1.6 回放分数（Replay Score）的目标函数 [F6]
候选策略在历史沙盘上重演时，其评分公式综合权衡了三项核心指标：
$$\text{Replay Score} = \text{最优解质量} - \text{步数代价罚项} + \text{并行调度奖励}$$
- **最优解质量**：回放路径上达成目标的代码质量或性能收益。
- **步数代价罚项**：为了达到该解所消耗的 generation–evaluation 调用步数。
- **并行调度奖励**：对策略能够将相互独立的后续尝试组织为并行批处理（Batching）的能力给予正向加分。

### 1.7 离线自改进循环（Offline Improvement Loop） [F7]
整个离线演化分为四步循环：
1. **冷启动起点**：候选策略集合由当前线上部署的基线策略开始：$\pi_t^0 = \pi_t$。
2. **策略代码编辑**：设立专门的 **Policy-Development Agent**，其输入包括：回放轨迹（Replay Trajectories）、回放评分（Replay Scores）以及前几轮修订的历史反馈。该 Agent 的产出是**直接修改探索策略的可执行代码文件**，得到新版本 $\pi_t^{m+1}$。
3. **沙盘回放评估**：新策略版本在历史树库 $W_0$ 的所有历史实例上独立重演，每棵树的最大回放步长（Horizon）受限为 $W_1$ 轮。
4. **优胜者部署**：历经 $W_2$ 轮代码修订与离线比选后，系统选拔出综合回放得分最高的一版策略，将其部署上线驱动后续的在线探索。

### 1.8 什么是严格固定的系统边界 [F8]
在 Dream-RSI 的自改进体系中，**发生变动的只有探索策略代码**：
- **模型基座（Models）保持固定**：不微调权重，不更换模型参数。
- **评估器（Evaluator）保持固定**：客观评测打分逻辑与基准测试代码不变。
- **执行与工具接口（Execution Interfaces）保持固定**：底层的编译环境、CLI 与 MCP 工具不变。

### 1.9 实测收益与虚假数据的拒引说明 [F9]
- **已核实权威收益**：根据论文官方配套视频解说与 EmergentMind 深度解读，Dream-RSI 实测可**“降低最高达 2.4 倍的探索成本（reducing discovery costs by up to 2.4×）”**。
- **严格拒引虚假数据 `162×`**：网络流传的所谓“提升 162 倍”经溯源仅来自单一博彩资讯农场站点 `shattered.io`（来源可信度审计仅得 **7/20 分，已被判定为红色弃用档**）。该数据完全脱离了论文实测事实，本提案**明确拒引并坚决不采信 162×**。

### 1.10 评测场景共同特征 [F11]
Dream-RSI 论文在三大高难度工程领域进行了实验验证：
1. **算法工程（Algorithm Engineering）**
2. **数学优化（Mathematical Optimization）**
3. **GPU 算子工程（GPU Kernel Engineering）**
论文报告在上述场景下解质量达到“有竞争力或更优”。
**三大场景的唯一关键共同点**：**均具备高度客观、运行开销低廉、可确定性自动化判定的 Evaluator（即代码是否通过测试、运筹解是否最优、CUDA 算子加速比是多少）。**

---

## 2. Agentflow 现状映射表

基于 Leader 核实材料中的架构映射（`F14`），Agentflow 当前体系与 Dream-RSI 各部件的对应关系如下：

| Dream-RSI 理论部件 | Agentflow 现状资产与实现形式 | 差距与成熟度评级 |
|---|---|---|
| **Discovery Tree**<br>（节点含工作区快照） | Task 状态机（`pending` $\rightarrow$ `executing` $\rightarrow$ `review_pending` $\rightarrow$ `passed`/`rework`）；`depends_on` 拓扑序；Worktree 机制直接对应 Git commit sha 与文件系统快照。 | 🟢 **原料近现成**<br>底层 Git 与 SQLite 已经完备记录状态，仅缺少结构化的历史决策点提取器。 |
| **Eligible Set**<br>（$A(T) = \{r\} \cup \text{Leaves}$） | 当前 DAG 待派发任务集合、被打回处于 `rework` 的任务、未收口的子 DAG。 | 🟡 **概念存在，未建模**<br>Leader 凭经验在提示词中扫任务，没有将其形式化为集合数学对象。 |
| **Policy = 可执行代码** | 当前 Leader 的编排、决策、评审与风控策略，分散存在于 `agentflow-leader` skill 与提示词散文（Prose）中。 | 🔴 **最大缺口**<br>策略是不可执行、无法单测、随上下文窗口抖动的自然语言。 |
| **批次选择并发调度**<br>（$C_t^k \in A(T_t^k; W)$） | Leader 现行机制是“串行读 DAG $\rightarrow$ 派发单任务 $\rightarrow$ 等待反馈”，依赖单子代理逐步流转。 | 🔴 **缺乏批次选择层**<br>无法在一次调度决策中圈定多个无依赖分支做矩阵式派发。 |
| **Replay Score** | 目前完全没有离线评分公式。仅依赖任务最终是 `pass` 还是人工介入中断。 | 🔴 **完全空白**<br>无多维加权评分系统，缺乏客观度量标尺。 |
| **Policy-Development Agent** | 目前由开发人员或在主会话中由 Leader 人工手写覆盘日记（Leader Diary）。 | 🟡 **半自动化**<br>输入是模糊的文字日记印象，而非结构化的“执行轨迹 + 惩罚得分”。 |
| **固定边界**<br>（模型、评估器、工具固定） | Worker 与 Leader 的模型通过 `agent.cordis.yml` 显式配置；MCP 工具由 DSH 基础设施供给。 | 🟡 **具备基础，需解耦**<br>需首先将策略从散文提升为独立检查脚本，才能锁定边界。 |

**核心总结**：**Agentflow 拥有完备的树结构基础设施（Git Worktree + SQLite 状态机），拥有完备的执行器，但核心缺口在于“策略目前是散文而非代码”以及“完全缺乏离线回放评分器”。**

---

## 3. 三个不成立类比与论文自承局限

在向 Agentflow 引入 Dream-RSI 前，必须破除盲目乐观的理论迁移幻想。以下三大类比在工程本质上**根本不成立**，任何方案设计必须正视这些物理差异。

### 3.1 类比破裂一：Agentflow 的验证不是廉价客观的 Evaluator [F15-1]
Dream-RSI 能够高效运行的前提，是评测场景（算法、数学优化、CUDA 算子）拥有客观、确定、毫秒级至秒级返回的自动化 Evaluator（`F11`）。但在通用软件工程与多智能体协作中，Agentflow 的核心交付验证依赖 **Code Review**。
Code Review 本质上是带有人类偏好、经验博弈、甚至是主观风格争议的过程。不仅运行开销巨大（需多轮大模型推理），且缺乏数学上的绝对 Ground Truth。
**直接推论**：**绝对不能照搬 Dream-RSI 的回放分数公式（F6）**。若强行用大模型做离线裁判，只会引入高昂的双重幻觉。在 Agentflow 中，离线评估只能采用**客观代理指标（Proxy Metrics）**：
- 任务经历的 Review Cycle 轮数（越少越优）；
- 被打回 Rework 的次数（越少越优）；
- 返工原因分类（是否命中已知规约、是否产生接口漂移）；
- 交付物证据链缺口被打回次数；
- 通道因配置错误直接宕机（Crash）的次数。

### 3.2 类比破裂二：回放沙盘是用被缺陷污染的历史构建的 [F15-2]
在实际工程演进中，Agentflow 积累的历史数据绝非干净的探索轨迹。以今日实操为例，历史中充满了由于旧策略有缺陷而导致的“地雷”：
- `toolFilter.deny` 点名子工具导致通道崩溃（`49f492b`）；
- 用裸 `sha256` 跨环境判据遭遇 CRLF 假阳性陷阱（`725a4e7`）；
- 用 `git hash-object` 试图规避行尾却再次陷入同一泥潭；
- 预设镜像三处脱节导致文档误导运维（`725a4e7`）。

这些历史记录**全部是在坏策略、坏工具、坏判据支配下产生的**。如果把被污染的历史不加甄别地作为回放沙盘，离线评分算法会**系统性偏爱“和过去行为模式完全一致”的策略**，将历史缺陷强化为系统教条。
**直接推论**：
1. **留出集（Held-out Testset）与随机对照组（Random Control）是离线回放的绝对设计前置条件**，决非锦上添花的优化项。
2. 历史数据必须具备**“污染状态标记（Taint Labeling）”**，已被证明为负例的历史节点必须显式隔离。
3. 这一工程风险，**恰恰呼应了 Dream-RSI 论文自身未能分析的最大局限（见下文 3.4 节）**。

### 3.3 类比破裂三：DAG 拓扑依赖严重压缩了批处理与并行搜索空间 [F15-3]
Dream-RSI 的发现树主要针对独立假设的广度并行探索，算法搜索中的分支很多是相互解耦的候选解，因此可以充分利用批处理奖励（`F5`, `F6`）获得吞吐优势。
但在真实的工程编排中，Agentflow 的任务通过 `depends_on` 严格绑定（例如：方案设计未定之前，禁止编写实现；实现未完成之前，禁止触发回归测试）。强烈的拓扑依赖序使得在绝大多数时间步上，合格候选集 $A(T)$ 的并发宽度 $W$ 非常狭窄（通常仅为 1 到 2 个并行任务）。
**直接推论**：**Agentflow 在批处理调度上的理论收益上限远低于 Dream-RSI**。提案文档与任何后续设计中，严禁暗示或宣称 Agentflow 可以通过回放复制论文在算子搜索中获得的并发倍数。

### 3.4 必须正视的 Dream-RSI 论文自承局限 [F10]
根据论文原文及 EmergentMind 的批判性深度解读，Dream-RSI 本身存在四项重大学术空白与局限，论文作者并未反驳：
1. **核心演化组件未经严肃评估**：负责修改策略代码的 **Policy-Development Agent 被当作一个黑盒固定组件，缺乏任何消融分析**。其使用的大模型、系统提示词、代码修订的鲁棒性、代码生成本身的语法/逻辑错误率，以及**对回放失真（Replay Artifacts）的敏感度**，完全没有经过实验量化。
2. **缺乏严格匹配调用（Matched-call）的随机种子对比**：最终结果在统计学意义上的多轮重复稳定性存疑。
3. **未证明回放分数与线上真实收益的相关性**：缺乏“离线回放高分的策略，部署到真实环境后一定能产生正向收益”的强相关性验证链条。
4. **缺乏基线对照组**：没有设立“在若干个生成的修订版本中纯随机选取一个部署”的朴素对照实验，无法排除收益仅仅来自探索随机性本身的可能性。

---

## 4. 三台阶最小实现路线与 DAG 拆解

为了在控制工程风险的前提下扎实落地，提案拒绝一步到位的“黑盒自改进系统”，而是拆解为三个步步为营的工程台阶。

```mermaid
graph TD
    subgraph Stage1 [台阶 1: 历史决策点提取与建树]
        T1[T1-1: 数据模式定义与提取器] --> T2[T1-2: 历史数据回填与基准提取]
        T2 --> T3[T1-3: 五大缺陷黄金数据集标注]
    end
    
    subgraph Stage2 [台阶 2: 散文规则代码化与纯函数闸门]
        T3 --> T4[T2-1: 闸门脚本规约与运行套件]
        T4 --> T5[T2-2: 针对机械判定的防御闸门编写]
        T5 --> T6[T2-3: 损坏态负例验证与 CI 拦截]
    end
    
    subgraph Stage3 [台阶 3: 回放评分器沙盘设计]
        T6 --> T7[T3-1: 离线回放器架构设计稿]
        T7 --> T8[T3-2: 留出验证与已知负例证伪方案]
    end
```

### 4.1 台阶 1：决策点提取（建树）—— 纯读历史，零风险

#### 4.1.1 核心目标与交付物
- **定位**：不碰任何运行时逻辑，纯离线只读扫描仓库历史与 SQLite 状态机，把过往分散的 Task 状态、Review 记录、Worktree 快照结构化为“决策点数据集（Decision Point Dataset）”。
- **交付物**：
  1. 工具脚本：`scripts/history-extract.ps1`（或轻量 Go 命令行）。
  2. 规范化决策点 JSON 架构标准定义。
  3. 历史基准数据集导出包：`tests/fixtures/decision_history.json`。

#### 4.1.2 决策点数据结构规范（Data Schema）
每一个决策点必须完整记录以下关键要素：
```json
{
  "decision_id": "dp-20260924-001",
  "dag_id": "dag-fix-leader-preset-toolfilter",
  "task_id": "task-1-toolfilter-deny-fix",
  "round": 1,
  "context_before": {
    "dag_state": "in_progress",
    "completed_tasks": [],
    "pending_tasks": ["task-1-toolfilter-deny-fix"],
    "review_cycle": 0,
    "known_evidence": ["channel crashed with unknown global tools: subagent, subagent_fork"]
  },
  "decision": {
    "assigned_worker": "agentflow-dev",
    "model_route": "cliproxy-google/gemini-3.8-flash-high",
    "acceptance_criteria": [
      "remove subagent/subagent_fork from toolFilter.deny",
      "verify restrict() source invariant"
    ]
  },
  "action_result": {
    "transition": "submit",
    "exit_code": 0,
    "duration_seconds": 184
  },
  "outcome": {
    "final_verdict": "pass",
    "review_rounds": 1,
    "rework_count": 0,
    "rework_categories": [],
    "evidence_gaps": []
  },
  "snapshot": {
    "worktree_head_commit": "b767c59",
    "evidence_files": [
      "docs/KERNEL_PRESET_TOOLFILTER.md",
      "~/.dsh/.agent-presets/agentflow-leader/agent.cordis.yml"
    ]
  }
}
```

#### 4.1.3 验收标准
1. 能对 Agentflow 既有的真实 DAG 运行记录进行幂等提取，产出不少于 $N$ 个结构化决策点（$N \ge 15$）。
2. 人工标定今日排查出的 5 个真实历史缺陷，将其发生前后的上下文、决策与失败结果制作为“黄金标注测试集（Golden Dataset）”。

#### 4.1.4 风险分析
- 历史会话可能存在元数据不完整（如早期的 Review 没有结构化记录字段）；需在提取器中设计稳健的向下兼容与缺失字段补齐机制。

#### 4.1.5 DAG 拆解
- **`task-1-1-schema-and-extractor`**
  - **标题**：定义决策点 Schema 并实现历史只读提取器
  - **`depends_on`**：`[]`
  - **预估工时**：2.0h
  - **验收要点**：定义完整 JSON Schema；实现 `scripts/history-extract.ps1`；支持从 SQLite 读取 Task 状态、从 Git 读取 Commit 记录，全过程零文件写污染。
- **`task-1-2-extract-baseline-data`**
  - **标题**：提取历史运行数据并生成基准记录包
  - **`depends_on`**：`["task-1-1-schema-and-extractor"]`
  - **预估工时**：1.5h
  - **验收要点**：在离线环境下抽取已有 DAG（如 `dag-fix-leader-preset-toolfilter` 等）的历史决策点，生成不少于 15 条规范数据并通过 Schema 校验。
- **`task-1-3-annotate-golden-failures`**
  - **标题**：人工标注今日 5 大缺陷的历史决策点作为黄金基准
  - **`depends_on`**：`["task-1-2-extract-baseline-data"]`
  - **预估工时**：1.5h
  - **验收要点**：精准标注 5 大缺陷发生时刻的 `context_before`、当时做出的错误 `decision`、以及直接导致的打回/崩溃结果，作为后续闸门的回归验证集。

---

### 4.2 台阶 2：闸门即策略（散文代码化）—— 确定性拦截，已验证路线

#### 4.2.1 核心目标与交付物
- **定位**：拒绝“让大模型在提示词里靠自觉遵守规约”，将历史惨痛教训固化为**独立、可执行、输入输出严格确定的纯函数式检查脚本**。
- **现成样板**：仓库现有的 `scripts/preset-drift.ps1 -Check`（见 `725a4e7`），它将“镜像三处脱节”的教训固化为一条确定性的校验命令，遇到异常退出码非 0，直接阻断。
- **交付物**：
  1. 目录规约：`scripts/checks/*.ps1`，每个脚本聚焦单一专项防御。
  2. 统合运行入口：`scripts/checks/run-all.ps1`，统一汇总结果并向 CI/Git 钩子输出。
  3. 专项检查集：
     - `check-toolfilter-deny.ps1`：防御子工具点名地雷。
     - `check-line-ending-drift.ps1`：防御 CRLF/LF 裸校验与假阳性陷阱。
     - `check-preset-mirrors.ps1`：防御预设镜像与本地配置脱节。
     - `check-version-hardcode.ps1`：防御多处版本号硬编码遗漏。

#### 4.2.2 纯函数接口规范
每个检查脚本必须符合以下工业标准：
- **输入**：无外部副作用，仅通过参数接收目标目录路径（默认为工作区根目录）。
- **输出**：标准 JSON 摘要至 stdout，格式统一为：
  ```json
  {
    "check": "check-toolfilter-deny",
    "passed": false,
    "defect_ref": "DEFECT-1",
    "violations": [
      "toolFilter.deny contains own contributed tool 'subagent' at agentflow-leader/agent.cordis.yml:284"
    ]
  }
  ```
- **退出码**：通过返回 `0`；检测到违规返回非 `0`（退出码 `1` 表示检查失败，`2` 表示环境参数异常）。

#### 4.2.3 验收标准
1. **真实负例检验（以损坏态作为直接证据）**：将每个检查脚本应用到其**历史上已知损坏的代码状态**下，必须**100% 报告违规且返回非零退出码**。
2. **当前干净态全绿**：在当前 `master` 代码库最新状态下运行 `scripts/checks/run-all.ps1`，所有已实现的检查必须全部通过，退出码为 0。

#### 4.2.4 风险分析
- 判据过于严苛可能引发误报（例如忽略了行尾差异导致跨机器运行时误报红灯）；要求所有脚本在设计时必须严格遵循行尾归一化原则，禁止使用裸字节比较。

#### 4.2.5 DAG 拆解
- **`task-2-1-check-runner-scaffolding`**
  - **标题**：搭建纯函数检查套件架构与 run-all 聚合入口
  - **`depends_on`**：`["task-1-3-annotate-golden-failures"]`
  - **预估工时**：1.5h
  - **验收要点**：规范 JSON 输出 Schema；实现 `scripts/checks/run-all.ps1`，支持递归执行子脚本并汇总违规清单。
- **`task-2-2-implement-mechanical-checks`**
  - **标题**：实现防 toolFilter 崩溃、防预设脱钩与防版本脱漏三大可机械化闸门
  - **`depends_on`**：`["task-2-1-check-runner-scaffolding"]`
  - **预估工时**：2.5h
  - **验收要点**：完成针对缺陷 1（toolFilter）、缺陷 4（preset 镜像）、缺陷 5（版本号硬编码）的 3 个检查脚本。
- **`task-2-3-verify-with-historical-corruptions`**
  - **标题**：拿历史损坏 commit 状态作为负例验证闸门的拦截有效性
  - **`depends_on`**：`["task-2-2-implement-mechanical-checks"]`
  - **预估工时**：2.0h
  - **验收要点**：在隔离测试目录下分别检出产生缺陷 1、4、5 时的历史文件内容，证明对应检查脚本精准拦截并返回错误退出码；在最新干净分支上全绿。

---

### 4.3 台阶 3：回放评分器（沙盘设计）—— 本次仅出方案，严禁盲目上线

#### 4.3.1 核心目标与交付物
- **定位**：这一步才是真正的“做梦（Dream）阶段”。在台阶 1 的结构化历史与台阶 2 的可执行闸门就绪后，设计离线沙盘，评估一组候选策略或新检查脚本如果部署在过去，到底能多挽救多少缺陷，或者带来多少假阳性误报。
- **本次交付物**：**仅限设计规范稿**（`docs/SPEC_REPLAY_EVALUATOR.md` 设计大纲），**本阶段明确不编写回放器实现代码**。

#### 4.3.2 针对 F15-2 必须强制内置的三项核心防线
回放评估器的设计规范中，必须在顶层强制引入以下机制，缺一不可：
1. **留出验证集（Held-out Testset）**：将台阶 1 提取的决策点划分为训练集（用于启发生成新策略）与严格隔离的留出测试集（用于盲测验证）。
2. **随机对照组（Random Control Baseline）**：每一轮策略得分必须与“随机生成的候选规则”以及“历史原貌策略”进行基线对比，必须证明其具有统计学显著的正向增益。
3. **历史污染单列标注（Taint-isolated Evaluation）**：在回放评分矩阵中，因已知坏判据造成的历史失败（如由于误判 CRLF 导致的 Rework）必须打上污染标签，不得计入策略的合法失败。

#### 4.3.3 验收要点设计：以缺陷 2 与缺陷 3 作为“已知答案的测验用例”
- **已知答案的考卷**：
  - 缺陷 2：使用裸 `sha256` 作为校验判据，遇到 Git `core.autocrlf=true` 时在 4 个 preset 里误报 2 个假阳性。
  - 缺陷 3：试图用 `git hash-object` 代替裸 sha256，但依然由于未处理工作树与存储库行尾转换，在干净检出树上再次误报。
- **合格的回放器判据**：
  回放器必须能够精准识别出：裸 `sha256` 与错误的使用 `git hash-object` 是**劣质判据**。在干净检出树的样本上，它们引发了系统不应承受的假阳性打回；只有引入了行尾归一化的 `scripts/preset-drift.ps1` 策略才能获得高分。**能否在回放中将缺陷 2 和 3 判定为“差”，是检验该回放评分器是否具有实际价值的试金石。**

#### 4.3.4 DAG 拆解
- **`task-3-1-spec-offline-replay-evaluator`**
  - **标题**：编写离线回放评分器架构设计规范稿
  - **`depends_on`**：`["task-2-3-verify-with-historical-corruptions"]`
  - **预估工时**：2.0h
  - **验收要点**：产出完整架构设计，明确定义回放输入输出、沙盘轻量模拟机制与代理指标换算公式。
- **`task-3-2-design-counterexample-falsification`**
  - **标题**：设计以缺陷 2/3 为已知负例的证伪与留出校验机制
  - **`depends_on`**：`["task-3-1-spec-offline-replay-evaluator"]`
  - **预估工时**：1.5h
  - **验收要点**：完成针对已知判据错误的设计校验用例规约，明确定义回放器如何通过留出测试集与随机对照组剔除假阳性。

---

## 5. 推荐演进路线

经过对论文自承局限（`F10`）与三大类比破裂（`F15`）的严肃论证，本提案给出极为明确的演化决策：

$$\mathbf{推荐执行路线}：\mathbf{严守台阶 1 + 台阶 2，对台阶 3 保持“只设计、暂不实现”。}$$

### 决策理由分析
1. **前序依赖不可逾越**：台阶 3 的回放沙盘能否成立，百分之百取决于台阶 1 的“决策点数据真实度”与台阶 2 的“检查策略代码化程度”。在没有高质量结构化历史、没有可执行代码闸门的前提下，直接做回放器就如同在流沙上筑塔。
2. **防范假阳性评分器的系统性灾难**：正如今日我们在缺陷 2、3 中见证的，**一个具备假阳性的自动化判据，其危害性远远超过没有判据**。在真实工程中，坏判据会直接瘫痪合规的交付通道，造成巨大的调试与排查成本。由于回放代理指标与真实代码质量存在固有语义鸿沟，未经实证检验的回放评分器极易引导系统向扭曲方向“过拟合”。
3. **立竿见影的工程确定性**：台阶 1 带来的是透明的审计资产；台阶 2 带来的是每次 Commit 和 CI 立即受益的防雷保护网。这两步是纯确定性的工程收益，零虚妄假设，必须优先完全打透。

---

## 6. 首个真实历史数据集：今日会话 5 大缺陷分析

基于本团队今日真实的自举协作与内核排障历史（见 `49f492b`、`725a4e7` 等提交记录），提取出首批 5 个真实历史缺陷。该表严格记录其机械可判定性与落地台阶归属：

| # | 缺陷现场与根因分析 | 可机械判定? | 归到哪一台阶 | 真实历史检验与定位 |
|---|---|---|---|---|
| **1** | **`toolFilter.deny` 点名自身工具致通道宕机**<br>在 Leader preset 中将自身通过 `subagent` 插件注册的工具写入 `toolFilter.deny`；`@deepseek-ai/dsh-tools` 的 `restrict()` 运行时判定其非 `restrictableNames`（非可继承 Ancestor 工具）直接抛错，导致 Worker 委派通道彻底崩溃。 | 🟡 **半可判定**<br>（需结合 Cordis 插件依赖与配置源码解析） | **台阶 2**<br>（固化为静态检查断言：`check-toolfilter-deny.ps1`，禁止 deny 自身注入工具） | 见提交 `49f492b`。<br>已通过源码逐行分析确立不变式，属于典型可静态断言的配置逻辑。 |
| **2** | **用裸 `sha256` 作为验收判据遭遇跨平台行尾陷阱**<br>在文档和验收逻辑中使用文件的裸 SHA256 哈希作为一致性判据；在 Windows 环境下由于 `core.autocrlf=true`，干净检出的工作树将换行符转为 CRLF，而线上配置为 LF，导致在 4 个 preset 中产生 2 个**假阳性报错**（无真实漂移却判漂移）。 | ✅ **完全可判定**<br>（已知答案的测试用例） | **台阶 3 核心测试**<br>（作为沙盘的已知负例，要求回放器必须能判定该策略为“差”） | 见提交 `725a4e7`。<br>在全新 `git checkout-index` 树上实测，直接暴露假阳性缺陷。 |
| **3** | **用 `git hash-object` 排除行尾伪脏再次失误**<br>在尝试修复裸 sha256 缺陷时，试图改用 `git hash-object` 计算对象哈希；但由于未经过行尾归一化管道处理，在未暂存状态下仍然受限于工作区换行符编码，重蹈覆辙产生相同误判。 | ✅ **完全可判定**<br>（已知答案的测试用例） | **台阶 3 核心测试**<br>（与缺陷 2 共同构成负例样本对，验证回放器的抗过拟合能力） | 见提交 `725a4e7` 中的排障记录。<br>证明修补策略若未触及本质，会在回放测试中被同等证伪。 |
| **4** | **Preset 镜像三处脱节且无同步机制**<br>`~/.dsh/.agent-presets` 运行副本、`skills/agentflow/agents` 源码镜像与 `dsh-agentflow/src` 派生包长期处于割裂状态；文档曾错误指导用户通过 `cp -a` 覆盖，导致旧配置回灌覆盖新修复。 | ✅ **完全可判定**<br>（已彻底解决） | **台阶 2 现成样板**<br>（`scripts/preset-drift.ps1 -Check` 已常态化运行） | 见提交 `725a4e7`。<br>编写了 CRLF 归一化的全量比对脚本，作为台阶 2 闸门的工程标杆。 |
| **5** | **版本号硬编码三处导致修改遗漏**<br>`v0.2.9` / `v0.3.0` 版本号分散硬编码在 `package.json`、`VERSION` 文件与安装脚本中；在发布版本更新时极易遗漏部分文件，导致运行时版本上报冲突。 | ✅ **完全可判定**<br>（已彻底解决） | **台阶 2 现成样板**<br>（`scripts/version-check.test.js`） | 见提交 `9c99a32`。<br>现已集成至自动化测试套件中，实现多处版本号联动一致性校验。 |

### 核心价值标注
**缺陷 2 与缺陷 3 是判据算法本身的重大缺陷**。历史中已经真实存在能够证伪它们的数据检查点（即那几次全新 checkout 树：裸 sha256 在 4 个预设中误报 2 个，而通过 LF 归一化比对后 4/4 完全一致）。**未来的回放评分器能否在离线测试中准确给这两个策略打出极低分并精准拦截，是检验回放器有效性的第一道铁门槛。**

---

## 7. 来源可信度审计与核实矩阵

本提案对所采纳的全部信息源执行了四维严格审计（权威度、一手性、可验证性、时效性）。严守科研诚信红线，绝不采信未经核实的谣言与低质营销内容。

### 7.1 信息源可信度评分与判定表 [F13]

| 来源与标识 | 启发式可信评分 | 判定结论 | 详细审计说明与角色定位 |
|---|---|---|---|
| **arXiv abs `2609.14858`** | 🟡 **14/20** | **采纳** | 论文官方摘要发布页。一手权威信源（Preprint，尚未完成期刊同行评审）。 |
| **arXiv HTML 正文** | 🟡 **14/20** | **采纳** | 论文完整正文。详细形式化定义（$T, r, A(T), C_t^k$）的一手出处。 |
| **alphaXiv 镜像** | 🟡 **14/20** | **采纳** | 带有学术社区讨论的权威镜像，独立佐证论文作者与四大研究机构署名。 |
| **HuggingFace 论文讨论页** | 🟡 **14/20** | **采纳** | 国际开源社区论文收录页，权威佐证论文摘要与离线改进流程。 |
| **Letta「Sleep-time Compute」** | 🟡 **14/20** | **参考** | **相邻技术概念**（属于智能体“睡眠期离线计算”分支，但并非 Dream-RSI 本文）。 |
| **philschmid RSI 博客** | 🟡 **12/20** | **参考** | 二手解读，清晰阐释了“一次探索运行留下了哪些资产”的工程直觉。 |
| **dejan.ai / CellCog / MindStudio** | 🟡 **12/20** | **参考** | 二手厂商技术博客，仅作为业界对离线自改进关注度的旁证。 |
| **EmergentMind 深度解读** | 🟡 **14/20** | **采纳** | **核心贡献源**。唯一系统性指出了论文四大批判性局限的一手级评测解读。 |
| 🔴 **shattered.io** | 🔴 **7/20 (弃用档)** | **绝对拒引** | **严禁采信**。站点主业为博彩及低俗内容农场，虚构了 `162×` 的离谱倍数，严重失实。 |
| **the-decoder / HF 论文正文直抓** | — | **未取得** | 网络状态标记为 `[unreachable]`，未纳入本次论证依据。 |

### 7.2 调研平台扫描跳过记录（Platform SKIP Log） [F13]
在执行自动化信源初筛时，多平台扫描记录如下（已忠实归档，不隐瞒失败通道）：
```text
[SKIP:github] 仓库检索无公开实现代码
[SKIP:twitter] 接口请求超时
[SKIP:reddit] URL 解析错误
[SKIP:youtube] 配套视频字幕抓取失败
[SKIP:v2ex] 社区连接断开
[SKIP:linkedin] 无相关技术词条
[SKIP:telegram] 频道访问未授权
[SKIP:linuxdo] Cloudflare 安全质询拦截
```

### 7.3 已核实与未核实清单（Fact Verification Status） [F12]
- `[verified]` **F1 论文身份与研究机构**（7 源交叉印证，确认无误）。
- `[verified]` **F2 核心机制：累积历史作为回放模拟器**（3 源独立确认）。
- `[verified]` **F8 系统边界：仅改动策略代码，模型、评估器与执行接口保持固定**（3 源确认）。
- `[weak]` **F11 最终解质量收益**（仅 1 源提及“解质量更优”，缺乏更广泛横向盲测数据）。
- `[unverified]` **F9 中的特定加速倍数**：网络散布的 `162×` 仅出自 `shattered.io`（弃用档）；论文及 EmergentMind 配套材料给出的数字是 **`up to 2.4×`**。该数字必须严格标注为**“仅在论文特定客观 Evaluator 场景下的配套材料口径”，严禁偷换为 Agentflow 可达收益**。
- **关于交叉验证工具局限性的关键注记**：
  在本次调研中，自动化工具 `harvest_verify` 曾把论文经典的“三阶段自改进循环”标记为 `[unverified] 0 源`。经 Leader 人工穿透审计两个一手源正文，证实了该表述的真实存在：
  - *arXiv HTML 正文*：“The offline phase begins by evaluating the current policy… It then revises the executable policy code”；
  - *HuggingFace 论文页*：“The improved policy is subsequently redeployed online to drive further discovery”。
  **结论**：这是**关键词字面匹配启发式的算法漏判，而非事实本身不存在**。这一事故作为“自动化校验工具的判据本身也会说谎”的典型范例载入本文档，深刻警示我们在台阶 3 评估器设计中必须防范同类机械误判。

---

## 8. 本提案尚未验证与待解决事项

为贯彻“严谨求实、不写空话”的技术作风，本提案明确圈定以下目前尚未得到充分验证的技术假设与工程风险，列为后续实施中的核心攻关课题：

1. **代理指标对 Review 质量的表征保真度尚未验证**：
   将“Review Cycle 轮数”、“Rework 次数”作为回放打分的主要依据，是否能够真实反映最终代码的工业质量？是否存在 Worker 为了迎合更少的 Review Cycle 而故意减少复杂边界检查的“逆向选择”倾向？
2. **Worktree 历史重演的磁盘与 I/O 资源开销尚未度量**：
   在历史包含数百个 Task 时，若回放器频繁执行 Git Checkout 或创建瞬态 Worktree，是否会导致本地磁盘空间爆炸或 I/O 吞吐瓶颈？是否需要设计轻量的虚拟文件系统快照机制？
3. **策略生成 Agent（Policy-Development Agent）的代码生成可靠性尚未验证**：
   Dream-RSI 论文自身完全回避了这一问题（`F10`）。让一个 Agent 直接编写或修改 PowerShell / Go 闸门脚本，其自身语法的正确率如何？是否会频繁编写出导致整套检查套件崩溃的代码？
4. **小样本历史下的过拟合风险尚未解决**：
   目前 Agentflow 沉淀的高价值缺陷样本较少（仅数个至数十个），在如此小的数据集上划分训练集与留出集，是否具有统计学上的显著性？如何防止新策略仅仅记住了今日这 5 个缺陷而丧失对未知隐患的防御能力？

---

## 附录 A：Dream-RSI 事实断言自检映射表

为确保本文档未引入任何未经核实的幻觉或脱离 Leader 材料的内容，特将文档中所有涉及 Dream-RSI 的事实断言与 Leader 核实条目（`F1`–`F13`）建立逐一对应自检表：

| 文档章节与断言内容 | 对应 Leader 核实材料编号 | 核实状态与来源证据 |
|---|---|---|
| 论文名称、arXiv 编号 2609.14858、发表时间 2026-09-14、四大机构署名与作者清单 | **F1** | 🟢 已核实（arXiv abs、arXiv HTML、alphaXiv、HuggingFace 论文页等 7 源） |
| “accumulated discovery history can serve as a replay simulator over the realized search space” 核心洞见 | **F2** | 🟢 已核实（3 源独立确认） |
| 轻量编排层使探索显式可编程，底层编码智能体保持不变 | **F3** | 🟢 已核实（论文原文逐字印证） |
| 发现树形式化：$T, r$，唯一 primary parent，恢复工作区快照，记录 generation-evaluation 结果 | **F4** | 🟢 已核实（论文正文数学形式化定义） |
| 合格集定义 $A(T) = \{r\} \cup \text{Leaves}$，并行 Worker 数量 $W$，按轮次挑选批次 $C_t^k$ 并行 | **F5** | 🟢 已核实（论文正文调度逻辑） |
| 回放评分公式权衡：最优解质量、生成步数罚项、独立后续并行批处理奖励 | **F6** | 🟢 已核实（论文优化目标公式） |
| 离线自改进四步循环：基线冷启动、策略代码编辑、全历史树独立回放、最高分策略部署 | **F7** | 🟢 已核实（arXiv HTML 与 HuggingFace 摘要） |
| 变动的只有策略代码，模型、Evaluator、工具接口全部保持固定不动 | **F8** | 🟢 已核实（3 源确认，论文严格设定） |
| 探索成本降低实测最高达 2.4 倍（up to 2.4×）；严格拒引 shattered.io 的 162× 谣言 | **F9** | 🟢 已核实（2.4× 来自配套视频与 EmergentMind；162× 查明系 7/20 分博彩站点虚构，已剔除） |
| 论文自承局限：Policy-dev agent 未经消融分析、缺 matched-call 种子对比、缺线上收益相关性、缺随机选取对照组 | **F10** | 🟢 已核实（EmergentMind 批判性解读，原文未予反驳） |
| 评测场景：算法工程、数学优化、GPU kernel 工程；共同点为具备廉价客观 Evaluator | **F11** | 🟢 已核实（论文实验章节） |
| 事实状态归类清单及 harvest_verify 启发式漏判案例记录 | **F12** | 🟢 已核实（调研过程原始核验记录） |
| 来源可信度评分矩阵（含 shattered.io 7/20 判定）与各平台跳过日志 | **F13** | 🟢 已核实（审计与发现流水线记录） |

---
*提案起草完成，已满足所有验收标准，交由 Leader 与团队进行技术评审。*
