package server

import (
	"fmt"
	"sort"
	"strings"

	"github.com/toustifer/agentflow/pkg/engine"
)

const (
	dagMetadataLegacy         = "dag.legacy"
	dagMetadataResumePriority = "dag.resume_priority"
	dagMetadataSupersededBy   = "dag.superseded_by"
)

type resumePriority string

const (
	resumePriorityDefault      resumePriority = "default"
	resumePriorityDeprioritized resumePriority = "deprioritized"
	resumePriorityForce        resumePriority = "force"
)

type dagResumeHint struct {
	Legacy         bool
	ResumePriority resumePriority
	SupersededBy   string
}

func dagResumeHintFromMetadata(metadata map[string]string) dagResumeHint {
	hint := dagResumeHint{ResumePriority: resumePriorityDefault}
	if metadata == nil {
		return hint
	}
	if strings.EqualFold(metadata[dagMetadataLegacy], "true") {
		hint.Legacy = true
	}
	switch metadata[dagMetadataResumePriority] {
	case string(resumePriorityDeprioritized):
		hint.ResumePriority = resumePriorityDeprioritized
	case string(resumePriorityForce):
		hint.ResumePriority = resumePriorityForce
	}
	hint.SupersededBy = metadata[dagMetadataSupersededBy]
	return hint
}

func pickResumeDAG(dags []engine.DAG, targetDAGID string) *engine.DAG {
	// Recommendation-only helper for inspect/resume display.
	// Leader dispatch / leader_tick MUST use resolveDAGFocus instead
	// (explicit dag_id or single-auto; multi open DAGs error).
	if targetDAGID != "" {
		for i := range dags {
			if dags[i].ID == targetDAGID {
				return &dags[i]
			}
		}
		return nil
	}

	primary := make([]engine.DAG, 0, len(dags))
	fallback := make([]engine.DAG, 0, len(dags))
	for _, dag := range dags {
		if dag.Status == engine.DAGDone || dag.Status == engine.DAGCancelled {
			continue
		}
		fallback = append(fallback, dag)
		hint := dagResumeHintFromMetadata(dag.Metadata)
		if hint.Legacy || hint.ResumePriority == resumePriorityDeprioritized {
			continue
		}
		primary = append(primary, dag)
	}

	candidates := primary
	if len(candidates) == 0 {
		candidates = fallback
	}
	if len(candidates) == 0 {
		return nil
	}
	orderResumeCandidates(candidates)
	return &candidates[0]
}

// Focus source labels returned on project_next_steps / leader_tick.
const (
	focusSourceExplicit   = "explicit"
	focusSourceSingleAuto = "single_auto"
	focusSourceNone       = "none"
)

// MultiDAGFocusError is returned when dag_id is omitted but multiple
// non-done primary DAGs are open. Callers must pass dag_id explicitly.
type MultiDAGFocusError struct {
	Candidates []map[string]any
}

func (e *MultiDAGFocusError) Error() string {
	if e == nil {
		return "dag_id required: multiple active DAGs"
	}
	if len(e.Candidates) == 0 {
		return "dag_id required: multiple active DAGs"
	}
	ids := make([]string, 0, len(e.Candidates))
	for _, c := range e.Candidates {
		if id, ok := c["dag_id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return "dag_id required: multiple active DAGs; candidates=" + strings.Join(ids, ",")
}

// listPrimaryOpenDAGs returns non-done, non-legacy, non-deprioritized DAGs.
func listPrimaryOpenDAGs(dags []engine.DAG) []engine.DAG {
	out := make([]engine.DAG, 0, len(dags))
	for _, dag := range dags {
		if dag.Status == engine.DAGDone || dag.Status == engine.DAGCancelled {
			continue
		}
		hint := dagResumeHintFromMetadata(dag.Metadata)
		if hint.Legacy || hint.ResumePriority == resumePriorityDeprioritized {
			continue
		}
		out = append(out, dag)
	}
	orderResumeCandidates(out)
	return out
}

func dagCandidateSummary(dag engine.DAG) map[string]any {
	return map[string]any{
		"dag_id":           dag.ID,
		"title":            dag.Title,
		"status":           string(dag.Status),
		"execution_branch": dag.ExecutionBranch,
	}
}

// resolveDAGFocus selects the focused DAG for leader_tick / project_next_steps.
//
//	dag_id set     → that DAG (error if missing)
//	0 open primary → (nil, none)
//	1 open primary → that DAG (single_auto)
//	>1 open        → MultiDAGFocusError with candidates
func resolveDAGFocus(dags []engine.DAG, targetDAGID string) (*engine.DAG, string, error) {
	if targetDAGID != "" {
		for i := range dags {
			if dags[i].ID == targetDAGID {
				return &dags[i], focusSourceExplicit, nil
			}
		}
		return nil, "", fmt.Errorf("dag_id %q not found", targetDAGID)
	}
	primary := listPrimaryOpenDAGs(dags)
	switch len(primary) {
	case 0:
		return nil, focusSourceNone, nil
	case 1:
		return &primary[0], focusSourceSingleAuto, nil
	default:
		cands := make([]map[string]any, 0, len(primary))
		for _, d := range primary {
			cands = append(cands, dagCandidateSummary(d))
		}
		return nil, "", &MultiDAGFocusError{Candidates: cands}
	}
}

func orderResumeCandidates(dags []engine.DAG) {
	sort.Slice(dags, func(i, j int) bool {
		left := dags[i]
		right := dags[j]
		if left.Status != right.Status {
			if left.Status == engine.DAGInProgress {
				return true
			}
			if right.Status == engine.DAGInProgress {
				return false
			}
		}
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.After(right.UpdatedAt)
		}
		return left.ID < right.ID
	})
}

func summarizeSkippedLegacyDAGs(dags []engine.DAG, selectedID string) []map[string]any {
	items := make([]map[string]any, 0)
	for _, dag := range dags {
		if dag.ID == selectedID {
			continue
		}
		hint := dagResumeHintFromMetadata(dag.Metadata)
		if !hint.Legacy && hint.ResumePriority != resumePriorityDeprioritized {
			continue
		}
		items = append(items, map[string]any{
			"dag_id":          dag.ID,
			"title":           dag.Title,
			"legacy":          hint.Legacy,
			"resume_priority": string(hint.ResumePriority),
			"superseded_by":   hint.SupersededBy,
		})
	}
	return items
}
