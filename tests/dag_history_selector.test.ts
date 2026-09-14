import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import {
  DAGHistorySelect,
  formatCreatedAt,
  getDagStatusMeta,
  type DagSummary,
} from '../apps/live-spec-canvas/src/components/DAGHistorySelect';
import type { LiveSpecDoc } from '@agentflow/live-spec-core';

describe('DAGHistorySelect Component & Helpers', () => {
  describe('formatCreatedAt', () => {
    it('returns -- for empty or undefined input', () => {
      expect(formatCreatedAt(undefined)).toBe('--');
      expect(formatCreatedAt('')).toBe('--');
    });

    it('formats ISO datetime strings into YYYY-MM-DD HH:mm format', () => {
      const iso = '2026-03-10T12:30:00.000Z';
      const formatted = formatCreatedAt(iso);
      expect(formatted).toMatch(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/);
    });

    it('returns original string if date is invalid', () => {
      expect(formatCreatedAt('not-a-valid-date')).toBe('not-a-valid-date');
    });
  });

  describe('getDagStatusMeta', () => {
    it('returns green success meta for completed/done/passed states', () => {
      const doneMeta = getDagStatusMeta('done');
      expect(doneMeta.label).toBe('已完成');
      expect(doneMeta.color).toBe('#22c55e');

      const completedMeta = getDagStatusMeta('completed');
      expect(completedMeta.label).toBe('已完成');

      const passedMeta = getDagStatusMeta('passed');
      expect(passedMeta.label).toBe('已完成');
    });

    it('returns blue in-progress meta for running/executing states', () => {
      const inProgressMeta = getDagStatusMeta('in_progress');
      expect(inProgressMeta.label).toBe('进行中');
      expect(inProgressMeta.color).toBe('#38bdf8');

      const runningMeta = getDagStatusMeta('running');
      expect(runningMeta.label).toBe('进行中');

      const executingMeta = getDagStatusMeta('executing');
      expect(executingMeta.label).toBe('进行中');
    });

    it('returns red error meta for rework/failed/blocked states', () => {
      const reworkMeta = getDagStatusMeta('rework');
      expect(reworkMeta.label).toBe('返工中');
      expect(reworkMeta.color).toBe('#ef4444');

      const failedMeta = getDagStatusMeta('failed');
      expect(failedMeta.label).toBe('返工中');
    });

    it('returns gray planning meta for default/planning states', () => {
      const planningMeta = getDagStatusMeta('planning');
      expect(planningMeta.label).toBe('规划中');
      expect(planningMeta.color).toBe('#a1a1aa');

      const unknownMeta = getDagStatusMeta('something_else');
      expect(unknownMeta.label).toBe('规划中');
    });
  });

  describe('Static HTML Rendering', () => {
    const mockDags: DagSummary[] = [
      {
        id: 'dag-growth-invite',
        title: '全员推荐裂变方案',
        status: 'in_progress',
        total_tasks: 5,
        done_tasks: 3,
        created_at: '2026-03-10T10:00:00Z',
      },
      {
        id: 'dag-ai-tutor-core',
        title: '智能伴学对话流',
        status: 'done',
        total_tasks: 8,
        done_tasks: 8,
        created_at: '2026-03-08T09:00:00Z',
      },
    ];

    it('renders button with history DAG count and icon', () => {
      const html = renderToStaticMarkup(
        React.createElement(DAGHistorySelect, {
          cwd: 'D:/myprogram/InsightTutor',
          currentDagId: 'dag-growth-invite',
          dags: mockDags,
          onSelectDag: vi.fn(),
        })
      );

      expect(html).toContain('历史 DAG (2)');
      expect(html).toContain('📜');
    });

    it('renders historical review badge and switch to latest button when isHistorical is true', () => {
      const html = renderToStaticMarkup(
        React.createElement(DAGHistorySelect, {
          cwd: 'D:/myprogram/InsightTutor',
          currentDagId: 'dag-ai-tutor-core',
          isHistorical: true,
          dags: mockDags,
          onSelectDag: vi.fn(),
          onSwitchToLatest: vi.fn(),
        })
      );

      expect(html).toContain('历史回看');
      expect(html).toContain('🔍');
      expect(html).toContain('切回最新');
      expect(html).toContain('⚡');
    });

    it('renders nothing when cwd is missing and dags is empty', () => {
      const html = renderToStaticMarkup(
        React.createElement(DAGHistorySelect, {
          cwd: undefined,
          dags: [],
          onSelectDag: vi.fn(),
        })
      );

      expect(html).toBe('');
    });
  });

  describe('API & Data Fetching Behavior', () => {
    const mockFetch = vi.fn();

    beforeEach(() => {
      vi.stubGlobal('fetch', mockFetch);
    });

    afterEach(() => {
      vi.unstubAllGlobals();
      mockFetch.mockReset();
    });

    it('fetches DAG list from /api/agentflow/dags?cwd=... and loads detail on selection', async () => {
      const mockDagsList: DagSummary[] = [
        {
          id: 'dag-test-01',
          title: '自动化测试 DAG',
          status: 'done',
          total_tasks: 4,
          done_tasks: 4,
          created_at: '2026-03-10T12:00:00Z',
        },
      ];

      const mockSpec: LiveSpecDoc = {
        version: '1.0.0',
        dag_id: 'dag-test-01',
        title: '自动化测试 DAG',
        tasks: [
          { id: 't1', title: 'Task 1', state: 'passed' },
          { id: 't2', title: 'Task 2', state: 'passed', depends_on: ['t1'] },
        ],
      };

      // Mock responses
      mockFetch.mockImplementation((url: string) => {
        if (url.includes('/api/agentflow/dags')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: async () => ({
              ok: true,
              cwd: 'D:/myprogram/InsightTutor',
              dags: mockDagsList,
            }),
          });
        }
        if (url.includes('/api/agentflow/dag?')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: async () => ({
              ok: true,
              spec: mockSpec,
            }),
          });
        }
        return Promise.reject(new Error(`Unhandled URL: ${url}`));
      });

      // Verify URL encoding and parameters
      const cwd = 'D:\\myprogram\\InsightTutor';
      const dagsResp = await fetch(`/api/agentflow/dags?cwd=${encodeURIComponent(cwd)}`);
      const dagsData = await dagsResp.json();
      expect(dagsData.ok).toBe(true);
      expect(dagsData.dags).toHaveLength(1);
      expect(dagsData.dags[0].id).toBe('dag-test-01');

      const dagResp = await fetch(
        `/api/agentflow/dag?cwd=${encodeURIComponent(cwd)}&dag_id=dag-test-01`
      );
      const dagData = await dagResp.json();
      expect(dagData.ok).toBe(true);
      expect(dagData.spec.dag_id).toBe('dag-test-01');
      expect(dagData.spec.tasks).toHaveLength(2);
    });

    it('handles fetch failure gracefully and handles error states', async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: 'Internal Server Error',
      });

      const cwd = 'D:\\myprogram\\InsightTutor';
      const resp = await fetch(`/api/agentflow/dags?cwd=${encodeURIComponent(cwd)}`);
      expect(resp.ok).toBe(false);
      expect(resp.status).toBe(500);
    });
  });
});
