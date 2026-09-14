export * from './types';
export * from './extractor';
export * from './history-service';
export * from './routes';

import { handleDagsRequest, handleDagRequest, DAGS_API_PATH, DAG_API_PATH } from './routes';
import type { HistoryServiceOptions } from './history-service';

export const name = 'dsh-interactive-spec';

/**
 * Node/Cordis host-side plugin entry point.
 */
export function apply(ctx?: any, options?: HistoryServiceOptions): void {
  if (!ctx) return;

  const connection = Reflect.get(ctx, 'connection') ?? ctx.connection;
  if (connection && connection.fetch && typeof connection.fetch.register === 'function') {
    connection.fetch.register({
      path: DAGS_API_PATH,
      methods: ['GET'],
      requestBody: 'buffered',
      fetch: (request: Request) => handleDagsRequest(request, options),
    });

    connection.fetch.register({
      path: DAG_API_PATH,
      methods: ['GET'],
      requestBody: 'buffered',
      fetch: (request: Request) => handleDagRequest(request, options),
    });
  }

  // Support Express / Connect app if ctx mounts an HTTP server
  const app = Reflect.get(ctx, 'app') ?? ctx.app;
  if (app && typeof app.get === 'function') {
    app.get(DAGS_API_PATH, async (req: any, res: any) => {
      const host = req.headers?.host || 'localhost';
      const url = new URL(req.url, `http://${host}`);
      const request = new Request(url.toString(), { method: req.method, headers: req.headers });
      const response = await handleDagsRequest(request, options);
      const data = await response.json();
      res.status(response.status).json(data);
    });

    app.get(DAG_API_PATH, async (req: any, res: any) => {
      const host = req.headers?.host || 'localhost';
      const url = new URL(req.url, `http://${host}`);
      const request = new Request(url.toString(), { method: req.method, headers: req.headers });
      const response = await handleDagRequest(request, options);
      const data = await response.json();
      res.status(response.status).json(data);
    });
  }
}

export default {
  name,
  apply,
};
