import React from 'react';
import { CPMResult, CycleDetectionResult, LiveSpecDoc } from '@agentflow/live-spec-core';

export interface ParametricSettings {
  concurrency: number;
  autoRetry: boolean;
}

interface ParametricPanelProps {
  spec: LiveSpecDoc;
  cpm: CPMResult;
  cycle: CycleDetectionResult;
  settings: ParametricSettings;
  onSettingsChange: (settings: ParametricSettings) => void;
  selectedTaskId?: string | null;
  onSelectTask?: (taskId: string | null) => void;
  /** Narrow (half-width right pane) layout: shrink the rail without hiding anything. */
  isCompact?: boolean;
}

export const ParametricPanel: React.FC<ParametricPanelProps> = ({
  spec,
  cpm,
  cycle,
  settings,
  onSettingsChange,
  selectedTaskId,
  onSelectTask,
  isCompact = false,
}) => {
  const tasks = spec.tasks || [];

  // Group tasks by worker
  const workerCounts = tasks.reduce<Record<string, number>>((acc, t) => {
    const worker = t.assigned_worker || '未分配';
    acc[worker] = (acc[worker] || 0) + 1;
    return acc;
  }, {});

  return (
    <div
      data-testid="parametric-panel"
      style={{
        // Compact: give ~100px back to the React Flow canvas (714px viewport used
        // to leave only ~414px for the topology). Never below 200px so the panel
        // stays readable, and never display:none.
        width: isCompact ? '208px' : '300px',
        minWidth: isCompact ? '200px' : '280px',
        maxWidth: isCompact ? '220px' : '340px',
        flexShrink: 0,
        height: '100%',
        background: 'var(--panel-bg, #121215)',
        borderRight: '1px solid var(--border, #27272a)',
        display: 'flex',
        flexDirection: 'column',
        fontSize: '12px',
        color: 'var(--text, #f4f4f5)',
        overflowY: 'auto',
        overflowX: 'hidden',
      }}
    >
      {/* Panel Header */}
      <div
        style={{
          padding: isCompact ? '10px 12px' : '14px 16px',
          borderBottom: '1px solid var(--border, #27272a)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: '6px',
        }}
      >
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            fontWeight: 700,
            fontSize: '13px',
            minWidth: 0,
            overflowWrap: 'anywhere',
          }}
        >
          <span style={{ color: 'var(--accent, #38bdf8)', flexShrink: 0 }}>⚙️</span>
          <span>参数推演与分析</span>
        </div>
        <span
          style={{
            fontSize: '11px',
            color: 'var(--subtext, #a1a1aa)',
            background: 'var(--card, #18181b)',
            padding: '2px 6px',
            borderRadius: '4px',
            border: '1px solid var(--border, #27272a)',
            whiteSpace: 'nowrap',
            flexShrink: 0,
          }}
        >
          {tasks.length} 项任务
        </span>
      </div>

      <div
        style={{
          padding: isCompact ? '12px' : '16px',
          display: 'flex',
          flexDirection: 'column',
          gap: '18px',
          minWidth: 0,
        }}
      >
        {/* Cycle Warning Banner */}
        {cycle.hasCycle && (
          <div
            style={{
              padding: '10px 12px',
              borderRadius: '8px',
              backgroundColor: 'rgba(239, 68, 68, 0.15)',
              border: '1px solid #ef4444',
              color: '#fca5a5',
            }}
          >
            <div style={{ fontWeight: 700, display: 'flex', alignItems: 'center', gap: '6px', marginBottom: '4px' }}>
              <span>⛔</span>
              <span>检测到有向环路依赖</span>
            </div>
            <div style={{ fontSize: '11px', lineHeight: 1.4 }}>
              环路链路: {cycle.cycle ? cycle.cycle.join(' → ') : '存在循环引用'}
            </div>
            <div style={{ fontSize: '10px', marginTop: '4px', color: '#f87171' }}>
              提示: 点击拓扑图中红色的连线并按 Backspace/Delete 可删除环路边。
            </div>
          </div>
        )}

        {/* Dynamic Parameter: Concurrency Slider (1-8) */}
        <div
          style={{
            padding: '12px',
            background: 'var(--card, #18181b)',
            borderRadius: '8px',
            border: '1px solid var(--border, #27272a)',
          }}
        >
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              flexWrap: 'wrap',
              gap: '6px',
              marginBottom: '8px',
            }}
          >
            <span style={{ fontWeight: 600, minWidth: 0, overflowWrap: 'anywhere' }}>并发度限制 (Workers)</span>
            <span
              style={{
                background: 'var(--accent, #38bdf8)',
                color: '#09090b',
                fontWeight: 700,
                fontSize: '11px',
                padding: '2px 8px',
                borderRadius: '12px',
                whiteSpace: 'nowrap',
                flexShrink: 0,
              }}
            >
              {settings.concurrency} 并发
            </span>
          </div>
          <input
            type="range"
            min={1}
            max={8}
            step={1}
            value={settings.concurrency}
            onChange={(e) =>
              onSettingsChange({
                ...settings,
                concurrency: Number(e.target.value),
              })
            }
            style={{
              width: '100%',
              accentColor: 'var(--accent, #38bdf8)',
              cursor: 'pointer',
            }}
          />
          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              flexWrap: 'wrap',
              gap: '4px',
              fontSize: '10px',
              color: 'var(--subtext, #a1a1aa)',
              marginTop: '4px',
            }}
          >
            <span>1 (单工串行)</span>
            <span>4 (推荐)</span>
            <span>8 (极速并行)</span>
          </div>
        </div>

        {/* Retry Fault Tolerance Toggle */}
        <div
          style={{
            padding: '12px',
            background: 'var(--card, #18181b)',
            borderRadius: '8px',
            border: '1px solid var(--border, #27272a)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            flexWrap: 'wrap',
            gap: '8px',
          }}
        >
          <div style={{ minWidth: 0, flex: '1 1 auto' }}>
            <div style={{ fontWeight: 600, overflowWrap: 'anywhere' }}>重试容错 (Auto-Retry)</div>
            <div style={{ fontSize: '10px', color: 'var(--subtext, #a1a1aa)', marginTop: '2px', overflowWrap: 'anywhere' }}>
              返工 (rework) 任务自动重试恢复
            </div>
          </div>
          <label style={{ position: 'relative', display: 'inline-block', width: '38px', height: '20px', flexShrink: 0 }}>
            <input
              type="checkbox"
              checked={settings.autoRetry}
              onChange={(e) =>
                onSettingsChange({
                  ...settings,
                  autoRetry: e.target.checked,
                })
              }
              style={{ opacity: 0, width: 0, height: 0 }}
            />
            <span
              style={{
                position: 'absolute',
                cursor: 'pointer',
                top: 0,
                left: 0,
                right: 0,
                bottom: 0,
                backgroundColor: settings.autoRetry ? '#22c55e' : '#3f3f46',
                borderRadius: '20px',
                transition: '0.3s',
              }}
            >
              <span
                style={{
                  position: 'absolute',
                  content: '""',
                  height: '14px',
                  width: '14px',
                  left: settings.autoRetry ? '20px' : '3px',
                  bottom: '3px',
                  backgroundColor: '#ffffff',
                  borderRadius: '50%',
                  transition: '0.3s',
                }}
              />
            </span>
          </label>
        </div>

        {/* Dynamic CPM Calculation Box */}
        <div
          style={{
            padding: '14px',
            background: 'var(--card, #18181b)',
            borderRadius: '8px',
            border: '1px solid var(--border, #27272a)',
            display: 'flex',
            flexDirection: 'column',
            gap: '10px',
          }}
        >
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              flexWrap: 'wrap',
              gap: '6px',
            }}
          >
            <span style={{ fontWeight: 700, color: '#f97316', fontSize: '13px', minWidth: 0, overflowWrap: 'anywhere' }}>
              🔥 CPM 关键路径指标
            </span>
            <span
              style={{
                fontSize: '10px',
                padding: '2px 6px',
                borderRadius: '4px',
                background: cpm.hasCycle ? 'rgba(239, 68, 68, 0.2)' : 'rgba(249, 115, 22, 0.2)',
                color: cpm.hasCycle ? '#ef4444' : '#f97316',
                fontWeight: 600,
                whiteSpace: 'nowrap',
                flexShrink: 0,
              }}
            >
              {cpm.hasCycle ? '不可算 (环路)' : '已推演'}
            </span>
          </div>

          <div
            style={{
              display: 'flex',
              alignItems: 'baseline',
              justifyContent: 'space-between',
              flexWrap: 'wrap',
              gap: '4px',
              padding: '8px 10px',
              borderRadius: '6px',
              background: 'var(--bg, #09090b)',
            }}
          >
            <span style={{ color: 'var(--subtext, #a1a1aa)', minWidth: 0, overflowWrap: 'anywhere' }}>
              预估总工期 (Duration)
            </span>
            <span style={{ fontSize: '16px', fontWeight: 800, color: '#f97316', whiteSpace: 'nowrap' }}>
              {cpm.totalDuration} 小时
            </span>
          </div>

          {/* Bottleneck Tasks */}
          <div>
            <div style={{ color: 'var(--subtext, #a1a1aa)', marginBottom: '4px', fontWeight: 500, overflowWrap: 'anywhere' }}>
              🚨 瓶颈任务 (零浮动时间节点):
            </div>
            {cpm.bottlenecks && cpm.bottlenecks.length > 0 ? (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px', minWidth: 0 }}>
                {cpm.bottlenecks.map((id) => (
                  <button
                    key={id}
                    onClick={() => onSelectTask?.(id)}
                    style={{
                      padding: '2px 6px',
                      fontSize: '10px',
                      borderRadius: '4px',
                      background: selectedTaskId === id ? '#f97316' : 'rgba(249, 115, 22, 0.15)',
                      color: selectedTaskId === id ? '#ffffff' : '#fb923c',
                      border: '1px solid rgba(249, 115, 22, 0.4)',
                      cursor: 'pointer',
                      fontWeight: 600,
                      maxWidth: '100%',
                      overflowWrap: 'anywhere',
                      wordBreak: 'break-all',
                    }}
                  >
                    {id}
                  </button>
                ))}
              </div>
            ) : (
              <div style={{ color: 'var(--subtext, #a1a1aa)', fontStyle: 'italic' }}>无明确瓶颈</div>
            )}
          </div>

          {/* Critical Path Sequence */}
          <div>
            <div style={{ color: 'var(--subtext, #a1a1aa)', marginBottom: '4px', fontWeight: 500, overflowWrap: 'anywhere' }}>
              📍 关键路径链 (Critical Path):
            </div>
            {cpm.criticalPath && cpm.criticalPath.length > 0 ? (
              <div
                style={{
                  padding: '6px 8px',
                  background: 'var(--bg, #09090b)',
                  borderRadius: '4px',
                  fontFamily: 'monospace',
                  fontSize: '11px',
                  color: '#fb923c',
                  lineHeight: 1.4,
                  overflowWrap: 'anywhere',
                  wordBreak: 'break-all',
                  minWidth: 0,
                }}
              >
                {cpm.criticalPath.join(' ➔ ')}
              </div>
            ) : (
              <div style={{ color: 'var(--subtext, #a1a1aa)', fontStyle: 'italic' }}>无依赖链路</div>
            )}
          </div>
        </div>

        {/* Worker Workload Distribution */}
        <div
          style={{
            padding: '12px',
            background: 'var(--card, #18181b)',
            borderRadius: '8px',
            border: '1px solid var(--border, #27272a)',
          }}
        >
          <div style={{ fontWeight: 600, marginBottom: '8px', overflowWrap: 'anywhere' }}>👥 工人负载分布</div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', minWidth: 0 }}>
            {Object.entries(workerCounts).map(([worker, count]) => (
              <div
                key={worker}
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  gap: '6px',
                  fontSize: '11px',
                  minWidth: 0,
                }}
              >
                <span
                  style={{
                    color: 'var(--subtext, #a1a1aa)',
                    minWidth: 0,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    overflowWrap: 'anywhere',
                  }}
                  title={worker}
                >
                  {worker}
                </span>
                <span
                  style={{
                    background: 'var(--bg, #09090b)',
                    padding: '1px 6px',
                    borderRadius: '4px',
                    fontWeight: 600,
                    whiteSpace: 'nowrap',
                    flexShrink: 0,
                  }}
                >
                  {count} 个任务
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
