import { describe, it, expect } from 'vitest';
import { calculateCPM } from '../src/cpm';
import { LiveSpecDoc, SpecTask } from '../src/types';

describe('calculateCPM', () => {
  it('should handle empty task list', () => {
    const doc: LiveSpecDoc = {
      version: '1.0',
      title: 'Empty',
      tasks: [],
    };
    const result = calculateCPM(doc);
    expect(result.hasCycle).toBe(false);
    expect(result.totalDuration).toBe(0);
    expect(result.criticalPath).toEqual([]);
    expect(result.criticalPaths).toEqual([]);
    expect(result.bottlenecks).toEqual([]);
    expect(result.nodes).toEqual({});
  });

  it('should calculate CPM for a single task', () => {
    const tasks: SpecTask[] = [
      { id: 'T1', title: 'Solo Task', estimated_hours: 5 },
    ];
    const result = calculateCPM(tasks);
    expect(result.hasCycle).toBe(false);
    expect(result.totalDuration).toBe(5);
    expect(result.criticalPath).toEqual(['T1']);
    expect(result.bottlenecks).toEqual(['T1']);
    expect(result.nodes['T1']).toEqual({
      id: 'T1',
      duration: 5,
      es: 0,
      ef: 5,
      ls: 0,
      lf: 5,
      slack: 0,
      isCritical: true,
      predecessors: [],
      successors: [],
    });
  });

  it('should calculate CPM for a linear sequence of tasks', () => {
    const tasks: SpecTask[] = [
      { id: 'A', title: 'Task A', estimated_hours: 2, depends_on: [] },
      { id: 'B', title: 'Task B', estimated_hours: 3, depends_on: ['A'] },
      { id: 'C', title: 'Task C', estimated_hours: 4, depends_on: ['B'] },
    ];
    const result = calculateCPM(tasks);
    expect(result.hasCycle).toBe(false);
    expect(result.totalDuration).toBe(9); // 2 + 3 + 4
    expect(result.criticalPath).toEqual(['A', 'B', 'C']);

    // Check node timing
    expect(result.nodes['A'].es).toBe(0);
    expect(result.nodes['A'].ef).toBe(2);
    expect(result.nodes['B'].es).toBe(2);
    expect(result.nodes['B'].ef).toBe(5);
    expect(result.nodes['C'].es).toBe(5);
    expect(result.nodes['C'].ef).toBe(9);

    // Bottlenecks sorted by duration descending: C (4h), B (3h), A (2h)
    expect(result.bottlenecks).toEqual(['C', 'B', 'A']);
  });

  it('should identify the critical path in a branching diamond graph', () => {
    // Start (1h)
    // /        \
    // B (slow, 5h)  C (fast, 2h)
    // \        /
    //   End (2h)
    const tasks: SpecTask[] = [
      { id: 'Start', title: 'Start', estimated_hours: 1, depends_on: [] },
      { id: 'B', title: 'Slow branch', estimated_hours: 5, depends_on: ['Start'] },
      { id: 'C', title: 'Fast branch', estimated_hours: 2, depends_on: ['Start'] },
      { id: 'End', title: 'End', estimated_hours: 2, depends_on: ['B', 'C'] },
    ];

    const result = calculateCPM(tasks);
    expect(result.hasCycle).toBe(false);
    // Path Start -> B -> End = 1 + 5 + 2 = 8
    // Path Start -> C -> End = 1 + 2 + 2 = 5
    expect(result.totalDuration).toBe(8);
    expect(result.criticalPath).toEqual(['Start', 'B', 'End']);

    // Node C should have slack
    expect(result.nodes['C'].isCritical).toBe(false);
    expect(result.nodes['C'].es).toBe(1);
    expect(result.nodes['C'].ef).toBe(3);
    // C's latest finish must be 6 (so End can start at 6), ls = 4, slack = 4 - 1 = 3
    expect(result.nodes['C'].slack).toBe(3);

    // Node B is critical
    expect(result.nodes['B'].isCritical).toBe(true);
    expect(result.nodes['B'].slack).toBe(0);

    // Bottlenecks should be critical nodes sorted by duration descending: B (5), End (2), Start (1)
    expect(result.bottlenecks).toEqual(['B', 'End', 'Start']);
  });

  it('should detect parallel critical paths when durations tie', () => {
    // Start (2h)
    // /        \
    // B (3h)    C (3h)
    // \        /
    //   End (1h)
    const tasks: SpecTask[] = [
      { id: 'Start', title: 'Start', estimated_hours: 2, depends_on: [] },
      { id: 'B', title: 'Branch B', estimated_hours: 3, depends_on: ['Start'] },
      { id: 'C', title: 'Branch C', estimated_hours: 3, depends_on: ['Start'] },
      { id: 'End', title: 'End', estimated_hours: 1, depends_on: ['B', 'C'] },
    ];

    const result = calculateCPM(tasks);
    expect(result.totalDuration).toBe(6); // 2 + 3 + 1
    expect(result.nodes['B'].isCritical).toBe(true);
    expect(result.nodes['C'].isCritical).toBe(true);
    expect(result.criticalPaths.length).toBe(2);
    expect(result.criticalPaths).toEqual(
      expect.arrayContaining([
        ['Start', 'B', 'End'],
        ['Start', 'C', 'End'],
      ])
    );
  });

  it('should return error when graph contains cycle', () => {
    const tasks: SpecTask[] = [
      { id: 'A', title: 'A', estimated_hours: 2, depends_on: ['B'] },
      { id: 'B', title: 'B', estimated_hours: 3, depends_on: ['A'] },
    ];
    const result = calculateCPM(tasks);
    expect(result.hasCycle).toBe(true);
    expect(result.error).toBeDefined();
    expect(result.totalDuration).toBe(0);
    expect(result.criticalPath).toEqual([]);
  });

  it('should default missing or negative estimated_hours to 1', () => {
    const tasks: SpecTask[] = [
      { id: 'A', title: 'A', depends_on: [] }, // missing estimated_hours -> 1
      { id: 'B', title: 'B', estimated_hours: 0, depends_on: ['A'] }, // 0 -> 1
    ];
    const result = calculateCPM(tasks);
    expect(result.nodes['A'].duration).toBe(1);
    expect(result.nodes['B'].duration).toBe(1);
    expect(result.totalDuration).toBe(2);
  });
});
