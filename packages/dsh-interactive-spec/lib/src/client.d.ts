import React from 'react';
import type { LiveSpecHostCardProps } from './types.js';
export interface DshClientContext {
    slots: {
        inject: (name: string, callback: () => (() => void) | void) => () => void;
        register: (descriptor: {
            name: string;
            key: string;
            [key: string]: unknown;
        }, component: unknown) => () => void;
    };
    sidebarRightTabs?: {
        register: (definition: Record<string, unknown>) => () => void;
    };
    effect?: (callback: () => void | (() => void), name?: string) => void;
    [key: string]: unknown;
}
export declare const LIVE_SPEC_TAB_KEY = "live-spec";
export declare const DEFAULT_CANVAS_URL = "/agentflow/canvas/index.html";
/**
 * Live-Spec Tab Title Component registered in DSH right sidebar.
 */
export declare function LiveSpecPaneTitle(): React.ReactElement;
/**
 * Host Card Container with Embedded Iframe and Fullscreen Modal Toggle
 */
export declare function LiveSpecHostCard({ canvasUrl, initialSpec, initialMeta, sessionContext: propSessionContext, sessionId: directSessionId, cwd: directCwd, useSessions, readOnly, theme, onApply, onFeedbackIntent, className, }: LiveSpecHostCardProps): React.ReactElement;
/**
 * Live-Spec Pane Body Component registered to DSH slot: sidebar.right.pane.tab
 */
export declare function LiveSpecPaneBody(props: any): React.ReactElement;
export declare const name = "dsh-interactive-spec";
export declare const inject: string[];
/**
 * Client plugin entry point for DeepSeek Harness (Cordis runner).
 */
export declare function apply(ctx: DshClientContext): void;
