import React, { useState, useEffect, useRef, useCallback } from 'react';
import { normalizeLiveSpec, extractSpecFromMarkdown } from './extractor.js';
export const LIVE_SPEC_TAB_KEY = 'live-spec';
export const DEFAULT_CANVAS_URL = '/agentflow/canvas/index.html';
/**
 * Live-Spec Tab Title Component registered in DSH right sidebar.
 */
export function LiveSpecPaneTitle() {
    return React.createElement('div', {
        style: {
            display: 'inline-flex',
            alignItems: 'center',
            gap: '6px',
            fontSize: '13px',
            fontWeight: 500,
            userSelect: 'none',
        },
        title: 'Agentflow Live-Spec Interactive Canvas',
    }, React.createElement('svg', {
        width: 14,
        height: 14,
        viewBox: '0 0 24 24',
        fill: 'none',
        stroke: 'currentColor',
        strokeWidth: 2,
        strokeLinecap: 'round',
        strokeLinejoin: 'round',
    }, React.createElement('circle', { cx: 6, cy: 6, r: 3 }), React.createElement('circle', { cx: 6, cy: 18, r: 3 }), React.createElement('circle', { cx: 18, cy: 12, r: 3 }), React.createElement('path', { d: 'M9 6h4a2 2 0 0 1 2 2v4m0 0a2 2 0 0 1-2 2H9' })), React.createElement('span', null, 'Live Spec'));
}
/**
 * Host Card Container with Embedded Iframe and Fullscreen Modal Toggle
 */
export function LiveSpecHostCard({ canvasUrl = DEFAULT_CANVAS_URL, initialSpec = null, initialMeta = null, sessionContext: propSessionContext = null, sessionId: directSessionId, cwd: directCwd, useSessions, readOnly = false, theme = 'dark', onApply, onFeedbackIntent, className, }) {
    const [isFullscreen, setIsFullscreen] = useState(false);
    const [currentSpec, setCurrentSpec] = useState(initialSpec);
    const [isReady, setIsReady] = useState(false);
    const [lastNotification, setLastNotification] = useState(null);
    // Resolve session context safely
    const resolvedSessionId = initialMeta?.sessionId ?? propSessionContext?.sessionId ?? directSessionId;
    const resolvedCwd = typeof useSessions === 'function' && resolvedSessionId
        ? useSessions((sessions) => sessions?.byId?.[resolvedSessionId]?.cwd)
        : (initialMeta?.cwd ?? propSessionContext?.cwd ?? directCwd);
    const sessionContext = {
        sessionId: resolvedSessionId,
        cwd: resolvedCwd,
        ...initialMeta,
        ...propSessionContext,
    };
    const sessionId = sessionContext.sessionId;
    const cwd = sessionContext.cwd;
    const iframeRef = useRef(null);
    // Helper to post message to iframe safely
    const postToCanvas = useCallback((message) => {
        if (iframeRef.current?.contentWindow) {
            iframeRef.current.contentWindow.postMessage(message, '*');
        }
    }, []);
    // Sync initialSpec when prop changes
    useEffect(() => {
        if (initialSpec) {
            setCurrentSpec(initialSpec);
            if (isReady && iframeRef.current?.contentWindow) {
                const msg = {
                    type: 'SPEC_MOUNT',
                    payload: {
                        spec: initialSpec,
                        readOnly,
                        sessionContext: { sessionId, cwd },
                    },
                };
                iframeRef.current.contentWindow.postMessage(msg, '*');
            }
        }
    }, [initialSpec, isReady, readOnly, sessionId, cwd]);
    // Sync sessionContext updates to canvas iframe when isReady
    useEffect(() => {
        if (isReady && iframeRef.current?.contentWindow) {
            const msg = {
                type: 'SESSION_CONTEXT_CHANGE',
                payload: {
                    sessionContext: { sessionId, cwd },
                },
            };
            iframeRef.current.contentWindow.postMessage(msg, '*');
        }
    }, [sessionId, cwd, isReady]);
    // Fetch or auto-load project DAG status for cwd when no initialSpec is provided
    useEffect(() => {
        if (initialSpec)
            return;
        if (!cwd) {
            setCurrentSpec(null);
            return;
        }
        let isMounted = true;
        const fetchProjectStatus = async () => {
            try {
                const res = await fetch(`/api/agentflow/status?cwd=${encodeURIComponent(cwd)}`);
                if (res.ok) {
                    const data = await res.json();
                    if (isMounted && data) {
                        const doc = data.spec ? normalizeLiveSpec(data.spec) : normalizeLiveSpec(data);
                        if (doc && doc.tasks && doc.tasks.length > 0) {
                            setCurrentSpec(doc);
                        }
                    }
                }
            }
            catch {
                // Fallback gracefully if API not yet up or network unavailable
            }
        };
        fetchProjectStatus();
        return () => {
            isMounted = false;
        };
    }, [cwd, initialSpec]);
    // Listen to broadcast event from DSH conversation / agentflow stream
    useEffect(() => {
        const handleSpecBroadcast = (event) => {
            const customEvent = event;
            if (customEvent.detail) {
                if (customEvent.detail.cwd && cwd && customEvent.detail.cwd !== cwd) {
                    return;
                }
                if (customEvent.detail.spec) {
                    const doc = normalizeLiveSpec(customEvent.detail.spec);
                    if (doc && doc.tasks && doc.tasks.length > 0) {
                        setCurrentSpec(doc);
                    }
                }
                else if (customEvent.detail.markdown) {
                    const doc = extractSpecFromMarkdown(customEvent.detail.markdown);
                    if (doc && doc.tasks && doc.tasks.length > 0) {
                        setCurrentSpec(doc);
                    }
                }
            }
        };
        window.addEventListener('agentflow:spec', handleSpecBroadcast);
        return () => {
            window.removeEventListener('agentflow:spec', handleSpecBroadcast);
        };
    }, [cwd]);
    // Listen to messages from child iframe
    useEffect(() => {
        const handleWindowMessage = (event) => {
            const data = event.data;
            if (!data || typeof data !== 'object' || !data.type)
                return;
            switch (data.type) {
                case 'CANVAS_READY': {
                    setIsReady(true);
                    // Mount spec if available
                    if (currentSpec) {
                        postToCanvas({
                            type: 'SPEC_MOUNT',
                            payload: {
                                spec: currentSpec,
                                readOnly,
                                sessionContext: { sessionId, cwd },
                            },
                        });
                    }
                    else {
                        postToCanvas({
                            type: 'SESSION_CONTEXT_CHANGE',
                            payload: {
                                sessionContext: { sessionId, cwd },
                            },
                        });
                    }
                    // Send theme
                    postToCanvas({
                        type: 'THEME_CHANGE',
                        payload: { theme },
                    });
                    break;
                }
                case 'SPEC_APPLY': {
                    const { spec, diff } = data.payload;
                    setCurrentSpec(spec);
                    setLastNotification(`Spec applied: ${diff?.summary || `${spec.tasks.length} tasks`}`);
                    onApply?.({ spec, diff });
                    break;
                }
                case 'SPEC_FEEDBACK_INTENT': {
                    const { diff, spec, prompt } = data.payload;
                    setLastNotification('Feedback sent to AI conversation');
                    onFeedbackIntent?.({ diff, spec, prompt });
                    break;
                }
                case 'REQUEST_FULLSCREEN': {
                    setIsFullscreen(Boolean(data.payload.fullscreen));
                    break;
                }
            }
        };
        window.addEventListener('message', handleWindowMessage);
        return () => {
            window.removeEventListener('message', handleWindowMessage);
        };
    }, [currentSpec, onApply, onFeedbackIntent, postToCanvas, readOnly, sessionId, cwd, theme]);
    const toggleFullscreen = useCallback(() => {
        setIsFullscreen((prev) => !prev);
    }, []);
    const containerStyle = isFullscreen
        ? {
            position: 'fixed',
            inset: 0,
            zIndex: 99999,
            backgroundColor: theme === 'light' ? '#f8fafc' : '#0f172a',
            display: 'flex',
            flexDirection: 'column',
            width: '100vw',
            height: '100vh',
            overflow: 'hidden',
        }
        : {
            position: 'relative',
            display: 'flex',
            flexDirection: 'column',
            width: '100%',
            height: '100%',
            minHeight: '480px',
            backgroundColor: theme === 'light' ? '#ffffff' : '#090d16',
            borderRadius: '6px',
            overflow: 'hidden',
        };
    const headerStyle = {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '8px 12px',
        backgroundColor: theme === 'light' ? '#f1f5f9' : '#1e293b',
        borderBottom: theme === 'light' ? '1px solid #e2e8f0' : '1px solid #334155',
        color: theme === 'light' ? '#0f172a' : '#f8fafc',
        fontSize: '12px',
        lineHeight: 1.4,
    };
    const buttonStyle = {
        display: 'inline-flex',
        alignItems: 'center',
        gap: '4px',
        padding: '4px 8px',
        fontSize: '12px',
        fontWeight: 500,
        borderRadius: '4px',
        border: theme === 'light' ? '1px solid #cbd5e1' : '1px solid #475569',
        backgroundColor: theme === 'light' ? '#ffffff' : '#334155',
        color: theme === 'light' ? '#1e293b' : '#f8fafc',
        cursor: 'pointer',
        transition: 'background-color 0.15s ease',
    };
    const hasValidDag = Boolean(currentSpec && Array.isArray(currentSpec.tasks) && currentSpec.tasks.length > 0);
    return React.createElement('div', {
        className,
        style: containerStyle,
        'data-testid': 'live-spec-host-container',
    }, 
    // Header
    React.createElement('div', { style: headerStyle }, React.createElement('div', { style: { display: 'flex', alignItems: 'center', gap: '8px' } }, React.createElement('span', { style: { fontWeight: 600 } }, currentSpec?.title || (cwd ? `工作区: ${cwd}` : 'Agentflow Live-Spec Canvas')), currentSpec?.dag_id
        ? React.createElement('span', {
            style: {
                fontSize: '11px',
                padding: '2px 6px',
                borderRadius: '4px',
                backgroundColor: theme === 'light' ? '#e2e8f0' : '#334155',
                color: theme === 'light' ? '#475569' : '#94a3b8',
            },
        }, currentSpec.dag_id)
        : cwd
            ? React.createElement('span', {
                'data-testid': 'header-cwd-badge',
                style: {
                    fontSize: '11px',
                    padding: '2px 6px',
                    borderRadius: '4px',
                    backgroundColor: theme === 'light' ? '#e2e8f0' : '#334155',
                    color: theme === 'light' ? '#475569' : '#94a3b8',
                    maxWidth: '240px',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                },
                title: cwd,
            }, cwd)
            : null, lastNotification
        ? React.createElement('span', {
            style: {
                fontSize: '11px',
                color: '#10b981',
                marginLeft: '8px',
            },
        }, `✓ ${lastNotification}`)
        : null), React.createElement('div', { style: { display: 'flex', alignItems: 'center', gap: '8px' } }, React.createElement('button', {
        type: 'button',
        onClick: toggleFullscreen,
        style: buttonStyle,
        title: isFullscreen ? '收起全屏 (Esc)' : '展开全屏模式',
        'data-testid': 'fullscreen-toggle-btn',
    }, isFullscreen ? '⤡ 收起' : '⤢ 展开'))), 
    // Empty state placeholder when no valid DAG is found
    !hasValidDag
        ? React.createElement('div', {
            className: 'live-spec-empty-state',
            'data-testid': 'live-spec-empty-state',
            style: {
                flex: 1,
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                padding: '32px 16px',
                textAlign: 'center',
                color: theme === 'light' ? '#475569' : '#94a3b8',
                backgroundColor: theme === 'light' ? '#f8fafc' : '#0b0f19',
            },
        }, React.createElement('svg', {
            width: 48,
            height: 48,
            viewBox: '0 0 24 24',
            fill: 'none',
            stroke: 'currentColor',
            strokeWidth: 1.5,
            strokeLinecap: 'round',
            strokeLinejoin: 'round',
            style: { marginBottom: '16px', opacity: 0.7 },
        }, React.createElement('rect', { x: 3, y: 3, width: 18, height: 18, rx: 2 }), React.createElement('path', { d: 'M9 9h6' }), React.createElement('path', { d: 'M9 13h6' }), React.createElement('path', { d: 'M9 17h4' })), React.createElement('div', {
            style: {
                fontSize: '15px',
                fontWeight: 600,
                marginBottom: '8px',
                color: theme === 'light' ? '#0f172a' : '#f8fafc',
            },
        }, '暂无活动 DAG 编排'), React.createElement('div', {
            'data-testid': 'live-spec-current-cwd',
            style: {
                fontSize: '13px',
                fontFamily: 'monospace',
                padding: '6px 12px',
                borderRadius: '4px',
                backgroundColor: theme === 'light' ? '#e2e8f0' : '#1e293b',
                color: theme === 'light' ? '#1e293b' : '#38bdf8',
                maxWidth: '90%',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                marginBottom: '12px',
            },
        }, `当前工作区: ${cwd || '未绑定工作区'}`), React.createElement('div', {
            style: {
                fontSize: '12px',
                maxWidth: '380px',
                lineHeight: 1.5,
                color: theme === 'light' ? '#64748b' : '#64748b',
            },
        }, '当前会话尚未检测到活跃的 Agentflow 任务 DAG。请在对话中让 Leader 启动流程或编排任务。'))
        : null, 
    // Embedded Iframe
    React.createElement('iframe', {
        ref: iframeRef,
        src: canvasUrl,
        title: 'Agentflow Live Spec Canvas',
        allow: 'clipboard-write; fullscreen',
        style: {
            flex: 1,
            width: '100%',
            height: '100%',
            border: 'none',
            backgroundColor: 'transparent',
            display: hasValidDag ? 'block' : 'none',
        },
    }));
}
/**
 * Live-Spec Pane Body Component registered to DSH slot: sidebar.right.pane.tab
 */
export function LiveSpecPaneBody(props) {
    const sessionId = props?.sessionId ?? props?.initialMeta?.sessionId;
    const useSessions = props?.useSessions;
    const cwd = typeof useSessions === 'function'
        ? useSessions((sessions) => sessions?.byId?.[sessionId]?.cwd)
        : (props?.cwd ?? props?.initialMeta?.cwd);
    const initialMeta = {
        ...(props?.initialMeta || {}),
        sessionId,
        cwd,
    };
    return React.createElement(LiveSpecHostCard, {
        canvasUrl: props?.canvasUrl || DEFAULT_CANVAS_URL,
        initialSpec: props?.spec ?? props?.initialSpec ?? null,
        initialMeta,
        sessionContext: { sessionId, cwd },
        readOnly: props?.readOnly ?? false,
        theme: props?.theme || 'dark',
        onApply: props?.onApply,
        onFeedbackIntent: props?.onFeedbackIntent,
    });
}
export const name = 'dsh-interactive-spec';
export const inject = ['slots', 'sidebarRightTabs'];
/**
 * Client plugin entry point for DeepSeek Harness (Cordis runner).
 */
export function apply(ctx) {
    const registerSlots = () => {
        // Register the tab definition with sidebarRightTabs if service is available
        if (ctx.sidebarRightTabs && typeof ctx.sidebarRightTabs.register === 'function') {
            ctx.sidebarRightTabs.register({
                id: LIVE_SPEC_TAB_KEY,
                kind: LIVE_SPEC_TAB_KEY,
                title: () => 'Live Spec',
                guide: [
                    {
                        order: 15,
                        title: () => 'Live Spec Canvas',
                        description: () => 'Interactive DAG canvas with real-time simulation and CPM analysis',
                    },
                ],
            });
        }
        // Register sidebar.right.pane.tab.title
        const unregisterTitle = ctx.slots.inject('sidebar.right.pane.tab.title', () => ctx.slots.register({
            name: 'sidebar.right.pane.tab.title',
            key: LIVE_SPEC_TAB_KEY,
        }, LiveSpecPaneTitle));
        // Register sidebar.right.pane.tab
        const unregisterBody = ctx.slots.inject('sidebar.right.pane.tab', () => ctx.slots.register({
            name: 'sidebar.right.pane.tab',
            key: LIVE_SPEC_TAB_KEY,
        }, LiveSpecPaneBody));
        return () => {
            unregisterTitle?.();
            unregisterBody?.();
        };
    };
    if (typeof ctx.effect === 'function') {
        ctx.effect(registerSlots, 'dsh-interactive-spec: sidebar slots');
    }
    else {
        registerSlots();
    }
}
