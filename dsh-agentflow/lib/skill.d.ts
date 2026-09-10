/** Outcome of one skill synchronization pass. */
export type SyncOutcome = 'copied' | 'skipped';
/**
 * Compare two `vX.Y.Z` version strings: -1 / 0 / 1. A string that does not
 * parse as a semver triple counts as older than any parseable one.
 */
export declare function compareVersions(a: string, b: string): number;
/** Read a directory's `VERSION` file, or null when absent/unreadable/empty. */
export declare function readVersion(dir: string): Promise<string | null>;
/**
 * Idempotently mirror a skill bundle into the target directory.
 * Skips only when both versions parse and the target is >= the source;
 * any unparseable side falls through to a copy (never a silent skip).
 */
export declare function syncSkill(sourceDir: string, targetDir: string): Promise<SyncOutcome>;
