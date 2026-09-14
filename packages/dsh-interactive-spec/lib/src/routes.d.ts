import { type HistoryServiceOptions } from './history-service.js';
export declare const DAGS_API_PATH = "/api/agentflow/dags";
export declare const DAG_API_PATH = "/api/agentflow/dag";
/**
 * Handles GET /api/agentflow/dags?cwd=...
 */
export declare function handleDagsRequest(request: Request, options?: HistoryServiceOptions): Promise<Response>;
/**
 * Handles GET /api/agentflow/dag?cwd=...&dag_id=...
 */
export declare function handleDagRequest(request: Request, options?: HistoryServiceOptions): Promise<Response>;
