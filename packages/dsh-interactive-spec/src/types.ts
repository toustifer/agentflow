import type { LiveSpecDoc, SpecDiffResult, SpecTask } from '@agentflow/live-spec-core';

export type { LiveSpecDoc, SpecDiffResult, SpecTask };

export type ThemeMode = 'light' | 'dark';

/**
 * Messages sent from Host (DSH plugin) to Child (Live-Spec Canvas iframe)
 */
export type HostToCanvasMessage =
  | {
      type: 'SPEC_MOUNT' | 'INIT_DOC';
      payload: {
        spec?: LiveSpecDoc;
        doc?: LiveSpecDoc;
        readOnly?: boolean;
      };
    }
  | {
      type: 'SPEC_PATCH' | 'SYNC_SPEC';
      payload: {
        spec?: LiveSpecDoc;
        patch?: Partial<LiveSpecDoc>;
        tasks?: SpecTask[];
        [key: string]: unknown;
      };
    }
  | {
      type: 'THEME_CHANGE';
      payload: {
        theme: ThemeMode;
      };
    };

/**
 * Messages sent from Child (Live-Spec Canvas iframe) to Host (DSH plugin)
 */
export type CanvasToHostMessage =
  | {
      type: 'CANVAS_READY';
      payload?: Record<string, unknown>;
    }
  | {
      type: 'SPEC_APPLY';
      payload: {
        spec: LiveSpecDoc;
        diff?: SpecDiffResult;
      };
    }
  | {
      type: 'SPEC_FEEDBACK_INTENT';
      payload: {
        diff: SpecDiffResult;
        spec: LiveSpecDoc;
        prompt?: string;
      };
    }
  | {
      type: 'REQUEST_FULLSCREEN';
      payload: {
        fullscreen: boolean;
      };
    };

export interface ExtractOptions {
  /**
   * Which valid spec block to pick when multiple blocks are found.
   * Defaults to 'last' (latest revision).
   */
  pick?: 'first' | 'last';
}

export interface LiveSpecHostCardProps {
  /**
   * Source URL for the canvas iframe.
   * Default: '/agentflow/canvas/index.html' or 'http://localhost:5173'
   */
  canvasUrl?: string;
  /**
   * Initial spec document to mount.
   */
  initialSpec?: LiveSpecDoc | null;
  /**
   * Read-only mode flag.
   */
  readOnly?: boolean;
  /**
   * Visual theme mode ('light' | 'dark').
   */
  theme?: ThemeMode;
  /**
   * Callback invoked when user confirms applying spec changes back to project.
   */
  onApply?: (payload: { spec: LiveSpecDoc; diff?: SpecDiffResult }) => void;
  /**
   * Callback invoked when user sends prompt/feedback intent to chat.
   */
  onFeedbackIntent?: (payload: { diff: SpecDiffResult; spec: LiveSpecDoc; prompt?: string }) => void;
  /**
   * Class name for styling.
   */
  className?: string;
}
