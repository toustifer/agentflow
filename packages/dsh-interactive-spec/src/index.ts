import { readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { homedir } from 'node:os';

export * from './types.js';
export * from './extractor.js';
export * from './history-service.js';
export * from './routes.js';

import { handleDagsRequest, handleDagRequest, DAGS_API_PATH, DAG_API_PATH } from './routes.js';
import type { HistoryServiceOptions } from './history-service';

export const name = 'dsh-interactive-spec';
export const inject = ['webServer'];

const MIME_MAP: Record<string, string> = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'application/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.json': 'application/json; charset=utf-8',
};

/**
 * Node/Cordis host-side plugin entry point.
 */
export function apply(ctx?: any, options?: HistoryServiceOptions): void {
  if (!ctx) return;

  const registerEffect = (fn: () => (() => void) | void, desc?: string) => {
    if (typeof ctx.effect === 'function') {
      ctx.effect(fn, desc);
    } else {
      fn();
    }
  };

  // Register WebServer routes for canvas static assets and agentflow history APIs
  const webServer = Reflect.get(ctx, 'webServer') ?? ctx.webServer;
  if (webServer && typeof webServer.register === 'function') {
    const canvasDist = join(homedir(), '.dsh', 'plugins', 'dsh-interactive-spec', 'canvas-dist');

    registerEffect(() => webServer.register({
      kind: 'prefix',
      path: '/agentflow/canvas',
      handler: async (req: any, res: any) => {
        try {
          const urlObj = new URL(String(req.url), 'http://127.0.0.1');
          let subPath = urlObj.pathname.slice('/agentflow/canvas'.length);
          if (subPath === '' || subPath === '/') subPath = '/index.html';

          const targetFile = join(canvasDist, subPath);
          const ext = subPath.slice(subPath.lastIndexOf('.'));
          const mime = MIME_MAP[ext] || 'application/octet-stream';

          const content = await readFile(targetFile);
          res.setHeader('Content-Type', mime);
          res.setHeader('Access-Control-Allow-Origin', '*');
          res.statusCode = 200;
          res.end(content);
        } catch {
          res.statusCode = 404;
          res.end('Not Found');
        }
      },
    }), 'dsh-interactive-spec: canvas static route');

    registerEffect(() => webServer.register({
      kind: 'exact',
      path: DAGS_API_PATH,
      handler: async (req: any, res: any) => {
        try {
          const url = new URL(String(req.url), `http://${req.headers?.host || '127.0.0.1'}`);
          const request = new Request(url.toString(), { method: req.method, headers: req.headers });
          const response = await handleDagsRequest(request, options);
          const data = await response.json();
          res.statusCode = response.status;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.setHeader('Access-Control-Allow-Origin', '*');
          res.setHeader('Cache-Control', 'no-store');
          res.end(JSON.stringify(data));
        } catch (err) {
          res.statusCode = 500;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.end(JSON.stringify({ ok: false, error: String(err) }));
        }
      },
    }), 'dsh-interactive-spec: /api/agentflow/dags exact route');

    registerEffect(() => webServer.register({
      kind: 'exact',
      path: DAG_API_PATH,
      handler: async (req: any, res: any) => {
        try {
          const url = new URL(String(req.url), `http://${req.headers?.host || '127.0.0.1'}`);
          const request = new Request(url.toString(), { method: req.method, headers: req.headers });
          const response = await handleDagRequest(request, options);
          const data = await response.json();
          res.statusCode = response.status;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.setHeader('Access-Control-Allow-Origin', '*');
          res.setHeader('Cache-Control', 'no-store');
          res.end(JSON.stringify(data));
        } catch (err) {
          res.statusCode = 500;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.end(JSON.stringify({ ok: false, error: String(err) }));
        }
      },
    }), 'dsh-interactive-spec: /api/agentflow/dag exact route');
  }

  // Register Connection Fetch routes if available
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
  inject,
  apply,
};
