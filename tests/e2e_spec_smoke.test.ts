import { describe, expect, it } from 'vitest';
import {
  calculateCPM,
  computeSpecDiff,
  createSimulator,
  detectCycle,
  LiveSpecDoc,
  SpecSimulator,
  SpecTask,
} from '@agentflow/live-spec-core';
import {
  extractAllSpecsFromMarkdown,
  extractSpecFromMarkdown,
} from '@agentflow/dsh-interactive-spec';

describe('E2E Live-Spec Full Lifecycle Smoke Test', () => {
  // ---------------------------------------------------------------------------
  // 1. Markdown Ingestion & Tolerant Extraction
  // ---------------------------------------------------------------------------
  const rawMarkdownWithSpec = `
# 架构演进与自动化编排方案

根据团队规划，我们准备自举重构 Agentflow 内核调度引擎。以下是初代 DAG 规划：

\`\`\`agentflow-spec
{
  "version": "1.0.0",
  "dag_id": "dag-core-engine",
  "namespace_id": "default",
  "title": "Agentflow 内核演进 DAG",
  "concurrency": 2,
  "tasks": [
    {
      "id": "task-1",
      "title": "架构设计与接口契约",
      "estimated_hours": 2,
      "priority": 10,
      "assigned_worker": "architect"
    },
    {
      "id": "task-2",
      "title": "前端画布组件开发",
      "estimated_hours": 4,
      "depends_on": ["task-1"],
      "priority": 5,
      "assigned_worker": "fe-dev"
    },
    {
      "id": "task-3",
      "title": "后端调度器与BT引擎改造",
      "estimated_hours": 6,
      "depends_on": ["task-1"],
      "priority": 8,
      "assigned_worker": "be-dev"
    },
    {
      "id": "task-4",
      "title": "前后端联调与协同沙箱",
      "estimated_hours": 3,
      "depends_on": ["task-2", "task-3"],
      "priority": 7,
      "assigned_worker": "agentflow-dev"
    },
    {
      "id": "task-5",
      "title": "全链路验收与门禁发布",
      "estimated_hours": 1,
      "depends_on": ["task-4"],
      "priority": 9,
      "assigned_worker": "qa-lead"
    } // trailing comma and comment
  ]
}
\`\`\`

随后 Leader 对该规划进行了微调更新：

\`\`\`agentflow-spec
{
  "version": "1.1.0",
  "dag_id": "dag-core-engine",
  "namespace_id": "default",
  "title": "Agentflow 内核演进 DAG (v1.1)",
  "concurrency": 2,
  "tasks": [
    {
      "id": "task-1",
      "title": "架构设计与接口契约",
      "estimated_hours": 2,
      "priority": 10,
      "assigned_worker": "architect"
    },
    {
      "id": "task-2",
      "title": "前端画布组件开发",
      "estimated_hours": 4,
      "depends_on": ["task-1"],
      "priority": 5,
      "assigned_worker": "fe-dev"
    },
    {
      "id": "task-3",
      "title": "后端调度器与BT引擎改造",
      "estimated_hours": 6,
      "depends_on": ["task-1"],
      "priority": 8,
      "assigned_worker": "be-dev"
    },
    {
      "id": "task-4",
      "title": "前后端联调与协同沙箱",
      "estimated_hours": 3,
      "depends_on": ["task-2", "task-3"],
      "priority": 7,
      "assigned_worker": "agentflow-dev"
    },
    {
      "id": "task-5",
      "title": "全链路验收与门禁发布",
      "estimated_hours": 1,
      "depends_on": ["task-4"],
      "priority": 9,
      "assigned_worker": "qa-lead"
    }
  ]
}
\`\`\`
`;

  it('1. should extract and parse ```agentflow-spec from Markdown with tolerant cleanup', () => {
    const allSpecs = extractAllSpecsFromMarkdown(rawMarkdownWithSpec);
    expect(allSpecs).toHaveLength(2);

    // Default pick is the latest (last) spec block
    const latestSpec = extractSpecFromMarkdown(rawMarkdownWithSpec);
    expect(latestSpec).not.toBeNull();
    expect(latestSpec?.version).toBe('1.1.0');
    expect(latestSpec?.dag_id).toBe('dag-core-engine');
    expect(latestSpec?.tasks).toHaveLength(5);
    expect(latestSpec?.concurrency).toBe(2);

    // Verify first spec block can also be picked explicitly
    const firstSpec = extractSpecFromMarkdown(rawMarkdownWithSpec, { pick: 'first' });
    expect(firstSpec?.version).toBe('1.0.0');
  });

  // ---------------------------------------------------------------------------
  // 2. Critical Path Method (CPM) & Bottleneck Detection
  // ---------------------------------------------------------------------------
  it('2. should calculate CPM critical path and identify bottleneck nodes accurately', () => {
    const doc = extractSpecFromMarkdown(rawMarkdownWithSpec)!;
    expect(doc).toBeDefined();

    const cpm = calculateCPM(doc);
    expect(cpm.hasCycle).toBe(false);

    // Expected Schedule:
    // task-1: dur=2, ES=0, EF=2, LS=0, LF=2, Slack=0 (Critical)
    // task-2: dur=4, ES=2, EF=6, LS=4, LF=8, Slack=2 (Non-critical)
    // task-3: dur=6, ES=2, EF=8, LS=2, LF=8, Slack=0 (Critical)
    // task-4: dur=3, ES=8, EF=11, LS=8, LF=11, Slack=0 (Critical)
    // task-5: dur=1, ES=11, EF=12, LS=11, LF=12, Slack=0 (Critical)
    expect(cpm.totalDuration).toBe(12);

    // Critical path: task-1 -> task-3 -> task-4 -> task-5
    expect(cpm.criticalPath).toEqual(['task-1', 'task-3', 'task-4', 'task-5']);

    // Nodes inspection
    expect(cpm.nodes['task-2'].slack).toBe(2);
    expect(cpm.nodes['task-2'].isCritical).toBe(false);
    expect(cpm.nodes['task-3'].slack).toBe(0);
    expect(cpm.nodes['task-3'].isCritical).toBe(true);

    // Bottlenecks should be critical nodes sorted by duration descending:
    // task-3 (6h) > task-4 (3h) > task-1 (2h) > task-5 (1h)
    expect(cpm.bottlenecks).toEqual(['task-3', 'task-4', 'task-1', 'task-5']);
    expect(cpm.bottlenecks[0]).toBe('task-3'); // Primary bottleneck
  });

  // ---------------------------------------------------------------------------
  // 3. Topology Modifications & Spec Diff Natural Language Summary
  // ---------------------------------------------------------------------------
  it('3. should track topology modifications and generate natural language feedback summary', () => {
    const baseDoc = extractSpecFromMarkdown(rawMarkdownWithSpec)!;

    // Simulate canvas interactive modifications:
    // a. Remove dependency: task-4 no longer depends on task-2
    // b. Add dependency: task-2 directly connects to task-5
    // c. Adjust duration: optimize bottleneck task-3 from 6h to 3h
    // d. Add new task: task-sec (Security Audit, 2h, depends on task-3)
    // e. Modify global concurrency: 2 -> 3
    const modifiedTasks: SpecTask[] = baseDoc.tasks.map((t) => {
      if (t.id === 'task-3') {
        return { ...t, estimated_hours: 3 };
      }
      if (t.id === 'task-4') {
        // remove task-2 from dependencies
        return { ...t, depends_on: ['task-3'] };
      }
      if (t.id === 'task-5') {
        // add task-2 to dependencies (depends on task-4 and task-2)
        return { ...t, depends_on: ['task-4', 'task-2'] };
      }
      return { ...t };
    });

    const newTaskSec: SpecTask = {
      id: 'task-sec',
      title: '自动化安全门禁审计',
      estimated_hours: 2,
      depends_on: ['task-3'],
      priority: 8,
      assigned_worker: 'secops',
    };
    modifiedTasks.push(newTaskSec);

    const currentDoc: LiveSpecDoc = {
      ...baseDoc,
      concurrency: 3,
      tasks: modifiedTasks,
    };

    const diff = computeSpecDiff(baseDoc, currentDoc);

    expect(diff.hasChanges).toBe(true);

    // 1. Added tasks
    expect(diff.addedTasks).toHaveLength(1);
    expect(diff.addedTasks[0].id).toBe('task-sec');

    // 2. Removed dependencies: task-2 => task-4
    expect(diff.removedDependencies).toEqual([{ from: 'task-2', to: 'task-4' }]);

    // 3. Added dependencies: task-3 => task-sec, task-2 => task-5
    expect(diff.addedDependencies).toEqual(
      expect.arrayContaining([
        { from: 'task-2', to: 'task-5' },
        { from: 'task-3', to: 'task-sec' },
      ])
    );

    // 4. Modified tasks: task-3 estimated_hours changed 6 -> 3
    const task3Diff = diff.modifiedTasks.find((m) => m.id === 'task-3');
    expect(task3Diff).toBeDefined();
    expect(task3Diff?.changes).toEqual(
      expect.arrayContaining([
        { field: 'estimated_hours', oldValue: 6, newValue: 3 },
      ])
    );

    // 5. Parameter changes: concurrency 2 -> 3
    expect(diff.parameterChanges).toEqual([
      { field: 'concurrency', oldValue: 2, newValue: 3 },
    ]);

    // 6. Natural Language Feedback Summary
    expect(diff.summary).toContain('Live-Spec 拓扑与参数变动反哺摘要');
    expect(diff.summary).toContain('新增任务');
    expect(diff.summary).toContain('task-sec');
    expect(diff.summary).toContain('自动化安全门禁审计');
    expect(diff.summary).toContain('拓扑依赖调整');
    expect(diff.summary).toContain('`task-2` ➔ `task-5`');
    expect(diff.summary).toContain('`task-2` ➔ `task-4`');
    expect(diff.summary).toContain('CPM 工期与瓶颈影响');
    expect(diff.summary).toContain('项目总工期');
  });

  // ---------------------------------------------------------------------------
  // 4. SpecSimulator State Machine Stepping & Concurrency Throttling
  // ---------------------------------------------------------------------------
  it('4. should step through SpecSimulator, enforce concurrency limits and test fault injection', () => {
    // Construct a DAG with 3 initial root tasks to test concurrency limit = 2
    const testDoc: LiveSpecDoc = {
      version: '1.0.0',
      dag_id: 'test-sim-dag',
      concurrency: 2,
      tasks: [
        { id: 'r1', title: 'Root 1', estimated_hours: 1, priority: 10 },
        { id: 'r2', title: 'Root 2', estimated_hours: 1, priority: 5 },
        { id: 'r3', title: 'Root 3', estimated_hours: 1, priority: 1 },
        { id: 'tail', title: 'Tail', estimated_hours: 1, depends_on: ['r1', 'r2', 'r3'] },
      ],
    };

    const sim = createSimulator(testDoc);
    expect(sim.clock).toBe(0);
    expect(sim.activeRunningCount).toBe(0);
    expect(sim.isComplete).toBe(false);

    // Tick 1: Root tasks are ready, but concurrency=2 => only r1 (p=10) and r2 (p=5) should run
    const snap1 = sim.tick();
    expect(sim.clock).toBe(1);
    expect(snap1.activeRunningCount).toBe(2);
    expect(snap1.tasks['r1'].state).toBe('running');
    expect(snap1.tasks['r2'].state).toBe('running');
    expect(snap1.tasks['r3'].state).toBe('ready'); // throttled by concurrency!
    expect(snap1.tasks['tail'].state).toBe('pending');

    // Tick 2: r1 and r2 progress=2 => become 'pass'
    // Concurrency slots freed. r3 (ready) moves into running
    const snap2 = sim.tick();
    expect(sim.clock).toBe(2);
    expect(snap2.tasks['r1'].state).toBe('pass');
    expect(snap2.tasks['r2'].state).toBe('pass');
    expect(snap2.tasks['r3'].state).toBe('running');
    expect(snap2.activeRunningCount).toBe(1);

    // Test Fault Injection: Inject rework failure into r3
    const injected = sim.injectFailure('r3', 'Database connection refused during r3');
    expect(injected).toBe(true);
    expect(sim.getTask('r3')?.state).toBe('rework');
    expect(sim.getTask('r3')?.failureReason).toBe('Database connection refused during r3');
    expect(sim.activeRunningCount).toBe(0);

    // Retry the rework task
    const retried = sim.retryTask('r3');
    expect(retried).toBe(true);
    expect(sim.getTask('r3')?.state).toBe('ready');

    // Continue simulation until complete
    const finalSnap = sim.runUntilComplete(20);
    expect(finalSnap.isComplete).toBe(true);
    expect(finalSnap.tasks['r1'].state).toBe('pass');
    expect(finalSnap.tasks['r2'].state).toBe('pass');
    expect(finalSnap.tasks['r3'].state).toBe('pass');
    expect(finalSnap.tasks['tail'].state).toBe('pass');
    expect(sim.activeRunningCount).toBe(0);
  });

  // ---------------------------------------------------------------------------
  // 5. Cycle Detection and Ingestion Gatekeeping
  // ---------------------------------------------------------------------------
  it('5. should detect directed cycles and gatekeep/block ingestion into store', () => {
    // Create an invalid cyclic DAG: A -> B -> C -> A
    const cyclicDoc: LiveSpecDoc = {
      version: '1.0.0',
      dag_id: 'cyclic-danger',
      tasks: [
        { id: 'task-A', title: 'Task A', estimated_hours: 1, depends_on: ['task-C'] },
        { id: 'task-B', title: 'Task B', estimated_hours: 1, depends_on: ['task-A'] },
        { id: 'task-C', title: 'Task C', estimated_hours: 1, depends_on: ['task-B'] },
      ],
    };

    // 1. Cycle detection direct check
    const cycleRes = detectCycle(cyclicDoc);
    expect(cycleRes.hasCycle).toBe(true);
    expect(cycleRes.cycles.length).toBeGreaterThanOrEqual(1);
    expect(cycleRes.cycle).toBeDefined();
    // Normalized canonical cycle: ['task-A', 'task-B', 'task-C', 'task-A']
    expect(cycleRes.cycle).toEqual(['task-A', 'task-B', 'task-C', 'task-A']);

    // 2. CPM calculation aborts on cycle
    const cpmRes = calculateCPM(cyclicDoc);
    expect(cpmRes.hasCycle).toBe(true);
    expect(cpmRes.error).toContain('Cannot calculate CPM: cycle detected');

    // 3. Simulated Ingestion Gatekeeper
    // When saving or ingesting to store, cycle detection acts as a guard.
    class CyclicDependencyError extends Error {
      constructor(cyclePath: string[]) {
        super(`[Gatekeeper Blocked] Cyclic dependency detected in spec: ${cyclePath.join(' -> ')}`);
        this.name = 'CyclicDependencyError';
      }
    }

    function ingestSpecToStore(doc: LiveSpecDoc): { success: boolean; id: string } {
      const check = detectCycle(doc);
      if (check.hasCycle) {
        throw new CyclicDependencyError(check.cycle || []);
      }
      return { success: true, id: doc.dag_id || 'unnamed' };
    }

    // Gatekeeper must throw CyclicDependencyError and block storage
    expect(() => ingestSpecToStore(cyclicDoc)).toThrowError(CyclicDependencyError);
    expect(() => ingestSpecToStore(cyclicDoc)).toThrowError(/task-A -> task-B -> task-C -> task-A/);

    // Valid spec passes through gatekeeper
    const validDoc: LiveSpecDoc = {
      version: '1.0.0',
      dag_id: 'clean-dag',
      tasks: [
        { id: 'task-1', title: 'Step 1' },
        { id: 'task-2', title: 'Step 2', depends_on: ['task-1'] },
      ],
    };
    expect(ingestSpecToStore(validDoc)).toEqual({ success: true, id: 'clean-dag' });
  });
});
