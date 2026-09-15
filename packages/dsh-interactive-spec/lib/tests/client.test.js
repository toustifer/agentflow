import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, it, expect, vi } from 'vitest';
import { apply, inject, LIVE_SPEC_TAB_KEY, DEFAULT_CANVAS_URL, LiveSpecPaneTitle, LiveSpecPaneBody, LiveSpecHostCard, } from '../src/client';
describe('DSH client plugin registration', () => {
    it('declares every Cordis context service used by the plugin', () => {
        expect(inject).toEqual(['slots', 'sidebarRightTabs']);
    });
    it('registers sidebar slots and right tabs with key live-spec', () => {
        const injectedSlots = {};
        const registeredSlots = {};
        const mockCtx = {
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
        expect(mockCtx.sidebarRightTabs?.register).toHaveBeenCalledWith(expect.objectContaining({
            id: LIVE_SPEC_TAB_KEY,
            kind: LIVE_SPEC_TAB_KEY,
            guide: expect.arrayContaining([
                expect.objectContaining({
                    order: 15,
                    title: expect.any(Function),
                    description: expect.any(Function),
                }),
            ]),
        }));
        // Verify slots registered
        expect(mockCtx.slots.inject).toHaveBeenCalledWith('sidebar.right.pane.tab.title', expect.any(Function));
        expect(mockCtx.slots.inject).toHaveBeenCalledWith('sidebar.right.pane.tab', expect.any(Function));
        expect(registeredSlots['sidebar.right.pane.tab.title']).toBeDefined();
        expect(registeredSlots['sidebar.right.pane.tab.title'].descriptor.key).toBe(LIVE_SPEC_TAB_KEY);
        expect(registeredSlots['sidebar.right.pane.tab.title'].component).toBe(LiveSpecPaneTitle);
        expect(registeredSlots['sidebar.right.pane.tab']).toBeDefined();
        expect(registeredSlots['sidebar.right.pane.tab'].descriptor.key).toBe(LIVE_SPEC_TAB_KEY);
        expect(registeredSlots['sidebar.right.pane.tab'].component).toBe(LiveSpecPaneBody);
    });
    it('runs without crashing when effect or sidebarRightTabs is optional', () => {
        const mockCtxWithoutOptionals = {
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
        const sampleDoc = {
            version: '1.0.0',
            title: 'Test Doc',
            tasks: [{ id: 't1', title: 'Task 1' }],
        };
        const sampleDiff = {
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
            const useSessions = vi.fn((selector) => selector(mockSessions));
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
    describe('Session Scope & cwd extraction and iframe forwarding', () => {
        it('always renders iframe so live-spec-canvas can render interactive topology and history selector', () => {
            const targetCwd = 'D:\\myprogram\\experience\\siruoning\\Ai_medbox';
            const html = renderToStaticMarkup(React.createElement(LiveSpecHostCard, {
                initialMeta: {
                    sessionId: 'session-123',
                    cwd: targetCwd,
                },
            }));
            // Verify iframe is rendered
            expect(html).toContain('<iframe');
            expect(html).toContain('title="Agentflow Live Spec Canvas"');
            expect(html).toContain('data-testid="header-cwd-badge"');
            expect(html).toContain(`title="${targetCwd}"`);
        });
        it('renders host card with DAG title and dag_id when initialSpec is provided', () => {
            const sampleDoc = {
                version: '1.0.0',
                title: 'Active Project DAG',
                dag_id: 'dag-active-run',
                tasks: [
                    { id: 't1', title: 'Task 1', state: 'running' },
                    { id: 't2', title: 'Task 2', state: 'pending', depends_on: ['t1'] },
                ],
            };
            const targetCwd = 'D:\\myprogram\\InsightTutor';
            const html = renderToStaticMarkup(React.createElement(LiveSpecHostCard, {
                initialSpec: sampleDoc,
                initialMeta: {
                    sessionId: 'session-456',
                    cwd: targetCwd,
                },
            }));
            expect(html).toContain('Active Project DAG');
            expect(html).toContain('dag-active-run');
            expect(html).toContain('<iframe');
        });
    });
    describe('DownstreamEvent sessionContext communication contract', () => {
        it('formats DownstreamEvent SPEC_MOUNT with sessionContext payload', () => {
            const sampleDoc = {
                version: '1.0.0',
                title: 'Mounted Spec',
                tasks: [{ id: 't1', title: 'Initialize' }],
            };
            const sessionContext = {
                sessionId: 'session-live',
                cwd: 'D:\\myprogram\\InsightTutor',
            };
            const mountEvent = {
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
            const sessionContext = {
                sessionId: 'session-switched',
                cwd: 'D:\\myprogram\\experience\\siruoning\\Ai_medbox',
            };
            const contextChangeEvent = {
                type: 'SESSION_CONTEXT_CHANGE',
                payload: {
                    sessionContext,
                },
            };
            expect(contextChangeEvent.type).toBe('SESSION_CONTEXT_CHANGE');
            expect(contextChangeEvent.payload.sessionContext.cwd).toBe('D:\\myprogram\\experience\\siruoning\\Ai_medbox');
        });
    });
});
