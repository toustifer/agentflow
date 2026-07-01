# agentflow SPEC — Worker Handbook & Diary（MCP 结构化存储）

## 一、隔离模型

```
namespace（= 项目，完全隔离）
  ├── Workers（注册表，DAG 间共享）
  │     ├── handbook ← Worker 知识库（scope + tech_stack + knowledge + pitfalls）
  │     └── diary    ← Worker 工作日志（date + task_id + markdown + tags）
  │
  ├── DAGs（功能分支）
  │     └── Tasks（节点，depends_on）
  │
  └── Leader
        └── diary    ← Leader 项目日志（entries with type/dag_id/task_id）
```

- namespace = project，不存在上层
- 数据按 namespace 完全隔离，不考虑跨项目共享
- Worker 在 namespace 内跨 DAG 共享

---

## 二、数据模型

### WorkerHandbook

```go
type KnowledgeItem struct {
    Topic     string   // 主题（如 "JWT Token刷新"）
    Content   string   // 说明
    Tags      []string // 搜索标签
    Source    string   // 来源 task（如 "T2: 登录 API 实现"）
}

type PitfallItem struct {
    Scenario  string   // 场景描述
    Problem   string   // 遇到的问题
    Solution  string   // 解决方案
    Tags      []string
    Source    string   // 来源 task
}

type WorkerHandbook struct {
    WorkerID    string
    NamespaceID string
    Scope       string           // 领域描述
    TechStack   []string         // 技术栈
    Knowledge   []KnowledgeItem  // 知识条目（核心，每次交付后追加）
    Pitfalls    []PitfallItem    // 踩过的坑
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

### WorkerDiary

```go
type WorkerDiary struct {
    WorkerID    string
    NamespaceID string
    Date        string    // YYYY-MM-DD，一天一篇
    TaskID      string    // 关联的任务 ID（可选）
    Content     string    // Markdown 正文
    Tags        []string  // 搜索标签
    CreatedAt   time.Time
}
```

一天一篇，同一天多个任务可以追加到同一篇。

### LeaderDiary

```go
type DiaryEntry struct {
    Type      string // "dag_complete" | "rework" | "decision" | "note"
    DAGID     string // 关联 DAG（可选）
    TaskID    string // 关联 Task（可选）
    Title     string
    Content   string // Markdown
    Tags      []string
    Timestamp time.Time
}

type LeaderDiary struct {
    NamespaceID string
    Date        string       // YYYY-MM-DD，一天一篇
    Entries     []DiaryEntry // 一天内可以有多个
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

---

## 三、SQLite 存储

```sql
CREATE TABLE worker_handbooks (
    namespace_id  TEXT NOT NULL,
    worker_id     TEXT NOT NULL,
    scope         TEXT NOT NULL DEFAULT '',
    tech_stack    TEXT NOT NULL DEFAULT '[]',
    knowledge     TEXT NOT NULL DEFAULT '[]',
    pitfalls      TEXT NOT NULL DEFAULT '[]',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    PRIMARY KEY (namespace_id, worker_id)
);

CREATE TABLE worker_diaries (
    namespace_id  TEXT NOT NULL,
    worker_id     TEXT NOT NULL,
    date          TEXT NOT NULL,
    task_id       TEXT NOT NULL DEFAULT '',
    content       TEXT NOT NULL,
    tags          TEXT NOT NULL DEFAULT '[]',
    created_at    TEXT NOT NULL,
    PRIMARY KEY (namespace_id, worker_id, date)
);

CREATE TABLE leader_diaries (
    namespace_id  TEXT NOT NULL,
    date          TEXT NOT NULL,
    entries       TEXT NOT NULL DEFAULT '[]',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    PRIMARY KEY (namespace_id, date)
);
```

---

## 四、MCP 工具（共 11 个）

### Handbook

| 工具 | 说明 |
|------|------|
| `worker_handbook_write` | 创建/更新 Worker 手册（upsert） |
| `worker_handbook_get` | 获取单个手册 |
| `worker_handbook_list` | 列出 namespace 下所有手册 |
| `find_knowledge` | 搜索 Knowledge 条目（按 topic/content/tags） |
| `find_pitfalls` | 搜索 Pitfall 条目 |

### Worker Diary

| 工具 | 说明 |
|------|------|
| `worker_diary_write` | 写 Worker 某天日记（同一天追加到已有条目） |
| `worker_diary_get` | 获取某 Worker 某天的日记 |
| `worker_diary_list` | 列出 Worker 的所有日记（日期倒序） |

### Leader Diary

| 工具 | 说明 |
|------|------|
| `leader_diary_write` | 写 Leader 日记（追加一个 Entry） |
| `leader_diary_get` | 获取某天 Leader 日记 |
| `leader_diary_list` | 列出 Leader 日记（日期倒序） |

---

## 五、工作流

### 1. Worker 初始化

```
worker_register(worker_id: "worker-auth", name: "认证服务", ...)
  →
worker_handbook_write(
    worker_id: "worker-auth",
    scope: "登录/注册/权限/OAuth",
    tech_stack: ["go", "postgres", "jwt"]
)
```

### 2. Worker 完成一个 Task

```
task_transition(T#, submit, actor_role: "worker")
  →
worker_diary_write(
    worker_id: "worker-auth",
    date: "2026-07-02",
    task_id: "T2",
    content: "## T2 登录 API 实现\n...",
    tags: ["jwt", "refresh-token"]
)
  →
worker_handbook_write(
    worker_id: "worker-auth",
    knowledge: [{ topic: "JWT Token 刷新", tags: ["jwt"], source: "T2" }]
)
  →
worker_handbook_write(
    worker_id: "worker-auth",
    pitfalls: [{ scenario: "Token 过期处理", problem: "...", source: "T2" }]
)
```

### 3. DAG 完成 / 关键节点

```
leader_diary_write(
    date: "2026-07-02",
    type: "dag_complete",
    dag_id: "login-feature",
    title: "登录功能 DAG 完成",
    content: "## 登录功能复盘\n...",
    tags: ["login", "retro"]
)
```

### 4. Leader 派新任务前查经验

```
find_knowledge(query: "WebAuthn")
  → 返回所有匹配的 Knowledge 条目（含 Worker 名、来源 task）
```

---

## 六、实现优先级

1. **`worker_handbook_write` + `worker_handbook_get` + `worker_handbook_list`**
2. **`worker_diary_write` + `worker_diary_get` + `worker_diary_list`**
3. **`leader_diary_write` + `leader_diary_get` + `leader_diary_list`**
4. **`find_knowledge` + `find_pitfalls`**
