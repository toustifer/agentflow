import { CycleDetectionResult, LiveSpecDoc, SpecTask } from './types';

/**
 * Normalizes input to an array of SpecTask
 */
function extractTasks(input: LiveSpecDoc | SpecTask[]): SpecTask[] {
  if (Array.isArray(input)) {
    return input;
  }
  return input.tasks || [];
}

/**
 * Normalizes a cycle array so that the lexicographically smallest node comes first,
 * making cycle representations canonical for deduplication.
 * E.g., ['B', 'C', 'A', 'B'] -> ['A', 'B', 'C', 'A']
 */
function canonicalCycle(cycle: string[]): string[] {
  if (cycle.length <= 1) return cycle;
  // The last element is identical to the first
  const nodes = cycle.slice(0, -1);
  let minIdx = 0;
  for (let i = 1; i < nodes.length; i++) {
    if (nodes[i] < nodes[minIdx]) {
      minIdx = i;
    }
  }
  const rotated = [...nodes.slice(minIdx), ...nodes.slice(0, minIdx)];
  rotated.push(rotated[0]);
  return rotated;
}

/**
 * Detects directed cycles in a task dependency graph using DFS state marking.
 * Edge convention: predecessor -> successor (dep -> task).
 *
 * @param input LiveSpecDoc or array of SpecTask
 * @returns CycleDetectionResult containing whether cycles exist, the first cycle, and all distinct cycles.
 */
export function detectCycle(input: LiveSpecDoc | SpecTask[]): CycleDetectionResult {
  const tasks = extractTasks(input);
  const taskMap = new Map<string, SpecTask>();
  for (const t of tasks) {
    taskMap.set(t.id, t);
  }

  // Build adjacency list: dep -> task (predecessor -> successor)
  const adj = new Map<string, string[]>();
  for (const t of tasks) {
    if (!adj.has(t.id)) {
      adj.set(t.id, []);
    }
  }

  for (const t of tasks) {
    const deps = t.depends_on || [];
    for (const dep of deps) {
      // Self loop
      if (dep === t.id) {
        if (!adj.get(t.id)!.includes(t.id)) {
          adj.get(t.id)!.push(t.id);
        }
        continue;
      }
      // If dependency is a known task in the spec
      if (taskMap.has(dep)) {
        adj.get(dep)!.push(t.id);
      }
    }
  }

  // 0 = unvisited, 1 = visiting (in recursion stack), 2 = visited
  const state = new Map<string, number>();
  for (const id of taskMap.keys()) {
    state.set(id, 0);
  }

  const detectedCycles: string[][] = [];
  const cycleKeys = new Set<string>();

  const currentPath: string[] = [];

  function dfs(u: string) {
    state.set(u, 1);
    currentPath.push(u);

    const neighbors = adj.get(u) || [];
    for (const v of neighbors) {
      // Check for self-loop edge
      if (v === u) {
        const cycle = [u, u];
        const key = cycle.join('->');
        if (!cycleKeys.has(key)) {
          cycleKeys.add(key);
          detectedCycles.push(cycle);
        }
        continue;
      }

      const vState = state.get(v) ?? 0;
      if (vState === 1) {
        // Back edge found: cycle detected!
        const cycleStartIdx = currentPath.indexOf(v);
        if (cycleStartIdx !== -1) {
          const rawCycle = [...currentPath.slice(cycleStartIdx), v];
          const canonical = canonicalCycle(rawCycle);
          const key = canonical.join('->');
          if (!cycleKeys.has(key)) {
            cycleKeys.add(key);
            detectedCycles.push(canonical);
          }
        }
      } else if (vState === 0) {
        dfs(v);
      }
    }

    currentPath.pop();
    state.set(u, 2);
  }

  for (const id of taskMap.keys()) {
    if ((state.get(id) ?? 0) === 0) {
      dfs(id);
    }
  }

  const hasCycle = detectedCycles.length > 0;
  return {
    hasCycle,
    cycle: hasCycle ? detectedCycles[0] : undefined,
    cycles: detectedCycles,
  };
}
