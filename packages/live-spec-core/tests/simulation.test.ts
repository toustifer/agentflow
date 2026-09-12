import { describe, it, expect, beforeEach } from 'vitest';
import { SpecSimulator, createSimulator } from '../src/simulation';
import { LiveSpecDoc, SpecTask } from '../src/types';

describe('SpecSimulator', () => {
  describe('Initialization', () => {
    it('should initialize correctly with an empty task document', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Empty Doc',
        tasks: [],
      };
      const sim = new SpecSimulator(doc);

      expect(sim.clock).toBe(0);
      expect(sim.activeRunningCount).toBe(0);
      expect(sim.isComplete).toBe(true);

      const snapshot = sim.getSnapshot();
      expect(snapshot.clock).toBe(0);
      expect(snapshot.activeRunningCount).toBe(0);
      expect(snapshot.isComplete).toBe(true);
      expect(Object.keys(snapshot.tasks).length).toBe(0);
    });

    it('should initialize tasks snapshot from LiveSpecDoc with default pending state', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Basic Doc',
        tasks: [
          { id: 'T1', title: 'Task 1', estimated_hours: 2 },
          { id: 'T2', title: 'Task 2', depends_on: ['T1'], estimated_hours: 1 },
        ],
      };
      const sim = new SpecSimulator(doc);

      expect(sim.clock).toBe(0);
      expect(sim.activeRunningCount).toBe(0);
      expect(sim.isComplete).toBe(false);

      const t1 = sim.getTask('T1');
      expect(t1).toBeDefined();
      expect(t1?.state).toBe('pending');
      expect(t1?.progress).toBe(0);
      expect(t1?.duration).toBe(2);

      const t2 = sim.getTask('T2');
      expect(t2).toBeDefined();
      expect(t2?.state).toBe('pending');
      expect(t2?.depends_on).toEqual(['T1']);
    });

    it('should respect initial state if specified in doc task', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Pre-seeded Doc',
        tasks: [
          { id: 'T1', title: 'Already Done', state: 'pass' },
          { id: 'T2', title: 'Needs Work', state: 'pending', depends_on: ['T1'] },
        ],
      };
      const sim = new SpecSimulator(doc);
      expect(sim.getTask('T1')?.state).toBe('pass');
      expect(sim.getTask('T2')?.state).toBe('pending');
    });

    it('should support createSimulator factory helper', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Factory Test',
        tasks: [{ id: 'T1', title: 'Solo' }],
      };
      const sim = createSimulator(doc, { concurrency: 3 });
      expect(sim).toBeInstanceOf(SpecSimulator);
      expect(sim.concurrency).toBe(3);
    });
  });

  describe('Tick-based State Machine Progression', () => {
    it('should advance a single task from pending -> ready -> running -> pass', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Single Task Flow',
        tasks: [{ id: 'T1', title: 'Solo Task', estimated_hours: 1 }],
      };
      const sim = new SpecSimulator(doc);

      // Initial state
      expect(sim.clock).toBe(0);
      expect(sim.getTask('T1')?.state).toBe('pending');
      expect(sim.isComplete).toBe(false);

      // Tick 1: T1 has no dependencies, transitions pending -> ready -> running
      sim.tick();
      expect(sim.clock).toBe(1);
      expect(sim.getTask('T1')?.state).toBe('running');
      expect(sim.activeRunningCount).toBe(1);
      expect(sim.isComplete).toBe(false);

      // Tick 2: T1 finishes 1 hour duration, transitions running -> pass
      sim.tick();
      expect(sim.clock).toBe(2);
      expect(sim.getTask('T1')?.state).toBe('pass');
      expect(sim.activeRunningCount).toBe(0);
      expect(sim.isComplete).toBe(true);
    });

    it('should handle multi-hour task duration correctly', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Multi-hour Task',
        tasks: [{ id: 'T1', title: 'Long Task', estimated_hours: 3 }],
      };
      const sim = new SpecSimulator(doc);

      // Tick 1: Starts running
      sim.tick();
      expect(sim.clock).toBe(1);
      expect(sim.getTask('T1')?.state).toBe('running');
      expect(sim.getTask('T1')?.progress).toBe(0);
      expect(sim.activeRunningCount).toBe(1);

      // Tick 2: Running, progress = 1
      sim.tick();
      expect(sim.clock).toBe(2);
      expect(sim.getTask('T1')?.state).toBe('running');
      expect(sim.getTask('T1')?.progress).toBe(1);

      // Tick 3: Running, progress = 2
      sim.tick();
      expect(sim.clock).toBe(3);
      expect(sim.getTask('T1')?.state).toBe('running');
      expect(sim.getTask('T1')?.progress).toBe(2);

      // Tick 4: Reaches duration 3, completes to pass
      sim.tick();
      expect(sim.clock).toBe(4);
      expect(sim.getTask('T1')?.state).toBe('pass');
      expect(sim.activeRunningCount).toBe(0);
      expect(sim.isComplete).toBe(true);
    });
  });

  describe('Dependency Chaining', () => {
    it('should respect linear dependencies: A -> B', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Linear A -> B',
        tasks: [
          { id: 'A', title: 'Task A', estimated_hours: 1 },
          { id: 'B', title: 'Task B', depends_on: ['A'], estimated_hours: 1 },
        ],
      };
      const sim = new SpecSimulator(doc);

      // Tick 1: A becomes running, B stays pending
      sim.tick();
      expect(sim.getTask('A')?.state).toBe('running');
      expect(sim.getTask('B')?.state).toBe('pending');
      expect(sim.activeRunningCount).toBe(1);

      // Tick 2: A completes to pass, B becomes ready -> running
      sim.tick();
      expect(sim.getTask('A')?.state).toBe('pass');
      expect(sim.getTask('B')?.state).toBe('running');
      expect(sim.activeRunningCount).toBe(1);

      // Tick 3: B completes to pass, all done
      sim.tick();
      expect(sim.getTask('A')?.state).toBe('pass');
      expect(sim.getTask('B')?.state).toBe('pass');
      expect(sim.activeRunningCount).toBe(0);
      expect(sim.isComplete).toBe(true);
    });

    it('should wait for all dependencies in diamond DAG: Start -> (B, C) -> End', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Diamond DAG',
        concurrency: 2,
        tasks: [
          { id: 'Start', title: 'Start', estimated_hours: 1 },
          { id: 'B', title: 'Fast Branch', depends_on: ['Start'], estimated_hours: 1 },
          { id: 'C', title: 'Slow Branch', depends_on: ['Start'], estimated_hours: 2 },
          { id: 'End', title: 'End Task', depends_on: ['B', 'C'], estimated_hours: 1 },
        ],
      };
      const sim = new SpecSimulator(doc);

      // Tick 1: Start runs
      sim.tick();
      expect(sim.getTask('Start')?.state).toBe('running');

      // Tick 2: Start passes, B and C both become ready -> running (concurrency 2)
      sim.tick();
      expect(sim.getTask('Start')?.state).toBe('pass');
      expect(sim.getTask('B')?.state).toBe('running');
      expect(sim.getTask('C')?.state).toBe('running');
      expect(sim.getTask('End')?.state).toBe('pending');
      expect(sim.activeRunningCount).toBe(2);

      // Tick 3: B passes (1h), C still running (progress 1 of 2), End still pending because C is not pass
      sim.tick();
      expect(sim.getTask('B')?.state).toBe('pass');
      expect(sim.getTask('C')?.state).toBe('running');
      expect(sim.getTask('End')?.state).toBe('pending');
      expect(sim.activeRunningCount).toBe(1);

      // Tick 4: C passes (2h), End becomes ready -> running
      sim.tick();
      expect(sim.getTask('C')?.state).toBe('pass');
      expect(sim.getTask('End')?.state).toBe('running');
      expect(sim.activeRunningCount).toBe(1);

      // Tick 5: End passes
      sim.tick();
      expect(sim.getTask('End')?.state).toBe('pass');
      expect(sim.activeRunningCount).toBe(0);
      expect(sim.isComplete).toBe(true);
    });
  });

  describe('Concurrency Slot Control', () => {
    it('should throttle running tasks according to concurrency limit', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Concurrency Limit 1',
        concurrency: 1,
        tasks: [
          { id: 'T1', title: 'Task 1', estimated_hours: 1 },
          { id: 'T2', title: 'Task 2', estimated_hours: 1 },
        ],
      };
      const sim = new SpecSimulator(doc);

      // Tick 1: Both T1 and T2 are ready, but concurrency is 1, so only T1 becomes running
      sim.tick();
      expect(sim.getTask('T1')?.state).toBe('running');
      expect(sim.getTask('T2')?.state).toBe('ready');
      expect(sim.activeRunningCount).toBe(1);

      // Tick 2: T1 passes, releasing slot, T2 transitions ready -> running
      sim.tick();
      expect(sim.getTask('T1')?.state).toBe('pass');
      expect(sim.getTask('T2')?.state).toBe('running');
      expect(sim.activeRunningCount).toBe(1);

      // Tick 3: T2 passes
      sim.tick();
      expect(sim.getTask('T2')?.state).toBe('pass');
      expect(sim.activeRunningCount).toBe(0);
      expect(sim.isComplete).toBe(true);
    });

    it('should prioritize ready tasks by priority descending', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Priority Scheduling',
        concurrency: 1,
        tasks: [
          { id: 'Low', title: 'Low Priority', priority: 1, estimated_hours: 1 },
          { id: 'High', title: 'High Priority', priority: 10, estimated_hours: 1 },
        ],
      };
      const sim = new SpecSimulator(doc);

      // Tick 1: High priority should be chosen first
      sim.tick();
      expect(sim.getTask('High')?.state).toBe('running');
      expect(sim.getTask('Low')?.state).toBe('ready');

      // Tick 2: High finishes, Low runs
      sim.tick();
      expect(sim.getTask('High')?.state).toBe('pass');
      expect(sim.getTask('Low')?.state).toBe('running');
    });

    it('should dynamically update concurrency limit', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Dynamic Concurrency',
        concurrency: 1,
        tasks: [
          { id: 'T1', title: 'Task 1', estimated_hours: 2 },
          { id: 'T2', title: 'Task 2', estimated_hours: 2 },
        ],
      };
      const sim = new SpecSimulator(doc);

      sim.tick();
      expect(sim.getTask('T1')?.state).toBe('running');
      expect(sim.getTask('T2')?.state).toBe('ready');
      expect(sim.activeRunningCount).toBe(1);

      // Increase concurrency to 2
      sim.setConcurrency(2);
      expect(sim.concurrency).toBe(2);

      // On next tick, T2 should now be picked up into running
      sim.tick();
      expect(sim.getTask('T1')?.state).toBe('running');
      expect(sim.getTask('T2')?.state).toBe('running');
      expect(sim.activeRunningCount).toBe(2);
    });
  });

  describe('Failure Injection (injectFailure)', () => {
    it('should inject failure into a running task and transition to rework', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Failure Test',
        concurrency: 1,
        tasks: [
          { id: 'T1', title: 'Fragile Task', estimated_hours: 2 },
          { id: 'T2', title: 'Waiting Task', estimated_hours: 1 },
        ],
      };
      const sim = new SpecSimulator(doc);

      sim.tick();
      expect(sim.getTask('T1')?.state).toBe('running');
      expect(sim.activeRunningCount).toBe(1);

      // Inject failure into T1
      const ok = sim.injectFailure('T1', 'Agent crashed on tool call');
      expect(ok).toBe(true);

      const t1 = sim.getTask('T1');
      expect(t1?.state).toBe('rework');
      expect(t1?.failureReason).toBe('Agent crashed on tool call');
      // Running slot freed
      expect(sim.activeRunningCount).toBe(0);

      // Next tick: since slot freed, T2 can run
      sim.tick();
      expect(sim.getTask('T2')?.state).toBe('running');
      expect(sim.getTask('T1')?.state).toBe('rework');
    });

    it('should return false when injecting failure to a non-existent task', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Non-existent',
        tasks: [{ id: 'T1', title: 'Task 1' }],
      };
      const sim = new SpecSimulator(doc);
      expect(sim.injectFailure('UNKNOWN')).toBe(false);
    });

    it('should prevent isComplete when a task is in rework', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Rework Incomplete',
        tasks: [{ id: 'T1', title: 'Task 1', estimated_hours: 1 }],
      };
      const sim = new SpecSimulator(doc);
      sim.tick();
      sim.injectFailure('T1');

      sim.tick();
      expect(sim.getTask('T1')?.state).toBe('rework');
      expect(sim.isComplete).toBe(false);
    });

    it('should support retryTask to recover from rework state', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Retry Recovery',
        tasks: [{ id: 'T1', title: 'Task 1', estimated_hours: 1 }],
      };
      const sim = new SpecSimulator(doc);
      sim.tick(); // T1 running
      sim.injectFailure('T1', 'Network timeout');
      expect(sim.getTask('T1')?.state).toBe('rework');

      // Retry task
      const retried = sim.retryTask('T1');
      expect(retried).toBe(true);
      expect(sim.getTask('T1')?.state).toBe('ready');
      expect(sim.getTask('T1')?.failureReason).toBeUndefined();

      // Next tick: T1 runs again
      sim.tick();
      expect(sim.getTask('T1')?.state).toBe('running');

      // Next tick: T1 completes
      sim.tick();
      expect(sim.getTask('T1')?.state).toBe('pass');
      expect(sim.isComplete).toBe(true);
    });
  });

  describe('Reset and runUntilComplete', () => {
    it('should reset simulation state back to initial', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Reset Test',
        tasks: [
          { id: 'T1', title: 'Task 1', estimated_hours: 1 },
          { id: 'T2', title: 'Task 2', depends_on: ['T1'], estimated_hours: 1 },
        ],
      };
      const sim = new SpecSimulator(doc);

      sim.tick(); // T1 running
      sim.tick(); // T1 pass, T2 running
      expect(sim.clock).toBe(2);
      expect(sim.activeRunningCount).toBe(1);

      // Reset
      sim.reset();
      expect(sim.clock).toBe(0);
      expect(sim.activeRunningCount).toBe(0);
      expect(sim.isComplete).toBe(false);
      expect(sim.getTask('T1')?.state).toBe('pending');
      expect(sim.getTask('T2')?.state).toBe('pending');

      // Can tick again from fresh
      sim.tick();
      expect(sim.clock).toBe(1);
      expect(sim.getTask('T1')?.state).toBe('running');
    });

    it('should runUntilComplete to advance all tasks to pass', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Run Until Complete',
        tasks: [
          { id: 'A', title: 'Task A', estimated_hours: 1 },
          { id: 'B', title: 'Task B', depends_on: ['A'], estimated_hours: 2 },
        ],
      };
      const sim = new SpecSimulator(doc);
      const snapshot = sim.runUntilComplete();

      expect(snapshot.isComplete).toBe(true);
      expect(sim.isComplete).toBe(true);
      expect(sim.getTask('A')?.state).toBe('pass');
      expect(sim.getTask('B')?.state).toBe('pass');
      expect(sim.activeRunningCount).toBe(0);
    });

    it('runUntilComplete should terminate safely when deadlock or rework halts progress', () => {
      const doc: LiveSpecDoc = {
        version: '1.0',
        title: 'Deadlock Test',
        tasks: [
          { id: 'A', title: 'Task A', depends_on: ['B'] },
          { id: 'B', title: 'Task B', depends_on: ['A'] },
        ],
      };
      const sim = new SpecSimulator(doc);
      const snapshot = sim.runUntilComplete(10);
      // Because of cycle deadlock, neither task can start
      expect(snapshot.isComplete).toBe(false);
      expect(snapshot.clock).toBeLessThanOrEqual(10);
    });
  });
});
