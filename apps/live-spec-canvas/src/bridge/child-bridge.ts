import { LiveSpecDoc, SpecDiffResult } from '@agentflow/live-spec-core';

export type ThemeMode = 'light' | 'dark';

export interface BridgeSpecPatchPayload {
  spec?: LiveSpecDoc;
  patch?: Partial<LiveSpecDoc>;
  tasks?: LiveSpecDoc['tasks'];
  [key: string]: unknown;
}

export type MountCallback = (spec: LiveSpecDoc) => void;
export type PatchCallback = (payload: BridgeSpecPatchPayload) => void;
export type ThemeCallback = (theme: ThemeMode) => void;

export class ChildBridge {
  private mountListeners: Set<MountCallback> = new Set();
  private patchListeners: Set<PatchCallback> = new Set();
  private themeListeners: Set<ThemeCallback> = new Set();
  private isListening = false;

  constructor() {
    this.init();
  }

  public init(): void {
    if (this.isListening || typeof window === 'undefined') return;
    window.addEventListener('message', this.handleMessage);
    this.isListening = true;
  }

  public destroy(): void {
    if (!this.isListening || typeof window === 'undefined') return;
    window.removeEventListener('message', this.handleMessage);
    this.isListening = false;
  }

  private handleMessage = (event: MessageEvent): void => {
    const data = event.data;
    if (!data || typeof data !== 'object' || !data.type) return;

    // SPEC_MOUNT or INIT_DOC
    if (data.type === 'SPEC_MOUNT' || data.type === 'INIT_DOC') {
      const spec: LiveSpecDoc | undefined =
        data.payload?.spec || data.payload?.doc || (Array.isArray(data.payload?.tasks) ? data.payload : undefined);
      if (spec && Array.isArray(spec.tasks)) {
        this.mountListeners.forEach((cb) => cb(spec));
      }
    }
    // SPEC_PATCH or SYNC_SPEC
    else if (data.type === 'SPEC_PATCH' || data.type === 'SYNC_SPEC') {
      this.patchListeners.forEach((cb) => cb(data.payload || {}));
    }
    // THEME_CHANGE
    else if (data.type === 'THEME_CHANGE') {
      const theme: ThemeMode = data.payload?.theme === 'light' ? 'light' : 'dark';
      this.themeListeners.forEach((cb) => cb(theme));
    }
  };

  public onMount(cb: MountCallback): () => void {
    this.mountListeners.add(cb);
    return () => this.mountListeners.delete(cb);
  }

  public onPatch(cb: PatchCallback): () => void {
    this.patchListeners.add(cb);
    return () => this.patchListeners.delete(cb);
  }

  public onTheme(cb: ThemeCallback): () => void {
    this.themeListeners.add(cb);
    return () => this.themeListeners.delete(cb);
  }

  public send(event: { type: string; payload?: unknown }): void {
    if (typeof window !== 'undefined' && window.parent && window.parent !== window) {
      window.parent.postMessage(event, '*');
    }
  }

  public signalReady(): void {
    this.send({ type: 'CANVAS_READY' });
  }

  public applyToProject(spec: LiveSpecDoc, diff?: SpecDiffResult): void {
    this.send({
      type: 'SPEC_APPLY',
      payload: { spec, diff },
    });
  }

  public feedbackToChat(diff: SpecDiffResult, spec: LiveSpecDoc, prompt?: string): void {
    this.send({
      type: 'SPEC_FEEDBACK_INTENT',
      payload: { diff, spec, prompt: prompt || diff.summary },
    });
  }
}

export const childBridge = new ChildBridge();
