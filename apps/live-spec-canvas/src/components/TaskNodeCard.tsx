import React, { memo } from 'react';
import { Handle, Position, NodeProps } from '@xyflow/react';
import { SpecTask, TaskState } from '@agentflow/live-spec-core';

export interface TaskNodeData extends Record<string, unknown> {
  task: SpecTask;
  isCritical?: boolean;
  state?: TaskState;
  progress?: number;
  duration?: number;
  failureReason?: string;
  isCycle?: boolean;
  onFaultInject?: (taskId: string) => void;
}

const stateMeta: Record<TaskState, { bg: string; text: string; label: string }> = {
  pending: { bg: '#3f3f46', text: '#e4e4e7', label: '待就绪' },
  ready: { bg: '#2563eb', text: '#ffffff', label: '就绪' },
  executing: { bg: '#d97706', text: '#ffffff', label: '运行中' },
  running: { bg: '#d97706', text: '#ffffff', label: '运行中' },
  submitted: { bg: '#7c3aed', text: '#ffffff', label: '待评审' },
  passed: { bg: '#16a34a', text: '#ffffff', label: '已通过' },
  pass: { bg: '#16a34a', text: '#ffffff', label: '已通过' },
  rework: { bg: '#dc2626', text: '#ffffff', label: '返工 (Rework)' },
  blocked: { bg: '#ea580c', text: '#ffffff', label: '阻塞' },
  cancelled: { bg: '#52525b', text: '#d4d4d8', label: '已取消' },
};

export const TaskNodeCard = memo(({ data, selected }: NodeProps) => {
  const nodeData = data as unknown as TaskNodeData;
  const task = nodeData?.task;
  if (!task) return null;

  const taskId = task.id || (task as unknown as { task_id?: string }).task_id || 'unknown';
  const currentState: TaskState = (nodeData.state || task.state || 'pending') as TaskState;
  const meta = stateMeta[currentState] || stateMeta.pending;
  const isCritical = Boolean(nodeData.isCritical);
  const isCycle = Boolean(nodeData.isCycle);
  const progress = typeof nodeData.progress === 'number' ? nodeData.progress : 0;
  const duration = typeof nodeData.duration === 'number' && nodeData.duration > 0 ? nodeData.duration : (task.estimated_hours ?? 1);
  const progressPercent = Math.min(100, Math.round((progress / duration) * 100));

  let borderColor = 'var(--border, #3f3f46)';
  let boxShadow = '0 2px 8px rgba(0, 0, 0, 0.25)';

  if (isCycle) {
    borderColor = '#ef4444';
    boxShadow = '0 0 14px rgba(239, 68, 68, 0.6)';
  } else if (isCritical) {
    borderColor = '#f97316';
    boxShadow = '0 0 14px rgba(249, 115, 22, 0.55)';
  } else if (selected) {
    borderColor = '#38bdf8';
    boxShadow = '0 0 10px rgba(56, 189, 248, 0.5)';
  }

  return (
    <div
      style={{
        background: 'var(--card, #18181b)',
        color: 'var(--text, #f4f4f5)',
        border: `2px solid ${borderColor}`,
        boxShadow,
        borderRadius: '10px',
        padding: '12px 14px',
        minWidth: '220px',
        maxWidth: '280px',
        fontSize: '12px',
        position: 'relative',
        transition: 'all 0.2s ease',
        cursor: 'pointer',
      }}
    >
      {/* Incoming dependency handle */}
      <Handle
        type="target"
        position={Position.Top}
        style={{
          width: '10px',
          height: '10px',
          background: isCritical ? '#f97316' : '#38bdf8',
          border: '2px solid #ffffff',
        }}
      />

      {/* Header: ID + State Badge */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: '8px',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span
            style={{
              fontWeight: 700,
              fontSize: '11px',
              color: isCritical ? '#f97316' : 'var(--accent, #38bdf8)',
              letterSpacing: '0.02em',
            }}
          >
            {taskId}
          </span>
          {isCritical && (
            <span
              style={{
                fontSize: '9px',
                padding: '1px 5px',
                borderRadius: '3px',
                background: '#f97316',
                color: '#ffffff',
                fontWeight: 700,
              }}
            >
              CPM
            </span>
          )}
          {isCycle && (
            <span
              style={{
                fontSize: '9px',
                padding: '1px 5px',
                borderRadius: '3px',
                background: '#dc2626',
                color: '#ffffff',
                fontWeight: 700,
              }}
            >
              环路
            </span>
          )}
        </div>
        <span
          style={{
            background: meta.bg,
            color: meta.text,
            padding: '2px 7px',
            borderRadius: '12px',
            fontSize: '10px',
            fontWeight: 600,
            lineHeight: 1.3,
            boxShadow: '0 1px 3px rgba(0,0,0,0.2)',
          }}
        >
          {meta.label}
        </span>
      </div>

      {/* Title */}
      <div
        style={{
          fontWeight: 600,
          fontSize: '13px',
          lineHeight: 1.35,
          marginBottom: '8px',
          color: 'var(--text, #f4f4f5)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
        title={task.title}
      >
        {task.title}
      </div>

      {/* Execution details: Worker + Hours */}
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          color: 'var(--subtext, #a1a1aa)',
          fontSize: '11px',
          paddingTop: '6px',
          borderTop: '1px dashed var(--border-subtle, #27272a)',
        }}
      >
        <span
          style={{
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            maxWidth: '130px',
          }}
          title={task.assigned_worker || '未分配'}
        >
          👤 {task.assigned_worker || '未分配'}
        </span>
        <span style={{ fontWeight: 500 }}>⏱ {task.estimated_hours ?? 1}h</span>
      </div>

      {/* Simulation Progress Bar when executing */}
      {(currentState === 'running' || currentState === 'executing') && (
        <div style={{ marginTop: '8px' }}>
          <div
            style={{
              width: '100%',
              height: '4px',
              backgroundColor: 'rgba(255,255,255,0.1)',
              borderRadius: '2px',
              overflow: 'hidden',
            }}
          >
            <div
              style={{
                width: `${progressPercent}%`,
                height: '100%',
                backgroundColor: '#d97706',
                transition: 'width 0.3s ease',
              }}
            />
          </div>
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              fontSize: '9px',
              color: '#d97706',
              marginTop: '2px',
            }}
          >
            <span>进度</span>
            <span>{progress}/{duration}h ({progressPercent}%)</span>
          </div>
        </div>
      )}

      {/* Failure reason if in rework */}
      {currentState === 'rework' && nodeData.failureReason && (
        <div
          style={{
            marginTop: '6px',
            padding: '4px 6px',
            borderRadius: '4px',
            backgroundColor: 'rgba(220, 38, 38, 0.15)',
            border: '1px solid rgba(220, 38, 38, 0.3)',
            color: '#f87171',
            fontSize: '10px',
            lineHeight: 1.3,
          }}
        >
          ⚠️ {nodeData.failureReason}
        </div>
      )}

      {/* Outgoing dependency handle */}
      <Handle
        type="source"
        position={Position.Bottom}
        style={{
          width: '10px',
          height: '10px',
          background: isCritical ? '#f97316' : '#38bdf8',
          border: '2px solid #ffffff',
        }}
      />
    </div>
  );
});

TaskNodeCard.displayName = 'TaskNodeCard';
