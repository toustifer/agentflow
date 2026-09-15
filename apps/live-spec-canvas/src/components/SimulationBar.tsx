import React from 'react';
import { SimulationSnapshot, SpecTask } from '@agentflow/live-spec-core';

export interface SimulationBarProps {
  isPlaying: boolean;
  onTogglePlay: () => void;
  onStep: () => void;
  onReset: () => void;
  onInjectFault: (taskId?: string) => void;
  snapshot: SimulationSnapshot | null;
  tasks: SpecTask[];
  selectedTaskId?: string | null;
  speedMs: number;
  onSpeedChange: (speed: number) => void;
  /** Narrow (half-width right pane) layout: wrap into 2 rows instead of clipping. */
  isCompact?: boolean;
}

export const SimulationBar: React.FC<SimulationBarProps> = ({
  isPlaying,
  onTogglePlay,
  onStep,
  onReset,
  onInjectFault,
  snapshot,
  tasks,
  selectedTaskId,
  speedMs,
  onSpeedChange,
  isCompact = false,
}) => {
  const clock = snapshot?.clock ?? 0;
  const runningCount = snapshot?.activeRunningCount ?? 0;
  const isComplete = snapshot?.isComplete ?? false;
  const concurrency = snapshot?.concurrency ?? 4;

  // Find candidate tasks for chaos injection
  const runningTasks = Object.values(snapshot?.tasks || {}).filter(
    (t) => t.state === 'running' || t.state === 'executing'
  );
  const passedTasks = Object.values(snapshot?.tasks || {}).filter(
    (t) => t.state === 'pass' || t.state === 'passed'
  );

  const handleInjectFaultClick = () => {
    if (selectedTaskId) {
      onInjectFault(selectedTaskId);
      return;
    }
    // Pick running task first, then passed task, or first task
    if (runningTasks.length > 0) {
      onInjectFault(runningTasks[0].id);
    } else if (passedTasks.length > 0) {
      onInjectFault(passedTasks[0].id);
    } else if (tasks.length > 0) {
      onInjectFault(tasks[0].id);
    }
  };

  return (
    <div
      data-testid="simulation-bar"
      style={{
        // minHeight + wrap: at ~714px the three groups no longer fit on one row,
        // so the bar grows to two rows instead of clipping its buttons.
        minHeight: '56px',
        height: 'auto',
        flex: '0 0 auto',
        background: 'var(--card, #18181b)',
        borderTop: '1px solid var(--border, #27272a)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        flexWrap: 'wrap',
        padding: isCompact ? '8px 12px' : '0 20px',
        rowGap: isCompact ? '8px' : '0px',
        columnGap: '12px',
        fontSize: '12px',
        color: 'var(--text, #f4f4f5)',
        zIndex: 10,
        boxShadow: '0 -2px 10px rgba(0,0,0,0.15)',
        boxSizing: 'border-box',
      }}
    >
      {/* Left: Clock and Slot Status */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: isCompact ? '10px' : '16px',
          flexShrink: 0,
          whiteSpace: 'nowrap',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span style={{ fontSize: '14px' }}>⏱️</span>
          <span
            style={{
              fontWeight: 700,
              fontSize: isCompact ? '12px' : '13px',
              fontFamily: 'monospace',
              whiteSpace: 'nowrap',
            }}
          >
            时钟: T+{clock}h
          </span>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: '6px' }}>
          <span style={{ color: 'var(--subtext, #a1a1aa)' }}>并行槽位:</span>
          <span
            style={{
              padding: '2px 8px',
              borderRadius: '10px',
              background: runningCount > 0 ? 'rgba(56, 189, 248, 0.2)' : 'var(--bg, #09090b)',
              color: runningCount > 0 ? '#38bdf8' : 'var(--subtext, #a1a1aa)',
              fontWeight: 700,
              fontFamily: 'monospace',
              whiteSpace: 'nowrap',
            }}
          >
            {runningCount} / {concurrency} 活跃
          </span>
        </div>

        {isComplete && (
          <span
            style={{
              background: '#16a34a',
              color: '#ffffff',
              padding: '2px 8px',
              borderRadius: '12px',
              fontSize: '11px',
              fontWeight: 700,
              whiteSpace: 'nowrap',
            }}
          >
            ✓ 全部任务完成
          </span>
        )}
      </div>

      {/* Center: Playback Controls */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: isCompact ? '8px' : '10px',
          flexShrink: 0,
          whiteSpace: 'nowrap',
        }}
      >
        {/* Play/Pause Button */}
        <button
          onClick={onTogglePlay}
          disabled={isComplete}
          style={{
            padding: isCompact ? '5px 9px' : '6px 14px',
            fontSize: isCompact ? '11px' : '12px',
            fontWeight: 600,
            borderRadius: '6px',
            cursor: isComplete ? 'not-allowed' : 'pointer',
            background: isPlaying ? '#ea580c' : '#0284c7',
            color: '#ffffff',
            border: 'none',
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            transition: 'background 0.2s',
            opacity: isComplete ? 0.5 : 1,
            flexShrink: 0,
            whiteSpace: 'nowrap',
          }}
        >
          <span>{isPlaying ? '⏸️ 暂停' : '▶️ 播放回放'}</span>
        </button>

        {/* Step Forward Button */}
        <button
          onClick={onStep}
          disabled={isPlaying || isComplete}
          style={{
            padding: isCompact ? '5px 9px' : '6px 12px',
            fontSize: isCompact ? '11px' : '12px',
            fontWeight: 500,
            borderRadius: '6px',
            cursor: isPlaying || isComplete ? 'not-allowed' : 'pointer',
            background: 'var(--bg, #09090b)',
            color: 'var(--text, #f4f4f5)',
            border: '1px solid var(--border, #27272a)',
            display: 'flex',
            alignItems: 'center',
            gap: '4px',
            opacity: isPlaying || isComplete ? 0.5 : 1,
            flexShrink: 0,
            whiteSpace: 'nowrap',
          }}
        >
          <span>⏭️ 单步推进 (+1h)</span>
        </button>

        {/* Reset Button */}
        <button
          onClick={onReset}
          style={{
            padding: isCompact ? '5px 9px' : '6px 12px',
            fontSize: isCompact ? '11px' : '12px',
            fontWeight: 500,
            borderRadius: '6px',
            cursor: 'pointer',
            background: 'var(--bg, #09090b)',
            color: 'var(--text, #f4f4f5)',
            border: '1px solid var(--border, #27272a)',
            display: 'flex',
            alignItems: 'center',
            gap: '4px',
            flexShrink: 0,
            whiteSpace: 'nowrap',
          }}
        >
          <span>🔄 重置</span>
        </button>

        {/* Speed Selector */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '4px',
            marginLeft: isCompact ? '2px' : '6px',
            flexShrink: 0,
            whiteSpace: 'nowrap',
          }}
        >
          <span style={{ fontSize: '11px', color: 'var(--subtext, #a1a1aa)', whiteSpace: 'nowrap' }}>速率:</span>
          {[
            { label: '0.5x', ms: 1500 },
            { label: '1x', ms: 800 },
            { label: '2x', ms: 400 },
          ].map((s) => (
            <button
              key={s.ms}
              onClick={() => onSpeedChange(s.ms)}
              style={{
                padding: '2px 6px',
                fontSize: '10px',
                borderRadius: '4px',
                background: speedMs === s.ms ? 'var(--accent, #38bdf8)' : 'var(--bg, #09090b)',
                color: speedMs === s.ms ? '#09090b' : 'var(--text, #f4f4f5)',
                border: '1px solid var(--border, #27272a)',
                cursor: 'pointer',
                fontWeight: speedMs === s.ms ? 700 : 400,
                flexShrink: 0,
                whiteSpace: 'nowrap',
              }}
            >
              {s.label}
            </button>
          ))}
        </div>
      </div>

      {/* Right: Chaos Engineering Fault Injection */}
      <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexShrink: 0, whiteSpace: 'nowrap' }}>
        <button
          onClick={handleInjectFaultClick}
          title="随机或对当前选中任务注入故障，令其进入 rework 返工状态"
          style={{
            padding: isCompact ? '5px 9px' : '6px 12px',
            fontSize: isCompact ? '11px' : '12px',
            fontWeight: 600,
            borderRadius: '6px',
            cursor: 'pointer',
            background: 'rgba(220, 38, 38, 0.15)',
            color: '#ef4444',
            border: '1px solid #dc2626',
            display: 'flex',
            alignItems: 'center',
            gap: '6px',
            transition: 'all 0.2s ease',
            flexShrink: 0,
            whiteSpace: 'nowrap',
          }}
        >
          <span>💥 混沌故障注入 (触发 Rework)</span>
        </button>
      </div>
    </div>
  );
};
