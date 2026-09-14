import {
  listDagsByCwd,
  getDagDetailByCwd,
  type HistoryServiceOptions,
} from './history-service.js';

export const DAGS_API_PATH = '/api/agentflow/dags';
export const DAG_API_PATH = '/api/agentflow/dag';

/**
 * Handles GET /api/agentflow/dags?cwd=...
 */
export async function handleDagsRequest(
  request: Request,
  options?: HistoryServiceOptions
): Promise<Response> {
  const url = new URL(request.url);
  const cwd = url.searchParams.get('cwd');

  if (!cwd || !cwd.trim()) {
    return Response.json(
      { ok: false, error: 'cwd query parameter is required' },
      { status: 400, headers: { 'cache-control': 'no-store' } }
    );
  }

  const result = listDagsByCwd(cwd, options);
  const status = result.ok ? 200 : 500;

  return Response.json(result, {
    status,
    headers: { 'cache-control': 'no-store' },
  });
}

/**
 * Handles GET /api/agentflow/dag?cwd=...&dag_id=...
 */
export async function handleDagRequest(
  request: Request,
  options?: HistoryServiceOptions
): Promise<Response> {
  const url = new URL(request.url);
  const cwd = url.searchParams.get('cwd') || undefined;
  const dagId = url.searchParams.get('dag_id');
  const namespaceId = url.searchParams.get('namespace_id') || undefined;

  if (!dagId || !dagId.trim()) {
    return Response.json(
      { ok: false, error: 'dag_id query parameter is required' },
      { status: 400, headers: { 'cache-control': 'no-store' } }
    );
  }

  const result = getDagDetailByCwd({ cwd, dag_id: dagId, namespace_id: namespaceId }, options);
  let status = 200;
  if (!result.ok) {
    status = result.error?.toLowerCase().includes('not found') ? 404 : 500;
  }

  return Response.json(result, {
    status,
    headers: { 'cache-control': 'no-store' },
  });
}
