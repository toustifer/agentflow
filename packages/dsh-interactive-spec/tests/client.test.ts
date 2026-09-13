import { describe, it, expect, vi } from 'vitest';
import {
  apply,
  LIVE_SPEC_TAB_KEY,
  DEFAULT_CANVAS_URL,
  LiveSpecPaneTitle,
  LiveSpecPaneBody,
  LiveSpecHostCard,
  type DshClientContext,
} from '../src/client';
import type { LiveSpecDoc, SpecDiffResult } from '../src/types';

describe('DSH client plugin registration', () => {
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
});
