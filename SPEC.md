# agentflow SPEC — Worker Handbook & Diary（MCP 结构化存储）

## 目标

把 `.mycompany/workers/{id}/handbook.json` 和 `diary/{date}.md` 这套「Worker 手册 + 工作日记」从文件系统搬到 MCP 后端，让 agent 通过 MCP 接口读写。

**核心价值**：
- Leader 派新任务前可以查询历史经验
- Worker 任务交付时可以附上调研笔记
- 跨项目跨 Worker 复用知识

---

## 数据模型

### WorkerHandbook

```go
type WorkerHandbook struct {
    WorkerID    string
    NamespaceID string
    
    Scope       string            // 领域描述（如 "登录/注册/权限/OAuth"）
    TechStack   []string          // 技术栈（go, postgres, redis...）
    BusinessFlow []string         // 核心业务流程（步骤列表）
    CodeMap     map[string]string // 文件路径 → 用途说明
    DangerZones []string          // 风险文件/模块说明
    ExternalDeps []string         // 外部依赖（数据库、API、服务）
    
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### WorkerDiary

```go
type WorkerDiary struct {
    WorkerID    string
    NamespaceID string
    Date        string    // YYYY-MM-DD
    Content     string    // Markdown 内容
    Tags        []string  // 任务ID标签等
    
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### Storage

新增两张 SQLite 表：

```sql
CREATE TABLE worker_handbooks (
    namespace_id TEXT NOT NULL,
    worker_id    TEXT NOT NULL,
    scope        TEXT NOT NULL DEFAULT '',
    tech_stack   TEXT NOT NULL DEFAULT '[]',
    business_flow TEXT NOT NULL DEFAULT '[]',
    code_map     TEXT NOT NULL DEFAULT '{}',
    danger_zones TEXT NOT NULL DEFAULT '[]',
    external_deps TEXT NOT NULL DEFAULT '[]',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (namespace_id, worker_id)
);

CREATE TABLE worker_diaries (
    namespace_id TEXT NOT NULL,
    worker_id    TEXT NOT NULL,
    date         TEXT NOT NULL,
    content      TEXT NOT NULL,
    tags         TEXT NOT NULL DEFAULT '[]',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    PRIMARY KEY (namespace_id, worker_id, date)
);
```

---

## MCP 工具

### Handbook 工具

| 工具 | 说明 |
|------|------|
| `worker_handbook_write` | 写入或更新 Worker 手册（upsert） |
| `worker_handbook_get` | 获取单个 Worker 手册 |
| `worker_handbook_list` | 列出 namespace 下所有手册 |
| `find_handbooks` | 搜索手册（按 scope/tech_stack/danger_zones 关键字匹配） |

### Diary 工具

| 工具 | 说明 |
|------|------|
| `worker_diary_write` | 写入 Worker 某天的工作日记 |
| `worker_diary_get` | 获取某 Worker 某天的日记 |
| `worker_diary_list` | 列出 Worker 的所有日记（按日期倒序） |
| `find_diaries` | 搜索日记（按 tag 或关键字） |

总计新增 **8 个 MCP 工具**。

---

## 工作流

### Worker 初始化（在 worker_register 之后）

```
Leader → worker_register(worker_id, name, scope)
Agent 调用 → worker_handbook_write(
    worker_id,
    scope: "登录/注册/权限/OAuth",
    tech_stack: ["go", "postgres", "jwt"],
    business_flow: ["用户登录", "token签发", "权限校验"],
    code_map: {"services/auth.go": "认证服务主入口"},
    danger_zones: ["services/auth.go — 修改前需要做影响评估"],
    external_deps: ["postgres", "redis"]
)
```

### Worker 任务完成后写日记

```
Worker 完成 T1 (登录 API)
  ↓
Worker → worker_diary_write(
    worker_id: "worker-auth",
    date: "2026-07-02",
    content: "## T1 登录 API 完成报告\n- 实现 JWT 签发\n- 密码采用 bcrypt...",
    tags: ["T1", "jwt", "security"]
)
```

### Leader 派任务前查询经验

```
Leader: "T5 要集成 WebAuthn，看 worker-fe 有没有经验"
  ↓
Agent → find_handbooks(
    query: "WebAuthn",
    namespace_id: "myapp"
)
→ 返回 worker-fe 的 handbook 摘要（如果有相关条目）

Agent → find_diaries(
    query: "WebAuthn",
    worker_id: "worker-fe"  // 可选
)
→ 返回 worker-fe 历史日记中提到 WebAuthn 的条目
```

---

## 接口细节

### worker_handbook_write

```json
{
  "namespace_id": "myapp",
  "worker_id": "worker-auth",
  "scope": "登录/注册/权限/OAuth",
  "tech_stack": ["go", "postgres", "jwt"],
  "business_flow": ["用户登录", "token签发"],
  "code_map": {
    "services/auth.go": "认证服务主入口",
    "services/token.go": "JWT 签发和验证"
  },
  "danger_zones": ["services/auth.go — 修改需评估影响"],
  "external_deps": ["postgres", "redis"]
}
```

### find_handbooks

按关键字搜索 scope、tech_stack、code_map 值、danger_zones、external_deps 中的所有文本。返回匹配的 handbook 列表（按匹配度排序）。

```json
{
  "namespace_id": "myapp",
  "query": "JWT"
}
```

### find_diaries

按 tag 精确匹配 + content 关键字模糊匹配。

```json
{
  "namespace_id": "myapp",
  "worker_id": "worker-auth",  // 可选
  "query": "JWT",
  "limit": 10
}
```

---

## 后续集成

### Phase 7: 关联到 Task

- Task 可选关联 handbook 引用（`metadata.handbook_ref`）
- Task 交付时自动写 diary
- Leader 派任务时自动 find 相关经验

### Phase 8: 跨 namespace 复用

- handbook 可标记 `reusable: true`
- 跨项目知识库（需要谨慎，仅某些领域）

---

## 实现优先级

1. **worker_handbook_write** + **worker_handbook_get**（最常用）
2. **worker_diary_write** + **worker_diary_get** + **worker_diary_list**
3. **find_handbooks** + **find_diaries**（查询接口）
4. **worker_handbook_list**（管理用）

先做 1-2，再加 3-4。