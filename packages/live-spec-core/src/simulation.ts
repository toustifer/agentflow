import {
  LiveSpecDoc,
  SimulatedTaskState,
  SimulationOptions,
  SimulationSnapshot,
  SpecTask,
  TaskState,
} from './types';

function isPassed(state: TaskState): boolean {
  return state === 'pass' || state === 'passed';
}

function isRunning(state: TaskState): boolean {
  return state === 'running' || state === 'executing';
}

/**
 * SpecSimulator
 * State machine time-step simulation engine for Live-Spec DAG execution.
 * Simulates task transitions: pending -> ready -> running -> pass,
 * respects concurrency limits, tracks clock, calculates active running count,
 * supports fault injection (rework), and provides snapshot/reset capabilities.
 */
export class SpecSimulator {
  private _doc: LiveSpecDoc;
  private _options: SimulationOptions;
  private _clock = 0;
  private _concurrency: number;
  private _tasks: Map<string, SimulatedTaskState> = new Map();
  private _initialSnapshot: Map<string, SimulatedTaskState> = new Map();

  constructor(doc: LiveSpecDoc, options: SimulationOptions = {}) {
    this._doc = doc;
    this._options = options;
    this._concurrency =
      options.concurrency ?? doc.concurrency ?? doc.settings?.concurrency ?? Infinity;
    this.initTasks();
  }

  private initTasks(): void {
    this._clock = 0;
    this._tasks.clear();
    this._initialSnapshot.clear();

    const tasks = this._doc.tasks || [];
    for (const t of tasks) {
      const duration =
        this._options.defaultDuration ??
        (typeof t.estimated_hours === 'number' && t.estimated_hours > 0
          ? Math.ceil(t.estimated_hours)
          : 1);
      const priority = typeof t.priority === 'number' ? t.priority : 0;
      const state = t.state ?? 'pending';
      const progress = isPassed(state) ? duration : 0;

      const simTask: SimulatedTaskState = {
        id: t.id,
        title: t.title,
        state,
        progress,
        duration,
        depends_on: t.depends_on ? [...t.depends_on] : [],
        priority,
        estimated_hours: t.estimated_hours,
        assigned_worker: t.assigned_worker,
        metadata: t.metadata ? { ...t.metadata } : undefined,
      };

      this._tasks.set(t.id, simTask);
      this._initialSnapshot.set(t.id, { ...simTask });
    }
  }

  /**
   * Current simulation clock time (ticks).
   */
  get clock(): number {
    return this._clock;
  }

  /**
   * Current concurrency limit.
   */
  get concurrency(): number {
    return this._concurrency;
  }

  /**
   * Dynamically update concurrency slot capacity.
   */
  setConcurrency(limit: number): void {
    this._concurrency = limit > 0 ? limit : Infinity;
  }

  /**
   * Get single simulated task state by id.
   */
  getTask(id: string): SimulatedTaskState | undefined {
    return this._tasks.get(id);
  }

  /**
   * Get all simulated task states.
   */
  getTasks(): SimulatedTaskState[] {
    return Array.from(this._tasks.values());
  }

  /**
   * Number of tasks actively executing in a concurrency slot.
   */
  get activeRunningCount(): number {
    let count = 0;
    for (const t of this._tasks.values()) {
      if (isRunning(t.state)) {
        count++;
      }
    }
    return count;
  }

  /**
   * Whether all tasks in the document have completed (passed).
   */
  get isComplete(): boolean {
    if (this._tasks.size === 0) {
      return true;
    }
    for (const t of this._tasks.values()) {
      if (!isPassed(t.state)) {
        return false;
      }
    }
    return true;
  }

  /**
   * Generate an immutable snapshot of current simulation status.
   */
  getSnapshot(): SimulationSnapshot {
    const tasksRecord: Record<string, SimulatedTaskState> = {};
    for (const [id, task] of this._tasks.entries()) {
      tasksRecord[id] = { ...task };
    }

    return {
      clock: this._clock,
      activeRunningCount: this.activeRunningCount,
      isComplete: this.isComplete,
      tasks: tasksRecord,
      concurrency: this._concurrency,
    };
  }

  /**
   * Advance the simulation by one discrete time tick.
   * State transitions:
   * 1. Running tasks advance progress -> pass if duration completed
   * 2. Auto-retry rework tasks if enabled
   * 3. Pending tasks check dependencies -> ready if satisfied
   * 4. Ready tasks enter running up to concurrency slots (prioritized by priority)
   */
  tick(): SimulationSnapshot {
    if (this.isComplete) {
      return this.getSnapshot();
    }

    this._clock++;

    // 1. Advance running tasks
    for (const task of this._tasks.values()) {
      if (isRunning(task.state)) {
        task.progress++;
        if (task.progress >= task.duration) {
          task.state = 'pass';
        }
      }
    }

    // 2. Auto-retry rework tasks if option enabled
    if (this._options.autoRetry) {
      for (const task of this._tasks.values()) {
        if (task.state === 'rework') {
          const depsOk = this.areDependenciesSatisfied(task);
          if (depsOk) {
            task.state = 'ready';
            task.failureReason = undefined;
            task.progress = 0;
          }
        }
      }
    }

    // 3. Promote pending tasks to ready if all dependencies are passed
    for (const task of this._tasks.values()) {
      if (task.state === 'pending') {
        if (this.areDependenciesSatisfied(task)) {
          task.state = 'ready';
        }
      }
    }

    // 4. Schedule ready tasks into running slots
    const runningCount = this.activeRunningCount;
    const availableSlots = this._concurrency - runningCount;

    if (availableSlots > 0) {
      const readyTasks: SimulatedTaskState[] = [];
      for (const task of this._tasks.values()) {
        if (task.state === 'ready') {
          readyTasks.push(task);
        }
      }

      // Sort ready tasks by priority descending (stable sort)
      readyTasks.sort((a, b) => b.priority - a.priority);

      const toSchedule = readyTasks.slice(0, availableSlots);
      for (const task of toSchedule) {
        task.state = 'running';
        task.progress = 0;
      }
    }

    return this.getSnapshot();
  }

  /**
   * Alias for tick() for controller compatibility.
   */
  step(): SimulationSnapshot {
    return this.tick();
  }

  /**
   * Checks if all dependencies of a task have reached pass/passed state.
   */
  private areDependenciesSatisfied(task: SimulatedTaskState): boolean {
    if (!task.depends_on || task.depends_on.length === 0) {
      return true;
    }
    for (const depId of task.depends_on) {
      const depTask = this._tasks.get(depId);
      if (!depTask || !isPassed(depTask.state)) {
        return false;
      }
    }
    return true;
  }

  /**
   * Fault injection: triggers task failure and transitions state to 'rework'.
   * Frees concurrency slot if task was running.
   */
  injectFailure(taskId: string, reason?: string): boolean {
    const task = this._tasks.get(taskId);
    if (!task) {
      return false;
    }

    task.state = 'rework';
    task.failureReason = reason ?? 'Fault injected';
    task.progress = 0;

    // In case a previously passed task fails, revert any downstream ready tasks back to pending
    for (const other of this._tasks.values()) {
      if (other.state === 'ready') {
        if (!this.areDependenciesSatisfied(other)) {
          other.state = 'pending';
        }
      }
    }

    return true;
  }

  /**
   * Retry/re-dispatch a task from rework state back into execution workflow.
   */
  retryTask(taskId: string): boolean {
    const task = this._tasks.get(taskId);
    if (!task || task.state !== 'rework') {
      return false;
    }

    task.failureReason = undefined;
    task.progress = 0;

    if (this.areDependenciesSatisfied(task)) {
      task.state = 'ready';
    } else {
      task.state = 'pending';
    }

    return true;
  }

  /**
   * Reset simulation to original initial state.
   */
  reset(): void {
    this._clock = 0;
    for (const [id, initTask] of this._initialSnapshot.entries()) {
      this._tasks.set(id, { ...initTask });
    }
  }

  /**
   * Convenience helper to step through simulation until completion or maxTicks reached.
   */
  runUntilComplete(maxTicks = 1000): SimulationSnapshot {
    let ticks = 0;
    while (!this.isComplete && ticks < maxTicks) {
      this.tick();
      ticks++;

      // If nothing is running and no ready tasks exist, execution is halted (e.g. deadlock or unresolved rework)
      const hasRunning = this.activeRunningCount > 0;
      const hasReady = this.getTasks().some((t) => t.state === 'ready');
      if (!hasRunning && !hasReady && !this.isComplete) {
        break;
      }
    }
    return this.getSnapshot();
  }
}

/**
 * Factory helper for creating a SpecSimulator instance.
 */
export function createSimulator(
  doc: LiveSpecDoc,
  options?: SimulationOptions,
): SpecSimulator {
  return new SpecSimulator(doc, options);
}
