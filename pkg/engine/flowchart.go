package engine

import (
	"context"
	"fmt"
	"strings"
)

// Flowchart returns a Mermaid flowchart string for the DAG.
func (e *Engine) DAGFlowchart(ctx context.Context, nsID, dagID string) (string, error) {
	graph, err := e.GetDAGGraph(ctx, nsID, dagID)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("```mermaid\n")
	b.WriteString("graph LR\n")

	// Title comment
	b.WriteString(fmt.Sprintf("  %% %s — %s (branch: %s)\n",
		graph.DAG.ID,
		graph.DAG.Title,
		graph.DAG.ExecutionBranch,
	))

	// Status emoji mapping
	statusIcon := func(state TaskState) string {
		switch state {
		case TaskDone:
			return "✅"
		case TaskExecuting, TaskReviewPending:
			return "🔄"
		case TaskReworkNeeded:
			return "🔁"
		case TaskCancelled:
			return "❌"
		default:
			return "⏸️"
		}
	}

	// Nodes: each task as a box with ID, title, worker, status
	for _, n := range graph.Nodes {
		// Mermaid node format: ID["label"]
		// Use ID_display as alias to avoid confusion with the real task ID in edges
		label := fmt.Sprintf("%s: %s\\n%s %s",
			n.TaskID,
			escapeMD(n.Title),
			n.AssignedWorker,
			statusIcon(n.State),
		)
		b.WriteString(fmt.Sprintf("  %s[\"%s\"]\n", n.TaskID, label))
	}

	b.WriteString("\n")

	// Edges between nodes (use task IDs directly, matches the node aliases)
	for _, edge := range graph.Edges {
		b.WriteString(fmt.Sprintf("  %s --> %s\n", edge.FromTaskID, edge.ToTaskID))
	}

	b.WriteString("```\n")

	// Human-readable summary
	b.WriteString(fmt.Sprintf("\n📊 **%s** — %s (%s)\n", graph.DAG.Title, graph.DAG.Status, graph.DAG.ExecutionBranch))
	b.WriteString(fmt.Sprintf("进度: %d/%d 完成 | 执行中: %d | 待处理: %d\n",
		doneCount(graph.Tasks), len(graph.Tasks),
		activeCount(graph.Tasks),
		pendingCount(graph.Tasks),
	))
	for _, w := range graph.Workers {
		b.WriteString(fmt.Sprintf("- %s: %d 个任务\n", w.WorkerID, len(w.Tasks)))
	}

	return b.String(), nil
}

func doneCount(tasks []Task) int {
	c := 0
	for _, t := range tasks {
		if t.State == TaskDone {
			c++
		}
	}
	return c
}

func activeCount(tasks []Task) int {
	c := 0
	for _, t := range tasks {
		switch t.State {
		case TaskExecuting, TaskReviewPending, TaskReworkNeeded:
			c++
		}
	}
	return c
}

func pendingCount(tasks []Task) int {
	c := 0
	for _, t := range tasks {
		if t.State == TaskAssigned {
			c++
		}
	}
	return c
}

func escapeMD(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	s = strings.ReplaceAll(s, "<", "(")
	s = strings.ReplaceAll(s, ">", ")")
	return s
}
