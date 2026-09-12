import { describe, it, expect } from 'vitest';
import { computeSpecDiff } from '../src/diff';
import { LiveSpecDoc } from '../src/types';

describe('computeSpecDiff', () => {
  const baseDoc: LiveSpecDoc = {
    version: '1.0',
    title: 'Project Alpha',
    concurrency: 2,
    tasks: [
      { id: 'T1', title: 'Task 1', estimated_hours: 2, assigned_worker: 'worker-a', depends_on: [] },
      { id: 'T2', title: 'Task 2', estimated_hours: 3, assigned_worker: 'worker-b', depends_on: ['T1'] },
      { id: 'T3', title: 'Task 3', estimated_hours: 4, assigned_worker: 'worker-c', depends_on: ['T2'] },
    ],
  };

  it('should detect no changes when documents are identical', () => {
    const currentDoc: LiveSpecDoc = JSON.parse(JSON.stringify(baseDoc));
    const diff = computeSpecDiff(baseDoc, currentDoc);

    expect(diff.hasChanges).toBe(false);
    expect(diff.addedTasks).toEqual([]);
    expect(diff.removedTasks).toEqual([]);
    expect(diff.modifiedTasks).toEqual([]);
    expect(diff.addedDependencies).toEqual([]);
    expect(diff.removedDependencies).toEqual([]);
    expect(diff.parameterChanges).toEqual([]);
    expect(diff.summary).toContain('无变更');
  });

  it('should detect added tasks', () => {
    const currentDoc: LiveSpecDoc = {
      ...baseDoc,
      tasks: [
        ...baseDoc.tasks,
        { id: 'T4', title: 'Task 4', estimated_hours: 1, depends_on: ['T3'] },
      ],
    };

    const diff = computeSpecDiff(baseDoc, currentDoc);
    expect(diff.hasChanges).toBe(true);
    expect(diff.addedTasks.length).toBe(1);
    expect(diff.addedTasks[0].id).toBe('T4');
    expect(diff.addedDependencies).toEqual([{ from: 'T3', to: 'T4' }]);
    expect(diff.summary).toContain('新增任务');
    expect(diff.summary).toContain('T4');
  });

  it('should detect removed tasks', () => {
    const currentDoc: LiveSpecDoc = {
      ...baseDoc,
      tasks: baseDoc.tasks.filter((t) => t.id !== 'T3'),
    };

    const diff = computeSpecDiff(baseDoc, currentDoc);
    expect(diff.hasChanges).toBe(true);
    expect(diff.removedTasks.length).toBe(1);
    expect(diff.removedTasks[0].id).toBe('T3');
    expect(diff.removedDependencies).toEqual([{ from: 'T2', to: 'T3' }]);
    expect(diff.summary).toContain('删除任务');
    expect(diff.summary).toContain('T3');
  });

  it('should detect task property modifications (hours, worker, title)', () => {
    const currentDoc: LiveSpecDoc = {
      ...baseDoc,
      tasks: [
        {
          ...baseDoc.tasks[0],
          estimated_hours: 5,
          assigned_worker: 'worker-x',
          title: 'Task 1 Updated',
        },
        baseDoc.tasks[1],
        baseDoc.tasks[2],
      ],
    };

    const diff = computeSpecDiff(baseDoc, currentDoc);
    expect(diff.hasChanges).toBe(true);
    expect(diff.modifiedTasks.length).toBe(1);
    expect(diff.modifiedTasks[0].id).toBe('T1');

    const fields = diff.modifiedTasks[0].changes?.map((c) => c.field);
    expect(fields).toContain('estimated_hours');
    expect(fields).toContain('assigned_worker');
    expect(fields).toContain('title');

    expect(diff.summary).toContain('属性调整');
    expect(diff.summary).toContain('T1');
  });

  it('should detect dependency additions and removals', () => {
    // Modify T3 to depend directly on T1 instead of T2
    const currentDoc: LiveSpecDoc = {
      ...baseDoc,
      tasks: [
        baseDoc.tasks[0],
        baseDoc.tasks[1],
        {
          ...baseDoc.tasks[2],
          depends_on: ['T1'], // removed T2, added T1
        },
      ],
    };

    const diff = computeSpecDiff(baseDoc, currentDoc);
    expect(diff.hasChanges).toBe(true);
    expect(diff.addedDependencies).toEqual([{ from: 'T1', to: 'T3' }]);
    expect(diff.removedDependencies).toEqual([{ from: 'T2', to: 'T3' }]);
    expect(diff.summary).toContain('依赖调整');
    expect(diff.summary).toContain('新增依赖');
    expect(diff.summary).toContain('移除依赖');
  });

  it('should detect global parameter changes like concurrency', () => {
    const currentDoc: LiveSpecDoc = {
      ...baseDoc,
      concurrency: 5,
    };

    const diff = computeSpecDiff(baseDoc, currentDoc);
    expect(diff.hasChanges).toBe(true);
    expect(diff.parameterChanges).toEqual([
      { field: 'concurrency', oldValue: 2, newValue: 5 },
    ]);
    expect(diff.summary).toContain('全局参数调整');
    expect(diff.summary).toContain('concurrency');
  });

  it('should include CPM impact in summary when topology or duration changes', () => {
    // T1 duration goes from 2h to 6h, total project duration increases
    const currentDoc: LiveSpecDoc = {
      ...baseDoc,
      tasks: [
        { ...baseDoc.tasks[0], estimated_hours: 6 },
        baseDoc.tasks[1],
        baseDoc.tasks[2],
      ],
    };

    const diff = computeSpecDiff(baseDoc, currentDoc);
    expect(diff.hasChanges).toBe(true);
    expect(diff.summary).toContain('工期');
  });
});
