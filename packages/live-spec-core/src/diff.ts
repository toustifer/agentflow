import {
  DependencyEdge,
  LiveSpecDoc,
  SpecDiffResult,
  SpecTask,
  TaskDiff,
  TaskFieldChange,
} from './types';
import { calculateCPM } from './cpm';

/**
 * Deep compares two values (arrays, primitives, objects).
 */
function isEqual(a: unknown, b: unknown): boolean {
  if (a === b) return true;
  if (typeof a !== typeof b) return false;
  if (a === null || b === null || typeof a !== 'object') return false;
  return JSON.stringify(a) === JSON.stringify(b);
}

/**
 * Computes topological differences, parameter mutations, and a natural language
 * feedback summary between a base LiveSpecDoc and a modified current LiveSpecDoc.
 */
export function computeSpecDiff(
  baseDoc: LiveSpecDoc,
  currentDoc: LiveSpecDoc
): SpecDiffResult {
  const baseTasks = baseDoc.tasks || [];
  const currentTasks = currentDoc.tasks || [];

  const baseMap = new Map<string, SpecTask>();
  for (const t of baseTasks) {
    baseMap.set(t.id, t);
  }

  const currentMap = new Map<string, SpecTask>();
  for (const t of currentTasks) {
    currentMap.set(t.id, t);
  }

  // 1. Added and Removed tasks
  const addedTasks: SpecTask[] = [];
  for (const t of currentTasks) {
    if (!baseMap.has(t.id)) {
      addedTasks.push(t);
    }
  }

  const removedTasks: SpecTask[] = [];
  for (const t of baseTasks) {
    if (!currentMap.has(t.id)) {
      removedTasks.push(t);
    }
  }

  // 2. Modified tasks
  const modifiedTasks: TaskDiff[] = [];
  const taskFieldsToCompare: (keyof SpecTask)[] = [
    'title',
    'description',
    'assigned_worker',
    'estimated_hours',
    'priority',
    'tags',
    'output_files',
    'acceptance_criteria',
    'state',
  ];

  for (const [id, currTask] of currentMap.entries()) {
    const prevTask = baseMap.get(id);
    if (!prevTask) continue;

    const changes: TaskFieldChange[] = [];
    for (const field of taskFieldsToCompare) {
      const oldVal = prevTask[field];
      const newVal = currTask[field];
      if (!isEqual(oldVal, newVal)) {
        changes.push({
          field,
          oldValue: oldVal,
          newValue: newVal,
        });
      }
    }

    if (changes.length > 0) {
      modifiedTasks.push({
        id,
        title: currTask.title || prevTask.title,
        status: 'modified',
        changes,
      });
    }
  }

  // 3. Topological Dependencies: Added and Removed edges
  const baseEdgeSet = new Set<string>();
  for (const t of baseTasks) {
    for (const dep of t.depends_on || []) {
      baseEdgeSet.add(`${dep}=>${t.id}`);
    }
  }

  const currentEdgeSet = new Set<string>();
  for (const t of currentTasks) {
    for (const dep of t.depends_on || []) {
      currentEdgeSet.add(`${dep}=>${t.id}`);
    }
  }

  const addedDependencies: DependencyEdge[] = [];
  for (const edge of currentEdgeSet) {
    if (!baseEdgeSet.has(edge)) {
      const [from, to] = edge.split('=>');
      addedDependencies.push({ from, to });
    }
  }

  const removedDependencies: DependencyEdge[] = [];
  for (const edge of baseEdgeSet) {
    if (!currentEdgeSet.has(edge)) {
      const [from, to] = edge.split('=>');
      removedDependencies.push({ from, to });
    }
  }

  // 4. Global parameter changes
  const parameterChanges: TaskFieldChange[] = [];
  if (baseDoc.concurrency !== currentDoc.concurrency) {
    parameterChanges.push({
      field: 'concurrency',
      oldValue: baseDoc.concurrency,
      newValue: currentDoc.concurrency,
    });
  }
  if (baseDoc.title !== currentDoc.title) {
    parameterChanges.push({
      field: 'title',
      oldValue: baseDoc.title,
      newValue: currentDoc.title,
    });
  }

  const hasChanges =
    addedTasks.length > 0 ||
    removedTasks.length > 0 ||
    modifiedTasks.length > 0 ||
    addedDependencies.length > 0 ||
    removedDependencies.length > 0 ||
    parameterChanges.length > 0;

  // 5. Generate Natural Language Feedback Summary
  const summaryLines: string[] = [];

  if (!hasChanges) {
    summaryLines.push('【Live-Spec 拓扑无变更】当前画布拓扑与配置参数无修改。');
  } else {
    summaryLines.push('### Live-Spec 拓扑与参数变动反哺摘要');

    if (addedTasks.length > 0) {
      summaryLines.push(`\n- 🟢 **新增任务** (${addedTasks.length} 个):`);
      for (const t of addedTasks) {
        summaryLines.push(
          `  * \`${t.id}\`「${t.title}」(预估工期: ${t.estimated_hours ?? 1}h, 负责人: ${t.assigned_worker || '未分配'})`
        );
      }
    }

    if (removedTasks.length > 0) {
      summaryLines.push(`\n- 🔴 **删除任务** (${removedTasks.length} 个):`);
      for (const t of removedTasks) {
        summaryLines.push(`  * \`${t.id}\`「${t.title}」`);
      }
    }

    if (addedDependencies.length > 0 || removedDependencies.length > 0) {
      summaryLines.push(`\n- 🔀 **拓扑依赖调整**:`);
      if (addedDependencies.length > 0) {
        const addedStr = addedDependencies.map((e) => `\`${e.from}\` ➔ \`${e.to}\``).join(', ');
        summaryLines.push(`  * 新增依赖: ${addedStr}`);
      }
      if (removedDependencies.length > 0) {
        const removedStr = removedDependencies.map((e) => `\`${e.from}\` ➔ \`${e.to}\``).join(', ');
        summaryLines.push(`  * 移除依赖: ${removedStr}`);
      }
    }

    if (modifiedTasks.length > 0) {
      summaryLines.push(`\n- ✏️ **任务属性调整** (${modifiedTasks.length} 个):`);
      for (const m of modifiedTasks) {
        const details = (m.changes || [])
          .map((c) => `${c.field}: ${JSON.stringify(c.oldValue)} -> ${JSON.stringify(c.newValue)}`)
          .join(', ');
        summaryLines.push(`  * \`${m.id}\`「${m.title}」: ${details}`);
      }
    }

    if (parameterChanges.length > 0) {
      summaryLines.push(`\n- ⚙️ **全局参数调整**:`);
      for (const p of parameterChanges) {
        summaryLines.push(`  * ${p.field}: ${JSON.stringify(p.oldValue)} -> ${JSON.stringify(p.newValue)}`);
      }
    }

    // 6. CPM Impact Assessment
    try {
      const baseCpm = calculateCPM(baseDoc);
      const currCpm = calculateCPM(currentDoc);

      if (!baseCpm.hasCycle && !currCpm.hasCycle) {
        const diffDuration = currCpm.totalDuration - baseCpm.totalDuration;
        const diffSign = diffDuration >= 0 ? `+${diffDuration}` : `${diffDuration}`;
        summaryLines.push(`\n- ⏱️ **CPM 工期与瓶颈影响**:`);
        summaryLines.push(
          `  * 项目总工期: ${baseCpm.totalDuration}h -> ${currCpm.totalDuration}h (${diffSign}h)`
        );
        summaryLines.push(
          `  * 关键路径: [${baseCpm.criticalPath.join(' -> ')}] -> [${currCpm.criticalPath.join(' -> ')}]`
        );
        if (currCpm.bottlenecks.length > 0) {
          summaryLines.push(`  * 当前首要瓶颈任务: \`${currCpm.bottlenecks[0]}\``);
        }
      } else if (currCpm.hasCycle) {
        summaryLines.push(`\n- ⚠️ **拓扑警告**: 当前拓扑存在有向环路，无法形成有效工期编排！`);
      }
    } catch {
      // Ignore CPM calculation issues during diff
    }
  }

  return {
    hasChanges,
    addedTasks,
    removedTasks,
    modifiedTasks,
    addedDependencies,
    removedDependencies,
    parameterChanges,
    summary: summaryLines.join('\n'),
  };
}
