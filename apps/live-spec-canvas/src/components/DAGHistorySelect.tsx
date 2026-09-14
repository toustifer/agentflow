import React, { useState, useEffect, useRef, useCallback } from 'react';
import type { LiveSpecDoc } from '@agentflow/live-spec-core';

export interface DagSummary {
  id: string;
  title: string;
  status: string;
  created_at: string;
  total_tasks: number;
  done_tasks: number;
}

export interface DAGHistorySelectProps {
  cwd?: string;
  currentDagId?: string;
  isHistorical?: boolean;
  dags?: DagSummary[];
  onSelectDag: (dagId: string, spec?: LiveSpecDoc) => void;
  onSwitchToLatest?: () => void;
  onHistoryLoaded?: (dags: DagSummary[]) => void;
  onRefresh?: () => void;
}

/**
 * Formats an ISO date string into a user-friendly date format (YYYY-MM-DD HH:mm).
 */
export function formatCreatedAt(isoString?: string): string {
  if (!isoString) return '--';
  try {
    const d = new Date(isoString);
    if (isNaN(d.getTime())) return isoString;
    const year = d.getFullYear();
    const month = String(d.getMonth() + 1).padStart(2, '0');
    const day = String(d.getDate()).padStart(2, '0');
    const hours = String(d.getHours()).padStart(2, '0');
    const minutes = String(d.getMinutes()).padStart(2, '0');
    return `${year}-${month}-${day} ${hours}:${minutes}`;
  } catch {
    return isoString;
  }
}

/**
 * Returns color, background and text label for different DAG statuses.
 */
export function getDagStatusMeta(status: string) {
  const norm = (status || '').toLowerCase();
  if (norm === 'done' || norm === 'completed' || norm === 'pass' || norm === 'passed') {
    return {
      label: '已完成',
      color: '#22c55e',
      bg: 'rgba(34, 197, 94, 0.15)',
      border: 'rgba(34, 197, 94, 0.35)',
    };
  }
  if (norm === 'in_progress' || norm === 'executing' || norm === 'running') {
    return {
      label: '进行中',
      color: '#38bdf8',
      bg: 'rgba(56, 189, 248, 0.15)',
      border: 'rgba(56, 189, 248, 0.35)',
    };
  }
  if (norm === 'rework' || norm === 'failed' || norm === 'blocked') {
    return {
      label: '返工中',
      color: '#ef4444',
      bg: 'rgba(239, 68, 68, 0.15)',
      border: 'rgba(239, 68, 68, 0.35)',
    };
  }
  return {
    label: '规划中',
    color: '#a1a1aa',
    bg: 'rgba(161, 161, 170, 0.15)',
    border: 'rgba(161, 161, 170, 0.35)',
  };
}

export const DAGHistorySelect: React.FC<DAGHistorySelectProps> = ({
  cwd,
  currentDagId,
  isHistorical = false,
  dags: propsDags,
  onSelectDag,
  onSwitchToLatest,
  onHistoryLoaded,
  onRefresh,
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingDagId, setLoadingDagId] = useState<string | null>(null);
  const [localDags, setLocalDags] = useState<DagSummary[]>([]);
  const [fetchError, setFetchError] = useState<string | null>(null);

  const containerRef = useRef<HTMLDivElement>(null);

  // Synchronize local dags if prop provided
  const dags = propsDags ?? localDags;

  // Close dropdown on outside click
  useEffect(() => {
    const handleOutsideClick = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };
    if (isOpen) {
      document.addEventListener('mousedown', handleOutsideClick);
    }
    return () => {
      document.removeEventListener('mousedown', handleOutsideClick);
    };
  }, [isOpen]);

  // Fetch DAGs when cwd changes or refreshed
  const fetchDags = useCallback(
    async (targetCwd?: string) => {
      if (!targetCwd || !targetCwd.trim()) {
        setLocalDags([]);
        setFetchError(null);
        onHistoryLoaded?.([]);
        return;
      }

      setLoading(true);
      setFetchError(null);

      try {
        const resp = await fetch(`/api/agentflow/dags?cwd=${encodeURIComponent(targetCwd)}`);
        if (!resp.ok) {
          throw new Error(`HTTP ${resp.status}: ${resp.statusText}`);
        }
        const data = await resp.json();
        if (data.ok && Array.isArray(data.dags)) {
          setLocalDags(data.dags);
          onHistoryLoaded?.(data.dags);
        } else {
          setFetchError(data.error || '获取历史 DAG 失败');
        }
      } catch (err: any) {
        setFetchError(err.message || String(err));
      } finally {
        setLoading(false);
      }
    },
    [onHistoryLoaded]
  );

  useEffect(() => {
    // Fetch if propsDags not supplied
    if (propsDags === undefined) {
      fetchDags(cwd);
    }
  }, [cwd, propsDags, fetchDags]);

  const handleManualRefresh = () => {
    if (onRefresh) {
      onRefresh();
    }
    fetchDags(cwd);
  };

  // Handle selecting a DAG item
  const handleItemClick = async (dag: DagSummary) => {
    if (loadingDagId) return;
    setLoadingDagId(dag.id);
    setFetchError(null);

    try {
      const url = cwd
        ? `/api/agentflow/dag?cwd=${encodeURIComponent(cwd)}&dag_id=${encodeURIComponent(dag.id)}`
        : `/api/agentflow/dag?dag_id=${encodeURIComponent(dag.id)}`;

      const resp = await fetch(url);
      if (!resp.ok) {
        throw new Error(`加载 DAG [${dag.id}] 失败: HTTP ${resp.status}`);
      }
      const data = await resp.json();
      if (data.ok && data.spec) {
        onSelectDag(dag.id, data.spec);
        setIsOpen(false);
      } else {
        setFetchError(data.error || 'DAG 结构解析异常');
      }
    } catch (err: any) {
      setFetchError(err.message || String(err));
    } finally {
      setLoadingDagId(null);
    }
  };

  if (!cwd && dags.length === 0) {
    return null;
  }

  return (
    <div
      ref={containerRef}
      style={{
        position: 'relative',
        display: 'inline-flex',
        alignItems: 'center',
        gap: '6px',
        zIndex: 50,
      }}
    >
      {/* Historical View Badge & Switch back button */}
      {isHistorical && (
        <div style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>
          <span
            style={{
              fontSize: '11px',
              padding: '3px 8px',
              borderRadius: '6px',
              background: 'rgba(234, 179, 8, 0.15)',
              color: '#eab308',
              border: '1px solid rgba(234, 179, 8, 0.4)',
              fontWeight: 700,
              display: 'flex',
              alignItems: 'center',
              gap: '4px',
            }}
            title="当前正在回看历史战役 DAG 拓扑"
          >
            <span>🔍</span>
            <span>历史回看</span>
          </span>

          {onSwitchToLatest && (
            <button
              onClick={onSwitchToLatest}
              style={{
                fontSize: '11px',
                padding: '3px 8px',
                borderRadius: '6px',
                background: 'rgba(56, 189, 248, 0.15)',
                color: '#38bdf8',
                border: '1px solid rgba(56, 189, 248, 0.35)',
                fontWeight: 600,
                cursor: 'pointer',
                display: 'flex',
                alignItems: 'center',
                gap: '4px',
                transition: 'all 0.15s ease',
              }}
              title="一键切回当前最新的活跃编排"
            >
              <span>⚡</span>
              <span>切回最新</span>
            </button>
          )}
        </div>
      )}

      {/* Dropdown Trigger Button */}
      <button
        onClick={() => setIsOpen((prev) => !prev)}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '6px',
          padding: '4px 10px',
          borderRadius: '6px',
          background: isHistorical
            ? 'rgba(234, 179, 8, 0.12)'
            : isOpen
              ? 'rgba(255, 255, 255, 0.12)'
              : 'rgba(255, 255, 255, 0.06)',
          border: isHistorical
            ? '1px solid rgba(234, 179, 8, 0.35)'
            : '1px solid var(--border-subtle, #3f3f46)',
          color: 'var(--text, #f4f4f5)',
          fontSize: '12px',
          fontWeight: 600,
          cursor: 'pointer',
          transition: 'all 0.2s ease',
          outline: 'none',
        }}
        title="查看该项目的全部历史战役与 DAG 流水线"
      >
        <span>📜</span>
        <span>历史 DAG ({dags.length})</span>
        <span
          style={{
            fontSize: '10px',
            opacity: 0.7,
            transform: isOpen ? 'rotate(180deg)' : 'none',
            transition: 'transform 0.2s',
          }}
        >
          ▼
        </span>
      </button>

      {/* Dropdown Menu Overlay */}
      {isOpen && (
        <div
          style={{
            position: 'absolute',
            top: 'calc(100% + 6px)',
            left: 0,
            width: '380px',
            maxHeight: '440px',
            background: 'var(--card, #18181b)',
            border: '1px solid var(--border-subtle, #3f3f46)',
            borderRadius: '10px',
            boxShadow: '0 12px 32px rgba(0, 0, 0, 0.65)',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
            zIndex: 1000,
          }}
        >
          {/* Header */}
          <div
            style={{
              padding: '10px 14px',
              background: 'rgba(255, 255, 255, 0.03)',
              borderBottom: '1px solid var(--border, #27272a)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
            }}
          >
            <div
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: '6px',
                fontSize: '12px',
                fontWeight: 700,
                color: 'var(--text, #f4f4f5)',
              }}
            >
              <span>📜</span>
              <span>历史战役 DAG 列表</span>
              <span
                style={{
                  fontSize: '11px',
                  fontWeight: 500,
                  color: 'var(--subtext, #a1a1aa)',
                  background: 'rgba(255,255,255,0.06)',
                  padding: '1px 6px',
                  borderRadius: '4px',
                }}
              >
                共 {dags.length} 项
              </span>
            </div>

            <button
              onClick={handleManualRefresh}
              disabled={loading}
              style={{
                background: 'transparent',
                border: 'none',
                color: 'var(--accent, #38bdf8)',
                cursor: loading ? 'not-allowed' : 'pointer',
                fontSize: '11px',
                display: 'flex',
                alignItems: 'center',
                gap: '4px',
                padding: '2px 6px',
                borderRadius: '4px',
              }}
              title="刷新 DAG 列表"
            >
              <span>{loading ? '⏳' : '🔄'}</span>
              <span>刷新</span>
            </button>
          </div>

          {/* Error Message */}
          {fetchError && (
            <div
              style={{
                padding: '8px 14px',
                background: 'rgba(239, 68, 68, 0.15)',
                borderBottom: '1px solid rgba(239, 68, 68, 0.3)',
                color: '#ef4444',
                fontSize: '11px',
              }}
            >
              ⚠️ {fetchError}
            </div>
          )}

          {/* List Content */}
          <div
            style={{
              overflowY: 'auto',
              maxHeight: '380px',
              padding: '6px',
              display: 'flex',
              flexDirection: 'column',
              gap: '4px',
            }}
          >
            {loading && dags.length === 0 ? (
              <div style={{ padding: '24px', textAlign: 'center', color: 'var(--subtext, #a1a1aa)', fontSize: '12px' }}>
                ⏳ 正在加载历史 DAG 流水线...
              </div>
            ) : dags.length === 0 ? (
              <div style={{ padding: '24px', textAlign: 'center', color: 'var(--subtext, #a1a1aa)', fontSize: '12px' }}>
                暂无历史 DAG 记录
              </div>
            ) : (
              dags.map((dag) => {
                const isCurrent = dag.id === currentDagId;
                const statusMeta = getDagStatusMeta(dag.status);
                const isLoadingThis = loadingDagId === dag.id;

                return (
                  <div
                    key={dag.id}
                    onClick={() => handleItemClick(dag)}
                    style={{
                      padding: '10px 12px',
                      borderRadius: '8px',
                      background: isCurrent
                        ? 'rgba(56, 189, 248, 0.1)'
                        : 'rgba(255, 255, 255, 0.02)',
                      border: isCurrent
                        ? '1px solid rgba(56, 189, 248, 0.4)'
                        : '1px solid transparent',
                      cursor: isLoadingThis ? 'wait' : 'pointer',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '6px',
                      transition: 'all 0.15s ease',
                    }}
                    onMouseEnter={(e) => {
                      if (!isCurrent) {
                        e.currentTarget.style.background = 'rgba(255, 255, 255, 0.06)';
                        e.currentTarget.style.borderColor = 'var(--border-subtle, #3f3f46)';
                      }
                    }}
                    onMouseLeave={(e) => {
                      if (!isCurrent) {
                        e.currentTarget.style.background = 'rgba(255, 255, 255, 0.02)';
                        e.currentTarget.style.borderColor = 'transparent';
                      }
                    }}
                  >
                    {/* Top row: Title + Status Badge */}
                    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: '8px' }}>
                      <span
                        style={{
                          fontSize: '13px',
                          fontWeight: 600,
                          color: isCurrent ? 'var(--accent, #38bdf8)' : 'var(--text, #f4f4f5)',
                          lineHeight: '1.4',
                          wordBreak: 'break-word',
                        }}
                      >
                        {isLoadingThis && '⏳ '}{dag.title || dag.id}
                      </span>

                      <span
                        style={{
                          fontSize: '10px',
                          fontWeight: 700,
                          padding: '2px 6px',
                          borderRadius: '4px',
                          background: statusMeta.bg,
                          color: statusMeta.color,
                          border: `1px solid ${statusMeta.border}`,
                          whiteSpace: 'nowrap',
                          flexShrink: 0,
                        }}
                      >
                        {statusMeta.label}
                      </span>
                    </div>

                    {/* Middle row: DAG ID */}
                    <div
                      style={{
                        fontSize: '11px',
                        fontFamily: 'ui-monospace, monospace',
                        color: 'var(--subtext, #a1a1aa)',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      🌿 {dag.id}
                    </div>

                    {/* Bottom row: Progress + Time */}
                    <div
                      style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        fontSize: '11px',
                        color: 'var(--subtext, #a1a1aa)',
                        marginTop: '2px',
                      }}
                    >
                      <span style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
                        <span>📊</span>
                        <span style={{ fontWeight: 500, color: 'var(--text, #f4f4f5)' }}>
                          {dag.done_tasks}/{dag.total_tasks} 完成
                        </span>
                        {dag.total_tasks > 0 && (
                          <span style={{ opacity: 0.7 }}>
                            ({Math.round((dag.done_tasks / dag.total_tasks) * 100)}%)
                          </span>
                        )}
                      </span>

                      <span style={{ opacity: 0.8 }} title={`创建时间: ${dag.created_at}`}>
                        🕒 {formatCreatedAt(dag.created_at)}
                      </span>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </div>
      )}
    </div>
  );
};
