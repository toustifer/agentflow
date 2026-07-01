package engine

import (
	"context"
	"errors"
	"sort"
)

// ---------------------------------------------------------------------------
// NextTask — a single recommended or blocked task for the Leader
// ---------------------------------------------------------------------------

type NextTask struct {
	TaskID        string `json:"task_id"`
	Title         string `json:"title"`
	DAGID         string `json:"dag_id"`
	AssignedWorker string `json:"assigned_worker"`
	State         string `json:"state"`
	DepsSatisfied bool   `json:"deps_satisfied"`
	WorkerBusy    bool   `json:"worker_busy"`
	Ready         bool   `json:"ready"`
	Reason        string `json:"reason,omitempty"`
}

// ProjectNextTasks returns all tasks in the namespace that are candidates
// for dispatch. Each task is annotated with dependency and worker status
// so the Leader knows what can be started right now and what is blocked.
func (e *Engine) ProjectNextTasks(ctx context.Context, nsID string) ([]NextTask, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}

	out := make([]NextTask, 0)

	// Scan tasks only in assigned or rework_needed state —
	// already-executing tasks should not be re-dispatched.
	for _, t := range e.tasks[nsID] {
		if t.State != TaskAssigned && t.State != TaskReworkNeeded {
			continue
		}

		// Check deps
		depsSatisfied := true
		for _, dep := range t.DependsOn {
			depTask, ok := e.tasks[nsID][dep]
			if !ok || depTask.State != TaskDone {
				depsSatisfied = false
				break
			}
		}

		// Check worker availability (cross-DAG)
		workerBusy := false
		if t.AssignedWorker != "" {
			workerBusy = e.workerStatusUnsafe(nsID, t.AssignedWorker) == WorkerBusy
		}

		ready := depsSatisfied && !workerBusy

		var reason string
		if !depsSatisfied {
			var blockedBy []string
			for _, dep := range t.DependsOn {
				depTask, ok := e.tasks[nsID][dep]
				if !ok || depTask.State != TaskDone {
					blockedBy = append(blockedBy, dep)
				}
			}
			reason = "blocked by: " + joinStrings(blockedBy)
		} else if workerBusy {
			reason = "worker " + t.AssignedWorker + " is busy on another task"
		}

		out = append(out, NextTask{
			TaskID:        t.ID,
			Title:         t.Title,
			DAGID:         t.DAGID,
			AssignedWorker: t.AssignedWorker,
			State:         string(t.State),
			DepsSatisfied: depsSatisfied,
			WorkerBusy:    workerBusy,
			Ready:         ready,
			Reason:        reason,
		})
	}

	sort.Slice(out, func(i, j int) bool {
		// Ready tasks first, then by ID
		if out[i].Ready != out[j].Ready {
			return out[i].Ready
		}
		return out[i].TaskID < out[j].TaskID
	})

	return out, nil
}

// ---------------------------------------------------------------------------
// Blockers — explicit and implicit blockers across all DAGs
// ---------------------------------------------------------------------------

type Blocker struct {
	TaskID    string `json:"task_id"`
	Title     string `json:"title"`
	DAGID     string `json:"dag_id"`
	Type      string `json:"type"`               // "dependency" or "worker"
	BlockedBy string `json:"blocked_by,omitempty"` // dep task ID or worker ID
}

// ProjectBlockers returns every task that cannot proceed right now,
// along with the reason (dependency not met or worker busy).
func (e *Engine) ProjectBlockers(ctx context.Context, nsID string) ([]Blocker, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}

	out := make([]Blocker, 0)

	for _, t := range e.tasks[nsID] {
		if t.State != TaskAssigned && t.State != TaskReworkNeeded {
			continue
		}

		// Dependency blockers
		for _, dep := range t.DependsOn {
			depTask, ok := e.tasks[nsID][dep]
			if !ok || depTask.State != TaskDone {
				out = append(out, Blocker{
					TaskID:    t.ID,
					Title:     t.Title,
					DAGID:     t.DAGID,
					Type:      "dependency",
					BlockedBy: dep,
				})
			}
		}

		// Worker blockers (only if deps are satisfied)
		if t.AssignedWorker != "" && len(t.DependsOn) > 0 {
			// Only report worker as blocker if deps are already satisfied
			depsSatisfied := true
			for _, dep := range t.DependsOn {
				depTask, ok := e.tasks[nsID][dep]
				if !ok || depTask.State != TaskDone {
					depsSatisfied = false
					break
				}
			}
			if depsSatisfied && e.workerStatusUnsafe(nsID, t.AssignedWorker) == WorkerBusy {
				out = append(out, Blocker{
					TaskID:    t.ID,
					Title:     t.Title,
					DAGID:     t.DAGID,
					Type:      "worker",
					BlockedBy: t.AssignedWorker,
				})
			}
		}
	}

	return out, nil
}

func joinStrings(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

// Ensure interface compliance
// ---------------------------------------------------------------------------
// ProjectReport — cross-DAG project-level summary
// ---------------------------------------------------------------------------

type ProjectReport struct {
	TotalDAGs     int                 `json:"total_dags"`
	TotalTasks    int                 `json:"total_tasks"`
	DoneTasks     int                 `json:"done_tasks"`
	ExecutingTasks int                `json:"executing_tasks"`
	PendingTasks  int                 `json:"pending_tasks"`
	CompletionPct float64             `json:"completion_pct"`
	DAGs          []DAGReport         `json:"dags"`
	Workers       []ProjectWorkerInfo `json:"workers"`
}

type ProjectWorkerInfo struct {
	WorkerID   string `json:"worker_id"`
	TotalTasks int    `json:"total_tasks"`
	DoneTasks  int    `json:"done_tasks"`
	Status     string `json:"status"`
}

// ProjectReport returns a cross-DAG summary for the entire namespace.
func (e *Engine) ProjectReport(ctx context.Context, nsID string) (*ProjectReport, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if _, ok := e.namespaces[nsID]; !ok {
		return nil, ErrNamespaceNotFound
	}

	dagSummary := make([]DAGReport, 0)
	workerAgg := make(map[string]*ProjectWorkerInfo)

	totalTasks := 0
	doneTasks := 0
	execTasks := 0

	// Walk all DAGs
	for _, dag := range e.dags[nsID] {
		var tasks []Task
		workerTasks := make(map[string]int)
		workerDone := make(map[string]int)

		for _, t := range e.tasks[nsID] {
			if t.DAGID == dag.ID {
				tasks = append(tasks, *cloneTask(t))
				totalTasks++
				switch t.State {
				case TaskDone:
					doneTasks++
					workerDone[t.AssignedWorker]++
				case TaskExecuting, TaskReviewPending, TaskReworkNeeded:
					execTasks++
				}
				workerTasks[t.AssignedWorker]++
			}
		}

		// Build worker summary for this DAG
		dagWorkers := make([]DAGWorkerSummary, 0)
		for wid, cnt := range workerTasks {
			dagWorkers = append(dagWorkers, DAGWorkerSummary{
				WorkerID:   wid,
				TotalTasks: cnt,
				DoneTasks:  workerDone[wid],
				Status:     string(e.workerStatusUnsafe(nsID, wid)),
			})
			// Accumulate to project-level
			if _, ok := workerAgg[wid]; !ok {
				workerAgg[wid] = &ProjectWorkerInfo{WorkerID: wid}
			}
			workerAgg[wid].TotalTasks += cnt
			workerAgg[wid].DoneTasks += workerDone[wid]
		}

		pct := 0.0
		if len(tasks) > 0 {
			d := 0
			for _, t := range tasks {
				if t.State == TaskDone {
					d++
				}
			}
			pct = float64(d) / float64(len(tasks)) * 100
		}

		dagSummary = append(dagSummary, DAGReport{
			DAG:            *cloneDAG(dag),
			TotalTasks:     len(tasks),
			DoneTasks:      doneInDAG(tasks),
			ExecutingTasks: activeInDAG(tasks),
			PendingTasks:   len(tasks) - doneInDAG(tasks) - activeInDAG(tasks),
			CompletionPct:  pct,
			Workers:        dagWorkers,
		})
	}

	workers := make([]ProjectWorkerInfo, 0, len(workerAgg))
	for _, w := range workerAgg {
		w.Status = string(e.workerStatusUnsafe(nsID, w.WorkerID))
		workers = append(workers, *w)
	}
	sort.Slice(workers, func(i, j int) bool { return workers[i].WorkerID < workers[j].WorkerID })

	pct := 0.0
	if totalTasks > 0 {
		pct = float64(doneTasks) / float64(totalTasks) * 100
	}

	return &ProjectReport{
		TotalDAGs:      len(dagSummary),
		TotalTasks:     totalTasks,
		DoneTasks:      doneTasks,
		ExecutingTasks: execTasks,
		PendingTasks:   totalTasks - doneTasks - execTasks,
		CompletionPct:  pct,
		DAGs:           dagSummary,
		Workers:        workers,
	}, nil
}

func doneInDAG(tasks []Task) int {
	c := 0
	for _, t := range tasks {
		if t.State == TaskDone {
			c++
		}
	}
	return c
}

func activeInDAG(tasks []Task) int {
	c := 0
	for _, t := range tasks {
		switch t.State {
		case TaskExecuting, TaskReviewPending, TaskReworkNeeded:
			c++
		}
	}
	return c
}

var _ = errors.New("unused") // keep import
