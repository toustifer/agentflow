import { describe, it, expect } from 'vitest';
import { detectCycle } from '../src/cycle';
import { LiveSpecDoc, SpecTask } from '../src/types';

describe('detectCycle', () => {
  it('should return false for empty tasks', () => {
    const doc: LiveSpecDoc = {
      version: '1.0',
      title: 'Empty Spec',
      tasks: [],
    };
    const result = detectCycle(doc);
    expect(result.hasCycle).toBe(false);
    expect(result.cycles).toEqual([]);
    expect(result.cycle).toBeUndefined();
  });

  it('should return false for a simple linear DAG', () => {
    const tasks: SpecTask[] = [
      { id: 'T1', title: 'Task 1', depends_on: [] },
      { id: 'T2', title: 'Task 2', depends_on: ['T1'] },
      { id: 'T3', title: 'Task 3', depends_on: ['T2'] },
    ];
    const result = detectCycle(tasks);
    expect(result.hasCycle).toBe(false);
    expect(result.cycles).toEqual([]);
  });

  it('should return false for a diamond DAG', () => {
    const tasks: SpecTask[] = [
      { id: 'A', title: 'Start', depends_on: [] },
      { id: 'B', title: 'Branch 1', depends_on: ['A'] },
      { id: 'C', title: 'Branch 2', depends_on: ['A'] },
      { id: 'D', title: 'Join', depends_on: ['B', 'C'] },
    ];
    const result = detectCycle(tasks);
    expect(result.hasCycle).toBe(false);
  });

  it('should detect self-referencing cycle (A -> A)', () => {
    const tasks: SpecTask[] = [
      { id: 'A', title: 'Self loop', depends_on: ['A'] },
    ];
    const result = detectCycle(tasks);
    expect(result.hasCycle).toBe(true);
    expect(result.cycle).toBeDefined();
    expect(result.cycle).toEqual(['A', 'A']);
    expect(result.cycles.length).toBeGreaterThan(0);
  });

  it('should detect a 2-node cycle (A -> B -> A)', () => {
    const tasks: SpecTask[] = [
      { id: 'A', title: 'Task A', depends_on: ['B'] },
      { id: 'B', title: 'Task B', depends_on: ['A'] },
    ];
    const result = detectCycle(tasks);
    expect(result.hasCycle).toBe(true);
    expect(result.cycle).toBeDefined();
    // Cycle should start and end with the same node
    expect(result.cycle![0]).toBe(result.cycle![result.cycle!.length - 1]);
    expect(result.cycle).toEqual(expect.arrayContaining(['A', 'B']));
  });

  it('should detect a 3-node cycle embedded in a larger graph', () => {
    const tasks: SpecTask[] = [
      { id: 'Start', title: 'Start', depends_on: [] },
      { id: 'A', title: 'A', depends_on: ['Start', 'C'] },
      { id: 'B', title: 'B', depends_on: ['A'] },
      { id: 'C', title: 'C', depends_on: ['B'] },
      { id: 'End', title: 'End', depends_on: ['C'] },
    ];
    const result = detectCycle(tasks);
    expect(result.hasCycle).toBe(true);
    expect(result.cycle).toBeDefined();
    // Should identify the cycle between A, B, C
    const cycleNodes = result.cycle!.slice(0, -1);
    expect(cycleNodes.sort()).toEqual(['A', 'B', 'C'].sort());
  });

  it('should detect multiple independent cycles', () => {
    const tasks: SpecTask[] = [
      // Cycle 1: C1_A <-> C1_B
      { id: 'C1_A', title: 'C1 A', depends_on: ['C1_B'] },
      { id: 'C1_B', title: 'C1 B', depends_on: ['C1_A'] },
      // Cycle 2: C2_X -> C2_Y -> C2_Z -> C2_X
      { id: 'C2_X', title: 'C2 X', depends_on: ['C2_Z'] },
      { id: 'C2_Y', title: 'C2 Y', depends_on: ['C2_X'] },
      { id: 'C2_Z', title: 'C2 Z', depends_on: ['C2_Y'] },
      // Independent non-cyclic task
      { id: 'Independent', title: 'Ind', depends_on: [] },
    ];
    const result = detectCycle(tasks);
    expect(result.hasCycle).toBe(true);
    expect(result.cycles.length).toBeGreaterThanOrEqual(2);
  });

  it('should handle tasks with non-existent dependencies gracefully', () => {
    const tasks: SpecTask[] = [
      { id: 'A', title: 'A', depends_on: ['non-existent'] },
      { id: 'B', title: 'B', depends_on: ['A'] },
    ];
    const result = detectCycle(tasks);
    expect(result.hasCycle).toBe(false);
  });
});
