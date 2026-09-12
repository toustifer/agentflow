/**
 * Agentflow Live-Spec Core DSL and Protocol Types
 */

export type TaskState =
  | 'pending'
  | 'ready'
  | 'executing'
  | 'submitted'
  | 'passed'
  | 'rework'
  | 'blocked'
  | 'cancelled';

export interface SpecTask {
  id: string;
  title: string;
  description?: string;
  assigned_worker?: string;
  depends_on?: string[];
  acceptance_criteria?: string[];
  estimated_hours?: number;
  priority?: number;
  tags?: string[];
  output_files?: string[];
  metadata?: Record<string, unknown>;
  state?: TaskState;
}

export interface SpecSettings {
  concurrency?: number;
  autoLayout?: boolean;
  theme?: string;
  [key: string]: unknown;
}

export interface LiveSpecDoc {
  version: string;
  title: string;
  dag_id?: string;
  namespace_id?: string;
  tasks: SpecTask[];
  concurrency?: number;
  metadata?: Record<string, unknown>;
  settings?: SpecSettings;
}

// CPM Types
export interface CPMTaskNode {
  id: string;
  duration: number;
  es: number; // Early Start
  ef: number; // Early Finish
  ls: number; // Late Start
  lf: number; // Late Finish
  slack: number; // Total Slack = ls - es
  isCritical: boolean;
  successors: string[];
  predecessors: string[];
}

export interface CPMResult {
  hasCycle: boolean;
  totalDuration: number;
  criticalPath: string[];
  criticalPaths: string[][];
  bottlenecks: string[];
  nodes: Record<string, CPMTaskNode>;
  error?: string;
}

// Cycle Detection Types
export interface CycleDetectionResult {
  hasCycle: boolean;
  cycle?: string[];
  cycles: string[][];
}

// Diff Types
export interface TaskFieldChange {
  field: string;
  oldValue: unknown;
  newValue: unknown;
}

export interface TaskDiff {
  id: string;
  title: string;
  status: 'added' | 'removed' | 'modified';
  changes?: TaskFieldChange[];
}

export interface DependencyEdge {
  from: string; // Predecessor
  to: string;   // Successor (depends on `from`)
}

export interface SpecDiffResult {
  hasChanges: boolean;
  addedTasks: SpecTask[];
  removedTasks: SpecTask[];
  modifiedTasks: TaskDiff[];
  addedDependencies: DependencyEdge[];
  removedDependencies: DependencyEdge[];
  parameterChanges: TaskFieldChange[];
  summary: string;
}

// Bridge Protocol Types
export type DownstreamEvent =
  | { type: 'INIT_DOC'; payload: { doc: LiveSpecDoc; readOnly?: boolean } }
  | { type: 'SYNC_SPEC'; payload: { doc: LiveSpecDoc } }
  | { type: 'SET_SIMULATION_PARAMS'; payload: { concurrency?: number; speed?: number } }
  | { type: 'INJECT_FAILURE'; payload: { taskId: string; reason?: string } }
  | { type: 'SIMULATION_CONTROL'; payload: { action: 'start' | 'pause' | 'step' | 'reset' } }
  | { type: 'APPLY_RESULT'; payload: { success: boolean; message?: string; taskIds?: string[] } };

export type UpstreamEvent =
  | { type: 'CANVAS_READY'; payload?: Record<string, unknown> }
  | { type: 'SPEC_CHANGED'; payload: { doc: LiveSpecDoc; diff?: SpecDiffResult } }
  | { type: 'APPLY_TO_PROJECT'; payload: { doc: LiveSpecDoc; diff: SpecDiffResult } }
  | { type: 'FEEDBACK_TO_CHAT'; payload: { prompt: string; diff: SpecDiffResult; doc: LiveSpecDoc } }
  | { type: 'NODE_SELECTED'; payload: { taskId: string | null } }
  | { type: 'REQUEST_FULLSCREEN'; payload: { fullscreen: boolean } };

export type BridgeEvent = DownstreamEvent | UpstreamEvent;
