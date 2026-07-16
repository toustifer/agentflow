package server

import (
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
