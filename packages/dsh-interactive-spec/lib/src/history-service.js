import { DatabaseSync } from 'node:sqlite';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
/**
 * Normalizes file path for cross-platform case-insensitive comparison.
 */
export function normalizePath(p) {
    if (!p)
        return '';
    return p
        .trim()
        .replace(/\\+/g, '/')
        .replace(/\/+$/, '')
        .toLowerCase();
}
/**
 * Parses a JSON string safely, returning fallback if invalid or null.
 */
function safeJsonParse(raw, fallback) {
    if (typeof raw !== 'string' || !raw.trim())
        return fallback;
    try {
        const val = JSON.parse(raw);
        return (val === null || val === undefined) ? fallback : val;
    }
    catch {
        return fallback;
    }
}
/**
 * Resolves the Agentflow SQLite database file path.
 * Checks candidate databases and prefers the one containing non-empty namespaces/dags.
 */
export function resolveAgentflowDbPath(customPath) {
    if (customPath) {
        return fs.existsSync(customPath) ? customPath : null;
    }
    const candidates = [];
    const envPath = process.env.AGENTFLOW_DB_PATH;
    if (envPath && fs.existsSync(envPath))
        candidates.push(envPath);
    const tempDb = path.join(process.env.TEMP || os.tmpdir(), 'agentflow.db');
    if (fs.existsSync(tempDb))
        candidates.push(tempDb);
    const userDshDb = path.join(os.homedir(), '.dsh', 'agentflow', 'agentflow.db');
    if (fs.existsSync(userDshDb))
        candidates.push(userDshDb);
    for (const p of candidates) {
        try {
            const db = new DatabaseSync(p, { open: true, readOnly: true });
            const row = db.prepare('SELECT COUNT(*) as count FROM namespaces').get();
            db.close();
            if (row && row.count > 0) {
                return p;
            }
        }
        catch {
            // ignore
        }
    }
    return candidates.length > 0 ? candidates[0] : null;
}
/**
 * Opens a SQLite database synchronously, returning null on failure or if path is invalid.
 */
export function openDatabase(dbPath) {
    const resolved = resolveAgentflowDbPath(dbPath);
    if (!resolved) {
        return null;
    }
    try {
        return new DatabaseSync(resolved, { open: true, readOnly: true });
    }
    catch {
        // If readOnly fails on new sqlite versions, retry default
        try {
            return new DatabaseSync(resolved);
        }
        catch {
            return null;
        }
    }
}
/**
 * Finds matching namespace for a given cwd.
 * Matches by exact workdir -> subpath -> directory name -> fallback to sole namespace.
 */
export function findNamespaceByCwd(db, cwd) {
    if (!cwd)
        return null;
    let rows = [];
    try {
        const stmt = db.prepare('SELECT id, name, metadata, created_at, updated_at FROM namespaces ORDER BY updated_at DESC');
        rows = stmt.all();
    }
    catch {
        return null;
    }
    if (rows.length === 0)
        return null;
    const targetNormalized = normalizePath(cwd);
    const targetBase = path.basename(cwd).trim().toLowerCase();
    const namespaces = rows.map((r) => ({
        id: String(r.id),
        name: String(r.name),
        metadata: safeJsonParse(r.metadata, {}),
        created_at: r.created_at,
        updated_at: r.updated_at,
    }));
    // 1. Exact match on metadata.workdir
    for (const ns of namespaces) {
        const workdir = ns.metadata['workdir'];
        if (typeof workdir === 'string' && workdir.trim()) {
            if (normalizePath(workdir) === targetNormalized) {
                return ns;
            }
        }
    }
    // 2. Subpath / prefix match (e.g. worktree inside workdir or vice-versa)
    for (const ns of namespaces) {
        const workdir = ns.metadata['workdir'];
        if (typeof workdir === 'string' && workdir.trim()) {
            const wdNorm = normalizePath(workdir);
            if (targetNormalized.startsWith(wdNorm + '/') || wdNorm.startsWith(targetNormalized + '/')) {
                return ns;
            }
        }
    }
    // 3. Basename fuzzy match on namespace id or name
    for (const ns of namespaces) {
        const idLower = ns.id.toLowerCase();
        const nameLower = ns.name.toLowerCase();
        if (idLower === targetBase || nameLower === targetBase || idLower.includes(targetBase) || targetBase.includes(idLower)) {
            return ns;
        }
    }
    // 4. Fallback: if only 1 namespace exists in database, use it
    if (namespaces.length === 1) {
        return namespaces[0];
    }
    return null;
}
/**
 * Maps database task state to LiveSpec TaskState.
 */
function mapTaskState(dbState) {
    switch (dbState) {
        case 'done':
        case 'passed':
            return 'pass';
        case 'rework_needed':
            return 'rework';
        case 'review_pending':
            return 'submitted';
        case 'executing':
            return 'executing';
        case 'assigned':
            return 'ready';
        case 'cancelled':
            return 'cancelled';
        default:
            return dbState || 'pending';
    }
}
/**
 * Queries all DAGs under the namespace matched by cwd.
 */
export function listDagsByCwd(cwd, options) {
    const db = openDatabase(options?.dbPath);
    if (!db) {
        return {
            ok: false,
            namespace_id: null,
            cwd,
            dags: [],
            error: 'Agentflow database not found or inaccessible',
        };
    }
    try {
        const ns = findNamespaceByCwd(db, cwd);
        if (!ns) {
            return {
                ok: true,
                namespace_id: null,
                cwd,
                dags: [],
            };
        }
        const query = `
      SELECT
        d.id,
        d.title,
        d.status,
        d.created_at,
        d.updated_at,
        d.metadata,
        COUNT(t.id) AS total_tasks,
        SUM(CASE WHEN t.state IN ('done', 'pass', 'passed') THEN 1 ELSE 0 END) AS done_tasks
      FROM dags d
      LEFT JOIN tasks t ON t.namespace_id = d.namespace_id AND t.dag_id = d.id
      WHERE d.namespace_id = ?
      GROUP BY d.id
      ORDER BY d.created_at DESC
    `;
        const stmt = db.prepare(query);
        const rows = stmt.all(ns.id);
        const dags = rows.map((r) => ({
            id: String(r.id),
            title: String(r.title),
            status: String(r.status || 'planning'),
            created_at: String(r.created_at || ''),
            total_tasks: Number(r.total_tasks || 0),
            done_tasks: Number(r.done_tasks || 0),
        }));
        return {
            ok: true,
            namespace_id: ns.id,
            cwd,
            dags,
        };
    }
    catch (err) {
        return {
            ok: false,
            namespace_id: null,
            cwd,
            dags: [],
            error: err?.message || String(err),
        };
    }
    finally {
        db.close();
    }
}
/**
 * Queries details and tasks of a specific DAG, returning formatted LiveSpecDoc.
 */
export function getDagDetailByCwd(params, options) {
    const { cwd, dag_id, namespace_id } = params;
    if (!dag_id) {
        return { ok: false, error: 'dag_id parameter is required' };
    }
    const db = openDatabase(options?.dbPath);
    if (!db) {
        return { ok: false, error: 'Agentflow database not found or inaccessible' };
    }
    try {
        let targetNsId = namespace_id;
        if (!targetNsId && cwd) {
            const ns = findNamespaceByCwd(db, cwd);
            if (ns) {
                targetNsId = ns.id;
            }
        }
        // Find DAG
        let dagRow = null;
        if (targetNsId) {
            const stmt = db.prepare('SELECT * FROM dags WHERE namespace_id = ? AND id = ? LIMIT 1');
            dagRow = stmt.get(targetNsId, dag_id);
        }
        if (!dagRow) {
            const stmt = db.prepare('SELECT * FROM dags WHERE id = ? LIMIT 1');
            dagRow = stmt.get(dag_id);
        }
        if (!dagRow) {
            return { ok: false, error: `DAG not found: ${dag_id}` };
        }
        // Query Tasks
        const tasksStmt = db.prepare(`
      SELECT * FROM tasks
      WHERE dag_id = ? AND (namespace_id = ? OR ? IS NULL)
      ORDER BY priority DESC, created_at ASC
    `);
        const taskRows = tasksStmt.all(dag_id, targetNsId || null, targetNsId || null);
        const dagMeta = safeJsonParse(dagRow.metadata, {});
        const maxConcurrency = typeof dagMeta.max_concurrency === 'number'
            ? dagMeta.max_concurrency
            : typeof dagMeta.concurrency === 'number'
                ? dagMeta.concurrency
                : 2;
        const tasks = taskRows.map((t) => {
            let dependsOn = [];
            try {
                const parsed = safeJsonParse(t.depends_on, []);
                dependsOn = Array.isArray(parsed) ? parsed : [];
            }
            catch { }
            let criteria = [];
            try {
                const parsed = safeJsonParse(t.acceptance_criteria, []);
                criteria = Array.isArray(parsed) ? parsed : [];
            }
            catch { }
            let outputFiles = [];
            try {
                const parsed = safeJsonParse(t.output_files, []);
                outputFiles = Array.isArray(parsed) ? parsed : [];
            }
            catch { }
            let tags = [];
            try {
                const parsed = safeJsonParse(t.tags, []);
                tags = Array.isArray(parsed) ? parsed : [];
            }
            catch { }
            let metadata = {};
            try {
                const parsed = safeJsonParse(t.metadata, {});
                metadata = parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
            }
            catch { }
            return {
                id: String(t.id),
                task_id: String(t.id),
                title: String(t.title),
                description: t.description ? String(t.description) : undefined,
                assigned_worker: t.assigned_worker ? String(t.assigned_worker) : undefined,
                depends_on: dependsOn,
                state: mapTaskState(String(t.state || 'assigned')),
                estimated_hours: typeof t.estimated_hours === 'number' ? t.estimated_hours : Number(t.estimated_hours || 0),
                priority: typeof t.priority === 'number' ? t.priority : Number(t.priority || 0),
                acceptance_criteria: criteria.length > 0 ? criteria : undefined,
                output_files: outputFiles.length > 0 ? outputFiles : undefined,
                tags: tags.length > 0 ? tags : undefined,
                metadata: Object.keys(metadata).length > 0 ? metadata : undefined,
            };
        });
        const spec = {
            version: '1.0.0',
            dag_id: String(dagRow.id),
            title: String(dagRow.title),
            namespace_id: targetNsId || String(dagRow.namespace_id),
            parameters: {
                max_concurrency: maxConcurrency,
                ...dagMeta,
            },
            tasks,
        };
        return {
            ok: true,
            spec,
        };
    }
    catch (err) {
        throw err;
    }
    finally {
        db.close();
    }
}
