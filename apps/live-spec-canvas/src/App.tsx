import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import {
  LiveSpecDoc,
  calculateCPM,
  computeSpecDiff,
  detectCycle,
  SpecSimulator,
  SimulationSnapshot,
} from '@agentflow/live-spec-core';
import { TopologyCanvas } from './components/TopologyCanvas';
import { ParametricPanel, ParametricSettings } from './components/ParametricPanel';
import { SimulationBar } from './components/SimulationBar';
import { childBridge, ThemeMode } from './bridge/child-bridge';

const defaultSampleSpec: LiveSpecDoc = {
  version: '1.0.0',
  title: 'Agentflow 自举流水线 (Live-Spec)',
  dag_id: 'bootstrap-pipeline',
  concurrency: 3,
  tasks: [
    {
      id: 'task-1-core',
      title: '核心算法库 live-spec-core',
      assigned_worker: 'agentflow-dev',
      depends_on: [],
      estimated_hours: 2,
      priority: 10,
      state: 'passed',
    },
    {
      id: 'task-2-bridge',
      title: '双向通信 ChildBridge 协议',
      assigned_worker: 'agentflow-dev',
      depends_on: ['task-1-core'],
      estimated_hours: 2,
      priority: 8,
      state: 'passed',
    },
    {
      id: 'task-3-canvas',
      title: 'React Flow 画布前端应用',
      assigned_worker: 'agentflow-dev',
      depends_on: ['task-2-bridge'],
      estimated_hours: 3,
      priority: 9,
      state: 'executing',
    },
    {
      id: 'task-4-plugin',
      title: 'DSH 伴生插件嵌入',
      assigned_worker: 'agentflow-dev',
      depends_on: ['task-3-canvas'],
      estimated_hours: 2,
      priority: 5,
      state: 'pending',
    },
    {
      id: 'task-5-gate',
      title: '四重门禁验收与打包交付',
      assigned_worker: 'agentflow-dev',
      depends_on: ['task-3-canvas', 'task-4-plugin'],
      estimated_hours: 1,
      priority: 6,
      state: 'pending',
    },
  ],
};

export const App: React.FC = () => {
  const [originalSpec, setOriginalSpec] = useState<LiveSpecDoc>(defaultSampleSpec);
  const [currentSpec, setCurrentSpec] = useState<LiveSpecDoc>(defaultSampleSpec);
  const [selectedTaskId, setSelectedTaskId] = useState<string | null>(null);
  const [theme, setTheme] = useState<ThemeMode>('dark');
  const [toastMessage, setToastMessage] = useState<string | null>(null);

  // Simulation settings
  const [settings, setSettings] = useState<ParametricSettings>({
    concurrency: defaultSampleSpec.concurrency ?? 3,
    autoRetry: true,
  });
  const [speedMs, setSpeedMs] = useState<number>(800);
  const [isPlaying, setIsPlaying] = useState<boolean>(false);
  const [snapshot, setSnapshot] = useState<SimulationSnapshot | null>(null);

  // Simulator instance ref
  const simulatorRef = useRef<SpecSimulator | null>(null);
  const timerRef = useRef<number | null>(null);

  // Initialize and synchronize simulator
  const initSimulator = useCallback((doc: LiveSpecDoc, currentSettings: ParametricSettings) => {
    const sim = new SpecSimulator(doc, {
      concurrency: currentSettings.concurrency,
      autoRetry: currentSettings.autoRetry,
    });
    simulatorRef.current = sim;
    setSnapshot(sim.getSnapshot());
  }, []);

  // Initialize simulator on mount
  useEffect(() => {
    initSimulator(currentSpec, settings);
  }, []);

  // Update simulator when settings change
  const handleSettingsChange = (newSettings: ParametricSettings) => {
    setSettings(newSettings);
    if (simulatorRef.current) {
      simulatorRef.current.setConcurrency(newSettings.concurrency);
      setSnapshot(simulatorRef.current.getSnapshot());
    }
  };

  // Toast auto-clear
  useEffect(() => {
    if (!toastMessage) return;
    const timer = window.setTimeout(() => setToastMessage(null), 3500);
    return () => window.clearTimeout(timer);
  }, [toastMessage]);

  // Sync theme with body class
  useEffect(() => {
    if (typeof document !== 'undefined') {
      document.body.className = theme === 'dark' ? 'theme-dark' : 'theme-light';
    }
  }, [theme]);

  // ChildBridge registration
  useEffect(() => {
    const unbindMount = childBridge.onMount((incomingSpec) => {
      setOriginalSpec(incomingSpec);
      setCurrentSpec(incomingSpec);
      const newSettings: ParametricSettings = {
        concurrency: incomingSpec.concurrency ?? incomingSpec.settings?.concurrency ?? 3,
        autoRetry: true,
      };
      setSettings(newSettings);
      initSimulator(incomingSpec, newSettings);
      setToastMessage(`✓ 已接收并挂载 Live-Spec: ${incomingSpec.title || incomingSpec.dag_id}`);
    });

    const unbindPatch = childBridge.onPatch((payload) => {
      if (payload.spec) {
        setCurrentSpec(payload.spec);
        initSimulator(payload.spec, settings);
        setToastMessage('✓ 已合并增量补丁 SPEC_PATCH');
      }
    });

    const unbindTheme = childBridge.onTheme((newTheme) => {
      setTheme(newTheme);
    });

    // Notify host that canvas is ready
    childBridge.signalReady();

    return () => {
      unbindMount();
      unbindPatch();
      unbindTheme();
      if (timerRef.current) {
        window.clearInterval(timerRef.current);
      }
    };
  }, [initSimulator, settings]);

  // Dynamic calculations via live-spec-core
  const cpm = useMemo(() => calculateCPM(currentSpec.tasks), [currentSpec.tasks]);
  const cycle = useMemo(() => detectCycle(currentSpec.tasks), [currentSpec.tasks]);
  const diff = useMemo(() => computeSpecDiff(originalSpec, currentSpec), [originalSpec, currentSpec]);

  // Handle DAG spec changes from canvas drag-and-drop
  const handleSpecChange = (updatedSpec: LiveSpecDoc) => {
    setCurrentSpec(updatedSpec);
    initSimulator(updatedSpec, settings);
  };

  // Simulation controls
  const handleTogglePlay = () => {
    if (isPlaying) {
      if (timerRef.current) window.clearInterval(timerRef.current);
      setIsPlaying(false);
    } else {
      if (!simulatorRef.current || simulatorRef.current.isComplete) {
        initSimulator(currentSpec, settings);
      }
      setIsPlaying(true);
    }
  };

  const handleStep = () => {
    if (!simulatorRef.current) return;
    const snap = simulatorRef.current.step();
    setSnapshot({ ...snap });
  };

  const handleReset = () => {
    if (timerRef.current) {
      window.clearInterval(timerRef.current);
      setIsPlaying(false);
    }
    if (simulatorRef.current) {
      simulatorRef.current.reset();
      setSnapshot(simulatorRef.current.getSnapshot());
    } else {
      initSimulator(currentSpec, settings);
    }
    setToastMessage('🔄 时序推演已重置至初始快照');
  };

  const handleInjectFault = (taskId?: string) => {
    if (!simulatorRef.current) return;
    const targetId = taskId || selectedTaskId || currentSpec.tasks[0]?.id;
    if (!targetId) return;

    const ok = simulatorRef.current.injectFailure(targetId, '故障注入：沙箱验收失败，触发返工');
    if (ok) {
      setSnapshot(simulatorRef.current.getSnapshot());
      setToastMessage(`💥 混沌故障注入成功：任务 [${targetId}] 进入 rework 返工状态`);
    }
  };

  // Simulation timer tick loop
  useEffect(() => {
    if (!isPlaying) {
      if (timerRef.current) window.clearInterval(timerRef.current);
      return;
    }

    timerRef.current = window.setInterval(() => {
      if (!simulatorRef.current) return;
      const snap = simulatorRef.current.step();
      setSnapshot({ ...snap });

      if (snap.isComplete) {
        if (timerRef.current) window.clearInterval(timerRef.current);
        setIsPlaying(false);
        setToastMessage('🎉 时序推演结束：所有任务已全部完成！');
      }
    }, speedMs);

    return () => {
      if (timerRef.current) window.clearInterval(timerRef.current);
    };
  }, [isPlaying, speedMs]);

  // Top action: Apply to project
  const handleApply = () => {
    if (cycle.hasCycle) {
      alert('存在环路依赖，禁止应用到工程！请先解除红线环路。');
      return;
    }
    childBridge.applyToProject(currentSpec, diff);
    setToastMessage('✓ 已向宿主发送 SPEC_APPLY 应用指令');
  };

  // Top action: Feedback to chat
  const handleFeedback = () => {
    childBridge.feedbackToChat(diff, currentSpec);
    setToastMessage('💬 已向对话发送 SPEC_FEEDBACK_INTENT 变更意图');
  };

  // Toggle Theme
  const handleThemeToggle = () => {
    setTheme((prev) => (prev === 'dark' ? 'light' : 'dark'));
  };

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        width: '100vw',
        height: '100vh',
        backgroundColor: 'var(--bg, #09090b)',
        overflow: 'hidden',
      }}
    >
      {/* Top Header Control Bar: [⬡ 标题] [💬 反哺改动到对话] [✓ 一键应用到工程] */}
      <header
        style={{
          height: '48px',
          background: 'var(--card, #18181b)',
          borderBottom: '1px solid var(--border, #27272a)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 18px',
          zIndex: 20,
          boxShadow: '0 1px 4px rgba(0,0,0,0.15)',
        }}
      >
        {/* Title & Status Badges */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          <span style={{ fontSize: '18px', color: 'var(--accent, #38bdf8)' }}>⬡</span>
          <h1 style={{ fontSize: '14px', fontWeight: 700, color: 'var(--text, #f4f4f5)', margin: 0 }}>
            {currentSpec.title || 'Agentflow Live-Spec 画布'}
          </h1>
          {cycle.hasCycle && (
            <span
              style={{
                fontSize: '11px',
                padding: '2px 8px',
                borderRadius: '4px',
                background: 'rgba(239, 68, 68, 0.2)',
                color: '#ef4444',
                border: '1px solid #ef4444',
                fontWeight: 700,
              }}
            >
              ⚠️ 环路告警
            </span>
          )}
          {diff.hasChanges && (
            <span
              style={{
                fontSize: '11px',
                padding: '2px 8px',
                borderRadius: '4px',
                background: 'rgba(56, 189, 248, 0.15)',
                color: '#38bdf8',
                border: '1px solid rgba(56, 189, 248, 0.4)',
                fontWeight: 600,
              }}
              title={diff.summary}
            >
              ● 存在本地变更
            </span>
          )}
        </div>

        {/* Action Buttons */}
        <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
          {/* Theme Toggle */}
          <button
            onClick={handleThemeToggle}
            title="切换亮色/暗色主题"
            style={{
              padding: '6px 10px',
              fontSize: '12px',
              borderRadius: '6px',
              background: 'var(--bg, #09090b)',
              color: 'var(--text, #f4f4f5)',
              border: '1px solid var(--border, #27272a)',
              cursor: 'pointer',
            }}
          >
            {theme === 'dark' ? '☀️ 浅色' : '🌙 深色'}
          </button>

          {/* Feedback to Chat Button */}
          <button
            onClick={handleFeedback}
            style={{
              padding: '6px 14px',
              fontSize: '12px',
              fontWeight: 600,
              borderRadius: '6px',
              cursor: 'pointer',
              background: 'var(--bg, #09090b)',
              color: 'var(--text, #f4f4f5)',
              border: '1px solid var(--border, #27272a)',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              transition: 'background 0.2s',
            }}
          >
            <span>💬 反哺改动到对话</span>
          </button>

          {/* One-click Apply Button */}
          <button
            onClick={handleApply}
            disabled={cycle.hasCycle}
            style={{
              padding: '6px 16px',
              fontSize: '12px',
              fontWeight: 700,
              borderRadius: '6px',
              cursor: cycle.hasCycle ? 'not-allowed' : 'pointer',
              background: cycle.hasCycle ? '#52525b' : '#0284c7',
              color: '#ffffff',
              border: 'none',
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              opacity: cycle.hasCycle ? 0.6 : 1,
              transition: 'all 0.2s ease',
              boxShadow: cycle.hasCycle ? 'none' : '0 2px 6px rgba(2, 132, 199, 0.4)',
            }}
          >
            <span>✓ 一键应用到工程</span>
          </button>
        </div>
      </header>

      {/* Toast Notification Banner */}
      {toastMessage && (
        <div
          style={{
            position: 'absolute',
            top: '56px',
            left: '50%',
            transform: 'translateX(-50%)',
            background: 'var(--card, #18181b)',
            color: 'var(--text, #f4f4f5)',
            padding: '8px 18px',
            borderRadius: '8px',
            border: '1px solid var(--border-subtle, #3f3f46)',
            boxShadow: '0 4px 16px rgba(0,0,0,0.3)',
            fontSize: '12px',
            fontWeight: 500,
            zIndex: 100,
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
          }}
        >
          <span>{toastMessage}</span>
        </div>
      )}

      {/* Three-Zone Layout: Left ParametricPanel, Center TopologyCanvas, Bottom SimulationBar */}
      <div style={{ flex: 1, display: 'flex', overflow: 'hidden', position: 'relative' }}>
        {/* Zone 1: Left Parametric Panel */}
        <ParametricPanel
          spec={currentSpec}
          cpm={cpm}
          cycle={cycle}
          settings={settings}
          onSettingsChange={handleSettingsChange}
          selectedTaskId={selectedTaskId}
          onSelectTask={setSelectedTaskId}
        />

        {/* Zone 2: Central Topology Canvas */}
        <TopologyCanvas
          spec={currentSpec}
          cpm={cpm}
          cycle={cycle}
          simTasks={snapshot?.tasks}
          selectedTaskId={selectedTaskId}
          onSelectTask={setSelectedTaskId}
          onSpecChange={handleSpecChange}
          onFaultInject={handleInjectFault}
        />
      </div>

      {/* Zone 3: Bottom Simulation Bar */}
      <SimulationBar
        isPlaying={isPlaying}
        onTogglePlay={handleTogglePlay}
        onStep={handleStep}
        onReset={handleReset}
        onInjectFault={handleInjectFault}
        snapshot={snapshot}
        tasks={currentSpec.tasks}
        selectedTaskId={selectedTaskId}
        speedMs={speedMs}
        onSpeedChange={setSpeedMs}
      />
    </div>
  );
};
