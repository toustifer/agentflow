export * from './types';
export * from './extractor';

export const name = 'dsh-interactive-spec';

/**
 * Node/Cordis host-side plugin entry point.
 */
export function apply(_ctx?: unknown): void {
  // Host-side companion lifecycle hooks or commands can be registered here
}

export default {
  name,
  apply,
};
