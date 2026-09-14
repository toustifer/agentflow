import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { DatabaseSync } from 'node:sqlite';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
import {
  listDagsByCwd,
  getDagDetailByCwd,
  findNamespaceByCwd,
  handleDagsRequest,
  handleDagRequest,
  apply,
  DAGS_API_PATH,
  DAG_API_PATH,
} from '../src/index';

describe('Agentflow History API', () => {
  let tmpDir: string;
  let testDbPath: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'af-test-'));
    testDbPath = path.join(tmpDir, 'test-agentflow.db');

    // Create test database schema and seed data
    const db = new DatabaseSync(testDbPath);
    db.exec(`
      CREATE TABLE namespaces (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        metadata TEXT NOT NULL DEFAULT '{}'
      );

      CREATE TABLE dags (
        id TEXT NOT NULL,
        namespace_id TEXT NOT NULL,
        title TEXT NOT NULL,
        branch TEXT NOT NULL DEFAULT '',
        execution_branch TEXT NOT NULL DEFAULT '',
        base_branch TEXT NOT NULL DEFAULT '',
        metadata TEXT NOT NULL DEFAULT '{}',
        worktree_path TEXT NOT NULL DEFAULT '',
        worktree_status TEXT NOT NULL DEFAULT '',
        head_sha TEXT NOT NULL DEFAULT '',
        active_task_id TEXT NOT NULL DEFAULT '',
        lease_holder_task_id TEXT NOT NULL DEFAULT '',
        lease_holder_worker_id TEXT NOT NULL DEFAULT '',
        lease_holder_agent_id TEXT NOT NULL DEFAULT '',
        lease_acquired_at TEXT NOT NULL DEFAULT '',
        runtime_updated_at TEXT NOT NULL DEFAULT '',
        status TEXT NOT NULL DEFAULT 'planning',
        priority TEXT NOT NULL DEFAULT 'P2',
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        PRIMARY KEY (namespace_id, id)
      );

      CREATE TABLE tasks (
        id TEXT NOT NULL,
        namespace_id TEXT NOT NULL,
        title TEXT NOT NULL,
        description TEXT NOT NULL DEFAULT '',
        state TEXT NOT NULL DEFAULT 'assigned',
        assigned_worker TEXT NOT NULL DEFAULT '',
        acceptance_criteria TEXT NOT NULL DEFAULT '[]',
        output_files TEXT NOT NULL DEFAULT '[]',
        dag_id TEXT NOT NULL DEFAULT '',
        depends_on TEXT NOT NULL DEFAULT '[]',
        tags TEXT NOT NULL DEFAULT '[]',
        priority INTEGER NOT NULL DEFAULT 0,
        estimated_hours REAL NOT NULL DEFAULT 0,
        actual_hours REAL NOT NULL DEFAULT 0,
        worker_agent_id TEXT NOT NULL DEFAULT '',
        review_cycle INTEGER NOT NULL DEFAULT 0,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        metadata TEXT NOT NULL DEFAULT '{}',
        PRIMARY KEY (namespace_id, id)
      );
    `);

    // Seed namespaces
    const insertNs = db.prepare(`
      INSERT INTO namespaces (id, name, created_at, updated_at, metadata)
      VALUES (?, ?, ?, ?, ?)
    `);
    insertNs.run(
      'insighttutor',
      '知迹伴学',
      '2026-03-01T10:00:00Z',
      '2026-03-01T10:00:00Z',
      JSON.stringify({ workdir: 'D:\\myprogram\\InsightTutor' })
    );
    insertNs.run(
      'other-project',
      'Other Project',
      '2026-03-02T10:00:00Z',
      '2026-03-02T10:00:00Z',
      JSON.stringify({ workdir: 'D:/myprogram/Other' })
    );

    // Seed DAGs
    const insertDag = db.prepare(`
      INSERT INTO dags (id, namespace_id, title, status, metadata, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, ?)
    `);
    insertDag.run(
      'dag-invite-referral-growth',
      'insighttutor',
      '全员推荐裂变：通用邀请码与返现激励',
      'in_progress',
      JSON.stringify({ max_concurrency: 2 }),
      '2026-03-10T12:00:00Z',
      '2026-03-10T12:00:00Z'
    );
    insertDag.run(
      'dag-ai-tutor-dialogue',
      'insighttutor',
      '智能对话风控与流式响应优化',
      'completed',
      JSON.stringify({ max_concurrency: 3 }),
      '2026-03-05T08:00:00Z',
      '2026-03-05T08:00:00Z'
    );

    // Seed tasks for dag-invite-referral-growth
    const insertTask = db.prepare(`
      INSERT INTO tasks (id, namespace_id, dag_id, title, state, assigned_worker, depends_on, estimated_hours, created_at, updated_at)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `);
    insertTask.run(
      'ir-01',
      'insighttutor',
      'dag-invite-referral-growth',
      '数据表设计与邀请码生成服务',
      'done',
      'backend-dev',
      JSON.stringify([]),
      2,
      '2026-03-10T12:01:00Z',
      '2026-03-10T12:01:00Z'
    );
    insertTask.run(
      'ir-02',
      'insighttutor',
      'dag-invite-referral-growth',
      '裂变邀请前端界面与海报生成',
      'executing',
      'frontend-dev',
      JSON.stringify(['ir-01']),
      3,
      '2026-03-10T12:02:00Z',
      '2026-03-10T12:02:00Z'
    );

    db.close();
  });

  afterEach(() => {
    try {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    } catch {
      // Ignore cleanup error on windows
    }
  });

  describe('findNamespaceByCwd', () => {
    it('matches namespace by exact workdir with backslash normalization', () => {
      const db = new DatabaseSync(testDbPath);
      try {
        const ns = findNamespaceByCwd(db, 'D:/myprogram/InsightTutor');
        expect(ns).not.toBeNull();
        expect(ns?.id).toBe('insighttutor');
      } finally {
        db.close();
      }
    });

    it('fuzzy matches namespace by folder basename fallback', () => {
      const db = new DatabaseSync(testDbPath);
      try {
        const ns = findNamespaceByCwd(db, 'C:/another/path/InsightTutor');
        expect(ns).not.toBeNull();
        expect(ns?.id).toBe('insighttutor');
      } finally {
        db.close();
      }
    });

    it('matches namespace when cwd is a subfolder of workdir', () => {
      const db = new DatabaseSync(testDbPath);
      try {
        const ns = findNamespaceByCwd(db, 'D:/myprogram/InsightTutor/apps/mobile');
        expect(ns).not.toBeNull();
        expect(ns?.id).toBe('insighttutor');
      } finally {
        db.close();
      }
    });

    it('returns null when cwd has no match and multiple namespaces exist', () => {
      const db = new DatabaseSync(testDbPath);
      try {
        const ns = findNamespaceByCwd(db, 'E:/totally/unrelated/path/xyz');
        expect(ns).toBeNull();
      } finally {
        db.close();
      }
    });
  });

  describe('listDagsByCwd', () => {
    it('lists all dags for matched namespace with task counts ordered by created_at DESC', () => {
      const result = listDagsByCwd('D:\\myprogram\\InsightTutor', { dbPath: testDbPath });
      expect(result.ok).toBe(true);
      expect(result.namespace_id).toBe('insighttutor');
      expect(result.cwd).toBe('D:\\myprogram\\InsightTutor');
      expect(result.dags).toHaveLength(2);

      // Latest first
      expect(result.dags[0].id).toBe('dag-invite-referral-growth');
      expect(result.dags[0].title).toBe('全员推荐裂变：通用邀请码与返现激励');
      expect(result.dags[0].status).toBe('in_progress');
      expect(result.dags[0].total_tasks).toBe(2);
      expect(result.dags[0].done_tasks).toBe(1);

      expect(result.dags[1].id).toBe('dag-ai-tutor-dialogue');
      expect(result.dags[1].total_tasks).toBe(0);
      expect(result.dags[1].done_tasks).toBe(0);
    });

    it('gracefully handles non-existent database file', () => {
      const result = listDagsByCwd('D:\\myprogram\\InsightTutor', {
        dbPath: path.join(tmpDir, 'non-existent.db'),
      });
      expect(result.ok).toBe(false);
      expect(result.dags).toEqual([]);
      expect(result.error).toBeDefined();
    });
  });

  describe('getDagDetailByCwd', () => {
    it('returns DAG details formatted as LiveSpecDoc', () => {
      const result = getDagDetailByCwd(
        { cwd: 'D:\\myprogram\\InsightTutor', dag_id: 'dag-invite-referral-growth' },
        { dbPath: testDbPath }
      );
      expect(result.ok).toBe(true);
      expect(result.spec).toBeDefined();
      const spec = result.spec!;
      expect(spec.dag_id).toBe('dag-invite-referral-growth');
      expect(spec.title).toBe('全员推荐裂变：通用邀请码与返现激励');
      expect(spec.parameters?.max_concurrency).toBe(2);
      expect(spec.tasks).toHaveLength(2);

      const t1 = spec.tasks.find((t: any) => t.task_id === 'ir-01' || t.id === 'ir-01')!;
      expect(t1).toBeDefined();
      expect(t1.title).toBe('数据表设计与邀请码生成服务');
      expect(t1.assigned_worker).toBe('backend-dev');
      expect(t1.depends_on).toEqual([]);
      expect(t1.state).toBe('pass');
      expect(t1.estimated_hours).toBe(2);

      const t2 = spec.tasks.find((t: any) => t.task_id === 'ir-02' || t.id === 'ir-02')!;
      expect(t2.depends_on).toEqual(['ir-01']);
      expect(t2.state).toBe('executing');
    });

    it('gracefully returns error when dag_id is not found', () => {
      const result = getDagDetailByCwd(
        { cwd: 'D:\\myprogram\\InsightTutor', dag_id: 'non-existent-dag' },
        { dbPath: testDbPath }
      );
      expect(result.ok).toBe(false);
      expect(result.error).toContain('not found');
    });

    it('finds DAG by dag_id without providing cwd', () => {
      const result = getDagDetailByCwd(
        { dag_id: 'dag-invite-referral-growth' },
        { dbPath: testDbPath }
      );
      expect(result.ok).toBe(true);
      expect(result.spec?.dag_id).toBe('dag-invite-referral-growth');
    });

    it('gracefully returns error when db is not found in getDagDetailByCwd', () => {
      const result = getDagDetailByCwd(
        { dag_id: 'dag-invite-referral-growth' },
        { dbPath: path.join(tmpDir, 'missing.db') }
      );
      expect(result.ok).toBe(false);
      expect(result.error).toBeDefined();
    });
  });

  describe('HTTP Route Handlers', () => {
    it('handleDagsRequest handles valid cwd query', async () => {
      const req = new Request(`http://localhost${DAGS_API_PATH}?cwd=${encodeURIComponent('D:\\myprogram\\InsightTutor')}`);
      const res = await handleDagsRequest(req, { dbPath: testDbPath });
      expect(res.status).toBe(200);
      const json = await res.json();
      expect(json.ok).toBe(true);
      expect(json.namespace_id).toBe('insighttutor');
      expect(json.dags).toHaveLength(2);
    });

    it('handleDagsRequest returns 400 when cwd is missing', async () => {
      const req = new Request(`http://localhost${DAGS_API_PATH}`);
      const res = await handleDagsRequest(req, { dbPath: testDbPath });
      expect(res.status).toBe(400);
      const json = await res.json();
      expect(json.ok).toBe(false);
      expect(json.error).toBeDefined();
    });

    it('handleDagRequest handles valid cwd and dag_id queries', async () => {
      const req = new Request(
        `http://localhost${DAG_API_PATH}?cwd=${encodeURIComponent('D:\\myprogram\\InsightTutor')}&dag_id=dag-invite-referral-growth`
      );
      const res = await handleDagRequest(req, { dbPath: testDbPath });
      expect(res.status).toBe(200);
      const json = await res.json();
      expect(json.ok).toBe(true);
      expect(json.spec?.dag_id).toBe('dag-invite-referral-growth');
      expect(json.spec?.tasks).toHaveLength(2);
    });

    it('handleDagRequest returns 400 when dag_id is missing', async () => {
      const req = new Request(
        `http://localhost${DAG_API_PATH}?cwd=${encodeURIComponent('D:\\myprogram\\InsightTutor')}`
      );
      const res = await handleDagRequest(req, { dbPath: testDbPath });
      expect(res.status).toBe(400);
      const json = await res.json();
      expect(json.ok).toBe(false);
    });

    it('handleDagRequest returns 404 when dag is not found', async () => {
      const req = new Request(
        `http://localhost${DAG_API_PATH}?cwd=${encodeURIComponent('D:\\myprogram\\InsightTutor')}&dag_id=unknown-dag`
      );
      const res = await handleDagRequest(req, { dbPath: testDbPath });
      expect(res.status).toBe(404);
      const json = await res.json();
      expect(json.ok).toBe(false);
    });
  });

  describe('Cordis apply() plugin lifecycle', () => {
    it('registers routes to ctx.connection.fetch', () => {
      const registeredRoutes: any[] = [];
      const mockCtx = {
        connection: {
          fetch: {
            register: (route: any) => {
              registeredRoutes.push(route);
            },
          },
        },
      };

      apply(mockCtx, { dbPath: testDbPath });

      expect(registeredRoutes).toHaveLength(2);
      expect(registeredRoutes.some((r) => r.path === DAGS_API_PATH)).toBe(true);
      expect(registeredRoutes.some((r) => r.path === DAG_API_PATH)).toBe(true);
    });
  });
});
