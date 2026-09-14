import { DatabaseSync } from 'node:sqlite';
import type { LiveSpecDoc } from '@agentflow/live-spec-core';
export interface HistoryServiceOptions {
    dbPath?: string;
}
export interface NamespaceRecord {
    id: string;
    name: string;
    metadata: Record<string, unknown>;
    created_at?: string;
    updated_at?: string;
}
export interface DagSummary {
    id: string;
    title: string;
    status: string;
    created_at: string;
    total_tasks: number;
    done_tasks: number;
}
export interface ListDagsResult {
    ok: boolean;
    namespace_id: string | null;
    cwd: string;
    dags: DagSummary[];
    error?: string;
}
export interface DagDetailResult {
    ok: boolean;
    spec?: LiveSpecDoc;
    error?: string;
}
/**
 * Normalizes file path for cross-platform case-insensitive comparison.
 */
export declare function normalizePath(p: string): string;
/**
 * Resolves the Agentflow SQLite database file path.
 */
export declare function resolveAgentflowDbPath(customPath?: string): string | null;
/**
 * Opens a SQLite database synchronously, returning null on failure or if path is invalid.
 */
export declare function openDatabase(dbPath?: string): DatabaseSync | null;
/**
 * Finds matching namespace for a given cwd.
 * Matches by exact workdir -> subpath -> directory name -> fallback to sole namespace.
 */
export declare function findNamespaceByCwd(db: DatabaseSync, cwd: string): NamespaceRecord | null;
/**
 * Queries all DAGs under the namespace matched by cwd.
 */
export declare function listDagsByCwd(cwd: string, options?: HistoryServiceOptions): ListDagsResult;
/**
 * Queries details and tasks of a specific DAG, returning formatted LiveSpecDoc.
 */
export declare function getDagDetailByCwd(params: {
    cwd?: string;
    dag_id: string;
    namespace_id?: string;
}, options?: HistoryServiceOptions): DagDetailResult;
