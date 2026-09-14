import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { ChildBridge, SpecSessionContext } from '../apps/live-spec-canvas/src/bridge/child-bridge';
import { LiveSpecDoc } from '@agentflow/live-spec-core';

describe('ChildBridge Session Context and Dynamic Spec Protocol', () => {
  let bridge: ChildBridge;
  let listeners: ((event: MessageEvent) => void)[] = [];

  beforeEach(() => {
    listeners = [];
    vi.stubGlobal('window', {
      addEventListener: vi.fn((event: string, handler: (e: MessageEvent) => void) => {
        if (event === 'message') listeners.push(handler);
      }),
      removeEventListener: vi.fn((event: string, handler: (e: MessageEvent) => void) => {
        if (event === 'message') {
          listeners = listeners.filter((l) => l !== handler);
        }
      }),
      parent: {
        postMessage: vi.fn(),
      },
    });

    bridge = new ChildBridge();
  });

  afterEach(() => {
    bridge.destroy();
    vi.unstubAllGlobals();
  });

  it('triggers onSessionContextChange when SESSION_CONTEXT_CHANGE event is received', () => {
    const callback = vi.fn();
    const unbind = bridge.onSessionContextChange(callback);

    const testContext: SpecSessionContext = {
      sessionId: 'sess-12345',
      cwd: 'D:\\myprogram\\agentflow\\projects\\demo',
    };

    // Simulate receiving SESSION_CONTEXT_CHANGE postMessage
    const event = new MessageEvent('message', {
      data: {
        type: 'SESSION_CONTEXT_CHANGE',
        payload: {
          sessionContext: testContext,
        },
      },
    });

    listeners.forEach((l) => l(event));

    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith(testContext);

    // Test unbind
    unbind();
    listeners.forEach((l) => l(event));
    expect(callback).toHaveBeenCalledTimes(1);
  });

  it('triggers onMount and onSessionContextChange when SPEC_MOUNT with sessionContext is received', () => {
    const mountCb = vi.fn();
    const sessionCb = vi.fn();

    bridge.onMount(mountCb);
    bridge.onSessionContextChange(sessionCb);

    const mockSpec: LiveSpecDoc = {
      version: '1.0.0',
      dag_id: 'mounted-dag',
      title: 'Mounted DAG Test',
      tasks: [
        { id: 't1', title: 'Task 1', state: 'passed' },
        { id: 't2', title: 'Task 2', state: 'executing' },
      ],
    };

    const sessionContext: SpecSessionContext = {
      sessionId: 'sess-abc',
      cwd: 'C:\\Users\\dev\\project',
    };

    const event = new MessageEvent('message', {
      data: {
        type: 'SPEC_MOUNT',
        payload: {
          spec: mockSpec,
          sessionContext,
        },
      },
    });

    listeners.forEach((l) => l(event));

    expect(mountCb).toHaveBeenCalledTimes(1);
    expect(mountCb).toHaveBeenCalledWith(mockSpec);

    expect(sessionCb).toHaveBeenCalledTimes(1);
    expect(sessionCb).toHaveBeenCalledWith(sessionContext);
  });

  it('supports empty tasks SPEC_MOUNT and triggers both callbacks cleanly', () => {
    const mountCb = vi.fn();
    const sessionCb = vi.fn();

    bridge.onMount(mountCb);
    bridge.onSessionContextChange(sessionCb);

    const emptySpec: LiveSpecDoc = {
      version: '1.0.0',
      dag_id: 'empty-dag',
      tasks: [],
    };

    const event = new MessageEvent('message', {
      data: {
        type: 'SPEC_MOUNT',
        payload: {
          spec: emptySpec,
          sessionContext: { cwd: '/workspace/empty-project' },
        },
      },
    });

    listeners.forEach((l) => l(event));

    expect(mountCb).toHaveBeenCalledWith(emptySpec);
    expect(sessionCb).toHaveBeenCalledWith({ cwd: '/workspace/empty-project' });
  });

  it('triggers onSessionContextChange on SPEC_PATCH when payload contains sessionContext', () => {
    const patchCb = vi.fn();
    const sessionCb = vi.fn();

    bridge.onPatch(patchCb);
    bridge.onSessionContextChange(sessionCb);

    const updatedContext: SpecSessionContext = {
      sessionId: 'sess-patched',
      cwd: '/new/workspace/path',
    };

    const event = new MessageEvent('message', {
      data: {
        type: 'SPEC_PATCH',
        payload: {
          patch: { concurrency: 5 },
          sessionContext: updatedContext,
        },
      },
    });

    listeners.forEach((l) => l(event));

    expect(patchCb).toHaveBeenCalledTimes(1);
    expect(sessionCb).toHaveBeenCalledTimes(1);
    expect(sessionCb).toHaveBeenCalledWith(updatedContext);
  });
});
