export * from './types.js';
export * from './extractor.js';
export * from './history-service.js';
export * from './routes.js';
import type { HistoryServiceOptions } from './history-service';
export declare const name = "dsh-interactive-spec";
export declare const inject: string[];
/**
 * Node/Cordis host-side plugin entry point.
 */
export declare function apply(ctx?: any, options?: HistoryServiceOptions): void;
declare const _default: {
    name: string;
    inject: string[];
    apply: typeof apply;
};
export default _default;
