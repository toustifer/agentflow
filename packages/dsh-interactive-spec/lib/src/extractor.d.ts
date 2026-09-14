import type { LiveSpecDoc } from '@agentflow/live-spec-core';
import type { ExtractOptions } from './types.js';
/**
 * Tolerant JSON parse that cleans comments and trailing commas if raw parse fails.
 */
export declare function tolerantJsonParse(raw: string): unknown | null;
/**
 * Validates and normalizes raw parsed object into a clean LiveSpecDoc.
 */
export declare function normalizeLiveSpec(rawObj: unknown): LiveSpecDoc | null;
/**
 * Extracts all valid LiveSpecDoc blocks from Markdown content.
 */
export declare function extractAllSpecsFromMarkdown(markdown: string): LiveSpecDoc[];
/**
 * Extracts a LiveSpecDoc from Markdown content.
 * Defaults to the last (latest revision) valid spec block if multiple exist.
 */
export declare function extractSpecFromMarkdown(markdown: string, options?: ExtractOptions): LiveSpecDoc | null;
