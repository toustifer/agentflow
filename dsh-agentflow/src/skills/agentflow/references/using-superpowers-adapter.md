---
name: agentflow-superpowers
description: Agentflow 适配版的 using-superpowers — 在 Leader 出形态书/DAG 前按需调用，提醒 brainstorming 顺序 + Rigid/Flexible 分类 + 红旗自检
---

# /using-superpowers（agentflow 适配版）

精简版 using-superpowers，只保留 agentflow 真正用得到的 3 件事。**仅在以下时机调用**：出产品书（Step 0）时、拆 DAG（Step 1-2）时。其他场景不用读。

---

## 1. 顺序：Brainstorm → 出形态 → 拆 DAG

```
User: "/agentflow 帮我做 X"
  ↓
[Step -1] Leader 反问 1 个 "为什么"（30 秒）
  ↓
[Step 0]  出形态书（.claude/PROJECT_FINAL_SHAPE.md）
  ↓
[Step 1-2] 拆 DAG
```

**为什么需要 Step -1**：用户说"做一个日记 CLI"可能是想 (a) 个人记录 (b) 团队分享 (c) 学习练手 — 三种解法完全不同。形态书漂亮但答错问题 = 浪费。

反问模板（挑 1 个，别全问）：
- "你做这个主要是给自己用还是给别人？"
- "如果只能保留 1 个功能，是哪个？"
- "6 个月后你希望它变成什么样？"

---

## 2. Rigid vs Flexible

| 类型 | 例子 | 规则 |
|---|---|---|
| **Rigid**（写进形态书就不能改） | 项目类型、技术栈、数据模型、核心功能清单、明确不做的事 | 改 = 重新和用户对齐 |
| **Flexible**（Worker 自由发挥） | 具体函数命名、测试用例、目录内部结构、辅助工具 | Worker 自己定 |

形态书**只写 Rigid 字段**。Flexible 留给 Worker prompt_template 优化。

---

## 3. 红旗自检（出形态书前 Leader 自问）

出现以下任何一条 = 停下重做：

- ⛔ "用户没明说 Python 我就直接用 Python" → **没对齐就选 = 跳过 brainstorm**
- ⛔ "这个功能感觉多余，先砍了" → **砍功能 = 修改形态 = 必须问用户**
- ⛔ "形态书太长，先简化" → **形态书是为了消除歧义，省略就失去意义**
- ⛔ "用户说先做着吧，我按我的理解" → **没确认 = 红旗**
- ⛔ "Worker 应该知道怎么做" → **形态书没写的边界，Worker 不会自己猜**

---

## 调用时机

- ✅ Step 0 出形态书时读一次
- ✅ Step 1-2 拆 DAG 时再读一次
- ❌ Worker dispatch、review、复盘阶段**不读**（这些是机械流程）

如果忘了读 → Leader 会编造形态/漏掉约束 → DAG 跑偏。
