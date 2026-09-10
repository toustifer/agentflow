import { cp, readFile } from 'node:fs/promises';
import { join } from 'node:path';
/**
 * Compare two `vX.Y.Z` version strings: -1 / 0 / 1. A string that does not
 * parse as a semver triple counts as older than any parseable one.
 */
export function compareVersions(a, b) {
    const pa = parseVersion(a);
    const pb = parseVersion(b);
    if (pa === null && pb === null)
        return 0;
    if (pa === null)
        return -1;
    if (pb === null)
        return 1;
    for (let i = 0; i < 3; i++) {
        if (pa[i] !== pb[i])
            return pa[i] < pb[i] ? -1 : 1;
    }
    return 0;
}
function parseVersion(raw) {
    const match = /^v?(\d+)\.(\d+)\.(\d+)/.exec(raw.trim());
    if (match === null)
        return null;
    return [Number(match[1]), Number(match[2]), Number(match[3])];
}
/** Read a directory's `VERSION` file, or null when absent/unreadable/empty. */
export async function readVersion(dir) {
    try {
        const content = (await readFile(join(dir, 'VERSION'), 'utf8')).trim();
        return content === '' ? null : content;
    }
    catch {
        return null;
    }
}
/**
 * Idempotently mirror a skill bundle into the target directory.
 * Skips only when both versions parse and the target is >= the source;
 * any unparseable side falls through to a copy (never a silent skip).
 */
export async function syncSkill(sourceDir, targetDir) {
    const sourceVersion = await readVersion(sourceDir);
    const targetVersion = await readVersion(targetDir);
    if (sourceVersion !== null && targetVersion !== null && compareVersions(targetVersion, sourceVersion) >= 0) {
        return 'skipped';
    }
    await cp(sourceDir, targetDir, { recursive: true, force: true });
    return 'copied';
}
