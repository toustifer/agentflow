import React, { useMemo, useCallback, useEffect, useState } from 'react';
import {
  ReactFlow,
  Background,
  Controls,
  applyNodeChanges,
  applyEdgeChanges,
  Connection,
  Edge,
  Node,
  NodeChange,
  EdgeChange,
  MarkerType,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import { TaskNodeCard } from './TaskNodeCard';
import {
  LiveSpecDoc,
  CPMResult,
  CycleDetectionResult,
  SimulatedTaskState,
  SpecTask,
} from '@agentflow/live-spec-core';

interface TopologyProps {
  spec: LiveSpecDoc;
  cpm: CPMResult;
  cycle: CycleDetectionResult;
  simTasks?: Record<string, SimulatedTaskState>;
  selectedTaskId?: string | null;
  onSelectTask?: (taskId: string | null) => void;
  onSpecChange: (newSpec: LiveSpecDoc) => void;
  onFaultInject?: (taskId: string) => void;
  /** Narrow (half-width right pane) layout: compact the floating legend so it stops covering nodes. */
  isCompact?: boolean;
}

const nodeTypes = { taskNode: TaskNodeCard };

function computeLayout(tasks: SpecTask[]): Map<string, { x: number; y: number }> {
  const levelMap = new Map<string, number>();
  const taskMap = new Map<string, SpecTask>();
  tasks.forEach((t) => taskMap.set(t.id, t));

  const getLevel = (id: string, visited = new Set<string>()): number => {
    if (levelMap.has(id)) return levelMap.get(id)!;
    if (visited.has(id)) return 0;
    visited.add(id);

    const t = taskMap.get(id);
    if (!t || !t.depends_on || t.depends_on.length === 0) {
      levelMap.set(id, 0);
      return 0;
    }
    let maxDepLevel = 0;
    for (const depId of t.depends_on) {
      maxDepLevel = Math.max(maxDepLevel, getLevel(depId, new Set(visited)) + 1);
    }
    levelMap.set(id, maxDepLevel);
    return maxDepLevel;
  };

  tasks.forEach((t) => getLevel(t.id));

  const levels: Record<number, string[]> = {};
  tasks.forEach((t) => {
    const lvl = levelMap.get(t.id) ?? 0;
    if (!levels[lvl]) levels[lvl] = [];
    levels[lvl].push(t.id);
  });

  const positions = new Map<string, { x: number; y: number }>();
  Object.entries(levels).forEach(([lvlStr, ids]) => {
    const lvl = Number(lvlStr);
    const totalInLevel = ids.length;
    ids.forEach((id, idx) => {
      // Center the nodes on each level
      const x = 300 * idx - ((totalInLevel - 1) * 300) / 2 + 300;
      const y = 170 * lvl + 60;
      positions.set(id, { x, y });
    });
  });

  return positions;
}

export const TopologyCanvas: React.FC<TopologyProps> = ({
  spec,
  cpm,
  cycle,
  simTasks,
  selectedTaskId,
  onSelectTask,
  onSpecChange,
  onFaultInject,
  isCompact = false,
}) => {
  const tasks = spec.tasks || [];
  const criticalSet = useMemo(() => new Set(cpm.criticalPath || []), [cpm]);

  // Set of nodes involved in directed cycle
  const cycleNodesSet = useMemo(() => {
    const set = new Set<string>();
    if (cycle.hasCycle) {
      if (cycle.cycle) cycle.cycle.forEach((id) => set.add(id));
      if (cycle.cycles) cycle.cycles.forEach((c) => c.forEach((id) => set.add(id)));
    }
    return set;
  }, [cycle]);

  // Layout cache
  const initialPositions = useMemo(() => computeLayout(tasks), [tasks.length]);

  // Internal nodes and edges state
  const [nodes, setNodes] = useState<Node[]>([]);
  const [edges, setEdges] = useState<Edge[]>([]);

  // Helper to determine if an edge is part of a cycle
  const isEdgeInCycle = useCallback(
    (source: string, target: string): boolean => {
      if (!cycle.hasCycle) return false;
      if (cycle.cycle) {
        for (let i = 0; i < cycle.cycle.length - 1; i++) {
          if (cycle.cycle[i] === source && cycle.cycle[i + 1] === target) return true;
        }
      }
      if (cycle.cycles) {
        for (const c of cycle.cycles) {
          for (let i = 0; i < c.length - 1; i++) {
            if (c[i] === source && c[i + 1] === target) return true;
          }
        }
      }
      return false;
    },
    [cycle]
  );

  // Helper to determine if an edge is critical
  const isEdgeCritical = useCallback(
    (source: string, target: string): boolean => {
      if (cycle.hasCycle) return false;
      if (cpm.criticalPath && cpm.criticalPath.length > 1) {
        for (let i = 0; i < cpm.criticalPath.length - 1; i++) {
          if (cpm.criticalPath[i] === source && cpm.criticalPath[i + 1] === target) return true;
        }
      }
      if (cpm.criticalPaths) {
        for (const cp of cpm.criticalPaths) {
          for (let i = 0; i < cp.length - 1; i++) {
            if (cp[i] === source && cp[i + 1] === target) return true;
          }
        }
      }
      return false;
    },
    [cpm, cycle]
  );

  // Sync nodes when tasks, simTasks, cpm, cycle, or selection changes
  useEffect(() => {
    setNodes((prevNodes) => {
      const prevPosMap = new Map<string, { x: number; y: number }>();
      prevNodes.forEach((n) => prevPosMap.set(n.id, n.position));

      return tasks.map((t) => {
        const existingPos = prevPosMap.get(t.id);
        const autoPos = initialPositions.get(t.id) || { x: 100, y: 100 };
        const position = existingPos || autoPos;
        const simTask = simTasks?.[t.id];

        return {
          id: t.id,
          type: 'taskNode',
          position,
          selected: selectedTaskId === t.id,
          data: {
            task: t,
            isCritical: criticalSet.has(t.id),
            isCycle: cycleNodesSet.has(t.id),
            state: simTask?.state ?? t.state ?? 'pending',
            progress: simTask?.progress,
            duration: simTask?.duration,
            failureReason: simTask?.failureReason,
            onFaultInject,
          },
        };
      });
    });
  }, [tasks, simTasks, criticalSet, cycleNodesSet, selectedTaskId, initialPositions, onFaultInject]);

  // Sync edges when tasks, cpm, or cycle changes
  useEffect(() => {
    const edgeList: Edge[] = [];
    tasks.forEach((t) => {
      const deps = t.depends_on || [];
      deps.forEach((depId) => {
        const inCycle = isEdgeInCycle(depId, t.id);
        const critical = isEdgeCritical(depId, t.id);

        let stroke = '#64748b';
        let strokeWidth = 1.5;
        let animated = false;
        let markerColor = '#64748b';
        let label = undefined;

        if (inCycle) {
          stroke = '#ef4444';
          strokeWidth = 3;
          animated = true;
          markerColor = '#ef4444';
          label = '⛔ 环路';
        } else if (critical) {
          stroke = '#f97316';
          strokeWidth = 3;
          animated = true;
          markerColor = '#f97316';
        }

        edgeList.push({
          id: `e-${depId}-${t.id}`,
          source: depId,
          target: t.id,
          animated,
          label,
          labelStyle: inCycle ? { fill: '#ef4444', fontWeight: 700, fontSize: '10px' } : undefined,
          style: {
            stroke,
            strokeWidth,
          },
          markerEnd: {
            type: MarkerType.ArrowClosed,
            color: markerColor,
            width: 14,
            height: 14,
          },
        });
      });
    });
    setEdges(edgeList);
  }, [tasks, isEdgeInCycle, isEdgeCritical]);

  // React Flow changes
  const onNodesChange = useCallback(
    (changes: NodeChange[]) => setNodes((nds) => applyNodeChanges(changes, nds)),
    []
  );

  const onEdgesChange = useCallback(
    (changes: EdgeChange[]) => setEdges((eds) => applyEdgeChanges(changes, eds)),
    []
  );

  // Drag and connect to add dependency edge
  const onConnect = useCallback(
    (params: Connection) => {
      if (!params.source || !params.target || params.source === params.target) return;
      const sourceId = params.source;
      const targetId = params.target;

      const updatedTasks = tasks.map((t) => {
        if (t.id === targetId) {
          const currentDeps = t.depends_on ? [...t.depends_on] : [];
          if (!currentDeps.includes(sourceId)) {
            return { ...t, depends_on: [...currentDeps, sourceId] };
          }
        }
        return t;
      });

      onSpecChange({ ...spec, tasks: updatedTasks });
    },
    [tasks, spec, onSpecChange]
  );

  // Delete dependency edge
  const onEdgesDelete = useCallback(
    (deletedEdges: Edge[]) => {
      const updatedTasks = tasks.map((t) => {
        if (!t.depends_on || t.depends_on.length === 0) return t;
        const remainingDeps = t.depends_on.filter((depId) => {
          return !deletedEdges.some((e) => e.source === depId && e.target === t.id);
        });
        return { ...t, depends_on: remainingDeps };
      });

      onSpecChange({ ...spec, tasks: updatedTasks });
    },
    [tasks, spec, onSpecChange]
  );

  const onNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      onSelectTask?.(node.id);
    },
    [onSelectTask]
  );

  const onPaneClick = useCallback(() => {
    onSelectTask?.(null);
  }, [onSelectTask]);

  return (
    <div style={{ flex: 1, width: '100%', height: '100%', position: 'relative' }}>
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onEdgesDelete={onEdgesDelete}
        onNodeClick={onNodeClick}
        onPaneClick={onPaneClick}
        fitView
        minZoom={0.2}
        maxZoom={2}
      >
        <Background gap={20} size={1} color="var(--border-subtle, #27272a)" />
        <Controls />
      </ReactFlow>

      {/* Floating Canvas Quick Hint (all three hints always rendered — compact mode
          only shrinks/stacks them into a narrow corner block so they stop blanketing
          the task nodes) */}
      <div
        data-testid="canvas-quick-hint"
        style={{
          position: 'absolute',
          // 20px clears React Flow's own bottom-right attribution link.
          bottom: isCompact ? '20px' : '16px',
          right: isCompact ? '8px' : '16px',
          maxWidth: isCompact ? 'none' : 'calc(100% - 32px)',
          background: 'rgba(24, 24, 27, 0.85)',
          backdropFilter: 'blur(8px)',
          border: '1px solid var(--border, #27272a)',
          borderRadius: '6px',
          padding: isCompact ? '4px 8px' : '6px 12px',
          fontSize: isCompact ? '10px' : '11px',
          lineHeight: 1.4,
          color: 'var(--subtext, #a1a1aa)',
          pointerEvents: 'none',
          display: 'flex',
          flexDirection: isCompact ? 'column' : 'row',
          alignItems: isCompact ? 'flex-start' : 'center',
          gap: isCompact ? '2px' : '12px',
          boxSizing: 'border-box',
        }}
      >
        <span>💡 拖拽端点可连接依赖</span>
        <span>选中连线按 Backspace 可删除</span>
        <span>拖拽卡片可自定布局</span>
      </div>
    </div>
  );
};
