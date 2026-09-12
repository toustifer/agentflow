import { CPMResult, CPMTaskNode, LiveSpecDoc, SpecTask } from './types';
import { detectCycle } from './cycle';

function extractTasks(input: LiveSpecDoc | SpecTask[]): SpecTask[] {
  if (Array.isArray(input)) {
    return input;
  }
  return input.tasks || [];
}

/**
 * Calculates Critical Path Method (CPM) metrics for a given DAG of tasks.
 * Computes Early Start/Finish, Late Start/Finish, Slack, Critical Path(s),
 * total duration, and bottleneck tasks.
 */
export function calculateCPM(input: LiveSpecDoc | SpecTask[]): CPMResult {
  const tasks = extractTasks(input);

  if (tasks.length === 0) {
    return {
      hasCycle: false,
      totalDuration: 0,
      criticalPath: [],
      criticalPaths: [],
      bottlenecks: [],
      nodes: {},
    };
  }

  // Check for cycles first
  const cycleCheck = detectCycle(tasks);
  if (cycleCheck.hasCycle) {
    return {
      hasCycle: true,
      totalDuration: 0,
      criticalPath: [],
      criticalPaths: [],
      bottlenecks: [],
      nodes: {},
      error: `Cannot calculate CPM: cycle detected (${cycleCheck.cycle?.join(' -> ')})`,
    };
  }

  const taskMap = new Map<string, SpecTask>();
  for (const t of tasks) {
    taskMap.set(t.id, t);
  }

  // Predecessors and successors
  const predecessors = new Map<string, string[]>();
  const successors = new Map<string, string[]>();
  const durations = new Map<string, number>();

  for (const t of tasks) {
    predecessors.set(t.id, []);
    successors.set(t.id, []);
    const dur = typeof t.estimated_hours === 'number' && t.estimated_hours > 0 ? t.estimated_hours : 1;
    durations.set(t.id, dur);
  }

  for (const t of tasks) {
    const rawDeps = t.depends_on || [];
    for (const dep of rawDeps) {
      if (taskMap.has(dep)) {
        predecessors.get(t.id)!.push(dep);
        successors.get(dep)!.push(t.id);
      }
    }
  }

  // Topological sort via Kahn's algorithm
  const inDegree = new Map<string, number>();
  for (const t of tasks) {
    inDegree.set(t.id, predecessors.get(t.id)!.length);
  }

  const queue: string[] = [];
  for (const [id, deg] of inDegree.entries()) {
    if (deg === 0) {
      queue.push(id);
    }
  }

  const topoOrder: string[] = [];
  while (queue.length > 0) {
    const u = queue.shift()!;
    topoOrder.push(u);
    for (const v of successors.get(u) || []) {
      const nextDeg = inDegree.get(v)! - 1;
      inDegree.set(v, nextDeg);
      if (nextDeg === 0) {
        queue.push(v);
      }
    }
  }

  // Forward pass: Calculate ES and EF
  const esMap = new Map<string, number>();
  const efMap = new Map<string, number>();

  for (const u of topoOrder) {
    const preds = predecessors.get(u) || [];
    let es = 0;
    if (preds.length > 0) {
      for (const p of preds) {
        const pEf = efMap.get(p) ?? 0;
        if (pEf > es) {
          es = pEf;
        }
      }
    }
    esMap.set(u, es);
    efMap.set(u, es + durations.get(u)!);
  }

  let totalDuration = 0;
  for (const ef of efMap.values()) {
    if (ef > totalDuration) {
      totalDuration = ef;
    }
  }

  // Backward pass: Calculate LF, LS, and Slack
  const lfMap = new Map<string, number>();
  const lsMap = new Map<string, number>();
  const slackMap = new Map<string, number>();
  const isCriticalMap = new Map<string, boolean>();

  for (let i = topoOrder.length - 1; i >= 0; i--) {
    const u = topoOrder[i];
    const succs = successors.get(u) || [];
    let lf = totalDuration;
    if (succs.length > 0) {
      lf = Infinity;
      for (const s of succs) {
        const sLs = lsMap.get(s) ?? totalDuration;
        if (sLs < lf) {
          lf = sLs;
        }
      }
    }
    const ls = lf - durations.get(u)!;
    const slack = lf - efMap.get(u)!;
    lfMap.set(u, lf);
    lsMap.set(u, ls);
    slackMap.set(u, Math.round(slack * 1000) / 1000);
    isCriticalMap.set(u, Math.abs(slack) < 1e-6);
  }

  // Build CPM nodes
  const nodes: Record<string, CPMTaskNode> = {};
  for (const t of tasks) {
    const u = t.id;
    nodes[u] = {
      id: u,
      duration: durations.get(u)!,
      es: esMap.get(u)!,
      ef: efMap.get(u)!,
      ls: lsMap.get(u)!,
      lf: lfMap.get(u)!,
      slack: slackMap.get(u)!,
      isCritical: isCriticalMap.get(u)!,
      predecessors: predecessors.get(u) || [],
      successors: successors.get(u) || [],
    };
  }

  // Extract Critical Path(s)
  // A path is critical if all its nodes are critical,
  // it starts at a critical root (es === 0), ends at an exit (ef === totalDuration),
  // and for each step (u -> v): ef(u) === es(v).
  const criticalPaths: string[][] = [];

  function findCriticalPaths(curr: string, path: string[]) {
    const succs = successors.get(curr) || [];
    const criticalSuccs = succs.filter(
      (s) => isCriticalMap.get(s) && Math.abs(efMap.get(curr)! - esMap.get(s)!) < 1e-6
    );

    if (criticalSuccs.length === 0) {
      if (Math.abs(efMap.get(curr)! - totalDuration) < 1e-6) {
        criticalPaths.push([...path]);
      }
      return;
    }

    for (const s of criticalSuccs) {
      path.push(s);
      findCriticalPaths(s, path);
      path.pop();
    }
  }

  const rootCriticalNodes = topoOrder.filter(
    (u) => isCriticalMap.get(u) && esMap.get(u) === 0
  );

  for (const root of rootCriticalNodes) {
    findCriticalPaths(root, [root]);
  }

  const criticalPath = criticalPaths[0] || [];

  // Bottlenecks: Critical nodes sorted by duration descending
  const bottlenecks = Object.values(nodes)
    .filter((n) => n.isCritical)
    .sort((a, b) => {
      if (b.duration !== a.duration) {
        return b.duration - a.duration;
      }
      return a.id.localeCompare(b.id);
    })
    .map((n) => n.id);

  return {
    hasCycle: false,
    totalDuration,
    criticalPath,
    criticalPaths,
    bottlenecks,
    nodes,
  };
}
