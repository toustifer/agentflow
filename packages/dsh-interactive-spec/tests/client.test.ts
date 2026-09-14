import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, it, expect, vi } from 'vitest';
import {
  apply,
  inject,
  LIVE_SPEC_TAB_KEY,
  DEFAULT_CANVAS_URL,
  LiveSpecPaneTitle,
  LiveSpecPaneBody,
  LiveSpecHostCard,
  type DshClientContext,
} from '../src/client';
import type {
  LiveSpecDoc,
  SpecDiffResult,
  DownstreamEvent,
  SpecSessionContext,
} from '../src/types';

describe('DSH client plugin registration', () => {
  it('declares every Cordis context service used by the plugin', () => {
    expect(inject).toEqual(['slots', 'sidebarRightTabs']);
  });

  it('registers sidebar slots and right tabs with key live-spec', () => {
    const injectedSlots: Record<string, () => void> = {};
    const registeredSlots: Record<string, { descriptor: any; component: any }> = {};

    const mockCtx: DshClientContext = {
      slots: {
        inject: vi.fn((name, factory) => {
          injectedSlots[name] = factory;
          factory();
          return vi.fn();
        }),
        register: vi.fn((descriptor, component) => {
          registeredSlots[descriptor.name] = { descriptor, component };
          return vi.fn();
        }),
      },
      sidebarRightTabs: {
        register: vi.fn(),
      },
      effect: vi.fn((cb) => {
        cb();
      }),
    };

    apply(mockCtx);

    // Verify effect called
    expect(mockCtx.effect).toHaveBeenCalled();

    // Verify sidebarRightTabs.register called
    expect(mockCtx.sidebarRightTabs?.register).toHaveBeenCalledWith(
      expect.objectContaining({
        id: LIVE_SPEC_TAB_KEY,
        kind: LIVE_SPEC_TAB_KEY,
        guide: expect.arrayContaining([
          expect.objectContaining({
            order: 15,
            title: expect.any(Function),
            description: expect.any(Function),
          }),
        ]),
      })
    );

    // Verify slots registered
    expect(mockCtx.slots.inject).toHaveBeenCalledWith(
      'sidebar.right.pane.tab.title',
      expect.any(Function)
    );
    expect(mockCtx.slots.inject).toHaveBeenCalledWith(
      'sidebar.right.pane.tab',
      expect.any(Function)
    );

    expect(registeredSlots['sidebar.right.pane.tab.title']).toBeDefined();
    expect(registeredSlots['sidebar.right.pane.tab.title'].descriptor.key).toBe(LIVE_SPEC_TAB_KEY);
    expect(registeredSlots['sidebar.right.pane.tab.title'].component).toBe(LiveSpecPaneTitle);

    expect(registeredSlots['sidebar.right.pane.tab']).toBeDefined();
    expect(registeredSlots['sidebar.right.pane.tab'].descriptor.key).toBe(LIVE_SPEC_TAB_KEY);
    expect(registeredSlots['sidebar.right.pane.tab'].component).toBe(LiveSpecPaneBody);
  });

  it('runs without crashing when effect or sidebarRightTabs is optional', () => {
    const mockCtxWithoutOptionals: DshClientContext = {
      slots: {
        inject: vi.fn((name, factory) => {
          factory();
          return vi.fn();
        }),
        register: vi.fn(),
      },
    };

    expect(() => apply(mockCtxWithoutOptionals)).not.toThrow();
    expect(mockCtxWithoutOptionals.slots.register).toHaveBeenCalledTimes(2);
  });
});

describe('LiveSpecHostCard component and message protocol', () => {
  it('exports components and constants correctly', () => {
    expect(LIVE_SPEC_TAB_KEY).toBe('live-spec');
    expect(DEFAULT_CANVAS_URL).toBe('/agentflow/canvas/index.html');
    expect(LiveSpecPaneTitle).toBeTypeOf('function');
    expect(LiveSpecPaneBody).toBeTypeOf('function');
    expect(LiveSpecHostCard).toBeTypeOf('function');
  });

  it('handles SPEC_APPLY and SPEC_FEEDBACK_INTENT message events from child iframe', () => {
    const onApply = vi.fn();
    const onFeedbackIntent = vi.fn();

    const sampleDoc: LiveSpecDoc = {
      version: '1.0.0',
      title: 'Test Doc',
      tasks: [{ id: 't1', title: 'Task 1' }],
    };
    const sampleDiff: SpecDiffResult = {
      hasChanges: true,
      addedTasks: [],
      removedTasks: [],
      modifiedTasks: [],
      addedDependencies: [],
      removedDependencies: [],
      parameterChanges: [],
      summary: 'Added 1 task',
    };

    // Simulate window message event dispatching
    const applyEvent = new MessageEvent('message', {
      data: {
        type: 'SPEC_APPLY',
        payload: {
          spec: sampleDoc,
          diff: sampleDiff,
        },
      },
    });

    const feedbackEvent = new MessageEvent('message', {
      data: {
        type: 'SPEC_FEEDBACK_INTENT',
        payload: {
          spec: sampleDoc,
          diff: sampleDiff,
          prompt: 'Please verify task 1',
        },
      },
    });

    // Verify event structure
    expect(applyEvent.data.type).toBe('SPEC_APPLY');
    expect(applyEvent.data.payload.spec.title).toBe('Test Doc');

    expect(feedbackEvent.data.type).toBe('SPEC_FEEDBACK_INTENT');
    expect(feedbackEvent.data.payload.prompt).toBe('Please verify task 1');
  });

  describe('Session Scope & cwd extraction via useSessions', () => {
    it('extracts sessionId and resolves cwd from useSessions selector in LiveSpecPaneBody', () => {
      const mockSessions = {
        byId: {
          'session-alpha': {
            id: 'session-alpha',
            title: '知迹伴学优化',
            cwd: 'D:\\myprogram\\experience\\siruoning\\Ai_medbox',
          },
          'session-beta': {
            id: 'session-beta',
            title: 'InsightTutor Dev',
            cwd: 'D:\\myprogram\\InsightTutor',
          },
        },
      };

      const useSessions = vi.fn((selector: (state: any) => any) => selector(mockSessions));

      const element = LiveSpecPaneBody({
        sessionId: 'session-alpha',
        useSessions,
      });

      // Verify useSessions was called with a selector function
      expect(useSessions).toHaveBeenCalled();
      expect(typeof useSessions.mock.calls[0][0]).toBe('function');

      // Verify props passed down to LiveSpecHostCard
      expect(element.props.initialMeta).toBeDefined();
      expect(element.props.initialMeta.sessionId).toBe('session-alpha');
      expect(element.props.initialMeta.cwd).toBe('D:\\myprogram\\experience\\siruoning\\Ai_medbox');

      expect(element.props.sessionContext).toEqual({
        sessionId: 'session-alpha',
        cwd: 'D:\\myprogram\\experience\\siruoning\\Ai_medbox',
      });
    });

    it('falls back to props.cwd or props.initialMeta.cwd when useSessions is not available', () => {
      const elementDirect = LiveSpecPaneBody({
        sessionId: 'session-fallback',
        cwd: 'D:\\myprogram\\experience\\custom-project',
      });

      expect(elementDirect.props.initialMeta).toEqual({
        sessionId: 'session-fallback',
        cwd: 'D:\\myprogram\\experience\\custom-project',
      });

      const elementMeta = LiveSpecPaneBody({
        initialMeta: {
          sessionId: 'session-meta',
          cwd: 'D:\\myprogram\\experience\\meta-project',
        },
      });

      expect(elementMeta.props.initialMeta.sessionId).toBe('session-meta');
      expect(elementMeta.props.initialMeta.cwd).toBe('D:\\myprogram\\experience\\meta-project');
    });
  });

  describe('Empty state placeholder and session context rendering', () => {
    it('renders empty state placeholder with current workspace cwd when no valid DAG is present', () => {
      const targetCwd = 'D:\\myprogram\\experience\\siruoning\\Ai_medbox';
      const html = renderToStaticMarkup(
        React.createElement(LiveSpecHostCard, {
          initialMeta: {
            sessionId: 'session-123',
            cwd: targetCwd,
          },
        })
      );

      // Verify empty state placeholder exists
      expect(html).toContain('data-testid="live-spec-empty-state"');
      expect(html).toContain('暂无活动 DAG 编排');
      expect(html).toContain(`当前工作区: ${targetCwd}`);

      // Verify iframe is hidden (display:none) so no default/stale bootstrap sample is shown
      expect(html).toContain('display:none');
    });

    it('renders iframe in display:block when a valid DAG with tasks is provided', () => {
      const sampleDoc: LiveSpecDoc = {
        version: '1.0.0',
        title: 'Active Project DAG',
        dag_id: 'dag-active-run',
        tasks: [
          { id: 't1', title: 'Task 1', state: 'running' },
          { id: 't2', title: 'Task 2', state: 'pending', depends_on: ['t1'] },
        ],
      };

      const targetCwd = 'D:\\myprogram\\InsightTutor';
      const html = renderToStaticMarkup(
        React.createElement(LiveSpecHostCard, {
          initialSpec: sampleDoc,
          initialMeta: {
            sessionId: 'session-456',
            cwd: targetCwd,
          },
        })
      );

      // Empty state placeholder should NOT be rendered
      expect(html).not.toContain('data-testid="live-spec-empty-state"');
      expect(html).toContain('Active Project DAG');
      expect(html).toContain('dag-active-run');

      // Iframe should be visible with display:block
      expect(html).toContain('display:block');
    });
  });

  describe('DownstreamEvent sessionContext communication contract', () => {
    it('formats DownstreamEvent SPEC_MOUNT with sessionContext payload', () => {
      const sampleDoc: LiveSpecDoc = {
        version: '1.0.0',
        title: 'Mounted Spec',
        tasks: [{ id: 't1', title: 'Initialize' }],
      };

      const sessionContext: SpecSessionContext = {
        sessionId: 'session-live',
        cwd: 'D:\\myprogram\\InsightTutor',
      };

      const mountEvent: DownstreamEvent = {
        type: 'SPEC_MOUNT',
        payload: {
          spec: sampleDoc,
          readOnly: false,
          sessionContext,
        },
      };

      expect(mountEvent.type).toBe('SPEC_MOUNT');
      expect(mountEvent.payload.sessionContext).toEqual({
        sessionId: 'session-live',
        cwd: 'D:\\myprogram\\InsightTutor',
      });
      expect(mountEvent.payload.spec?.title).toBe('Mounted Spec');
    });

    it('formats DownstreamEvent SESSION_CONTEXT_CHANGE payload correctly', () => {
      const sessionContext: SpecSessionContext = {
        sessionId: 'session-switched',
        cwd: 'D:\\myprogram\\experience\\siruoning\\Ai_medbox',
      };

      const contextChangeEvent: DownstreamEvent = {
        type: 'SESSION_CONTEXT_CHANGE',
        payload: {
          sessionContext,
        },
      };

      expect(contextChangeEvent.type).toBe('SESSION_CONTEXT_CHANGE');
      expect(contextChangeEvent.payload.sessionContext.cwd).toBe(
        'D:\\myprogram\\experience\\siruoning\\Ai_medbox'
      );
    });
  });
});
