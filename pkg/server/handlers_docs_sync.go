package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/toustifer/agentflow/pkg/engine"
)

// docs_sync — mirror agentflow document data into the project git workdir.
//
// Layout (under {workdir}/.mycompany/agentflow/):
//
//	MANIFEST.json
//	docs/{section}/{safe_path}.md
//	workers/{worker_id}/handbook.json
//	workers/{worker_id}/diary/{date}.md
//	leader/diary/{date}.md
//
// direction:
//   - export (default): SQLite → repo files
//   - import: repo files → SQLite (docs + handbooks + diaries)
//   - both: export then import is NOT used; both means export then skip import.
//     Use "import" for reverse.

const docsSyncRootRel = ".mycompany/agentflow"

func (s *Server) handleDocsSync(ctx context.Context, input map[string]any) (map[string]any, error) {
	nsID, err := requiredString(input, "namespace_id")
	if err != nil {
		return nil, err
	}
	direction, _ := optionalString(input, "direction")
	if direction == "" {
		direction = "export"
	}
	direction = strings.ToLower(strings.TrimSpace(direction))
	switch direction {
	case "export", "import", "both":
	default:
		return nil, fmt.Errorf("direction must be export|import|both, got %q", direction)
	}

	workdir, _ := optionalString(input, "workdir")
	if workdir == "" {
		ns, err := s.engine.GetNamespace(ctx, nsID)
		if err != nil {
			return nil, err
		}
		workdir = getWorkdir(ns)
	}
	if strings.TrimSpace(workdir) == "" {
		return nil, fmt.Errorf("docs_sync rejected: namespace %q has no workdir metadata (and workdir not provided)", nsID)
	}
	workdir = filepath.Clean(workdir)
	if st, err := os.Stat(workdir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("docs_sync rejected: workdir %q is not a directory", workdir)
	}

	root := filepath.Join(workdir, docsSyncRootRel)
	result := map[string]any{
		"namespace_id": nsID,
		"workdir":      workdir,
		"root":         root,
		"direction":    direction,
	}

	if direction == "export" || direction == "both" {
		exp, err := s.exportDocsToWorkdir(ctx, nsID, root)
		if err != nil {
			return nil, err
		}
		result["export"] = exp
	}
	if direction == "import" {
		imp, err := s.importDocsFromWorkdir(ctx, nsID, root)
		if err != nil {
			return nil, err
		}
		result["import"] = imp
	}
	// "both" = export only for safety (import can overwrite SQLite). Documented above.
	return result, nil
}

// bestEffortMirrorDocs exports after writes when workdir is known. Never fails the write path.
func (s *Server) bestEffortMirrorDocs(ctx context.Context, nsID string) {
	if s == nil || s.engine == nil || nsID == "" {
		return
	}
	ns, err := s.engine.GetNamespace(ctx, nsID)
	if err != nil {
		return
	}
	workdir := getWorkdir(ns)
	if workdir == "" {
		return
	}
	root := filepath.Join(workdir, docsSyncRootRel)
	_, _ = s.exportDocsToWorkdir(ctx, nsID, root)
}

func (s *Server) exportDocsToWorkdir(ctx context.Context, nsID, root string) (map[string]any, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir sync root: %w", err)
	}

	written := make([]string, 0)
	// path → content for content_tree_hash (sha256 of sorted "path\0content\n" lines)
	fileBodies := make(map[string]string)
	counts := map[string]int{
		"docs":           0,
		"handbooks":      0,
		"worker_diaries": 0,
		"leader_diaries": 0,
	}

	// --- project docs ---
	docs, err := s.engine.ListProjectDocs(ctx, nsID)
	if err != nil {
		return nil, err
	}
	docsDir := filepath.Join(root, "docs")
	_ = os.RemoveAll(docsDir) // full replace for docs tree
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return nil, err
	}
	for _, doc := range docs {
		rel, body := projectDocFile(doc)
		abs := filepath.Join(docsDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			return nil, err
		}
		relOut := filepath.ToSlash(filepath.Join("docs", rel))
		written = append(written, relOut)
		fileBodies[relOut] = body
		counts["docs"]++
	}

	// --- handbooks ---
	handbooks, err := s.engine.ListHandbooks(ctx, nsID)
	if err != nil {
		return nil, err
	}
	for _, hb := range handbooks {
		rel := filepath.ToSlash(filepath.Join("workers", sanitizePathSegment(hb.WorkerID), "handbook.json"))
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return nil, err
		}
		payload, err := json.MarshalIndent(hb, "", "  ")
		if err != nil {
			return nil, err
		}
		body := string(append(payload, '\n'))
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			return nil, err
		}
		written = append(written, rel)
		fileBodies[rel] = body
		counts["handbooks"]++
	}

	// --- worker diaries ---
	workers, err := s.engine.ListWorkers(ctx, nsID)
	if err != nil {
		return nil, err
	}
	for _, w := range workers {
		diaries, err := s.engine.ListWorkerDiaries(ctx, nsID, w.ID)
		if err != nil {
			return nil, err
		}
		for _, d := range diaries {
			rel := filepath.ToSlash(filepath.Join("workers", sanitizePathSegment(w.ID), "diary", sanitizePathSegment(d.Date)+".md"))
			abs := filepath.Join(root, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return nil, err
			}
			body := workerDiaryFile(d)
			if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
				return nil, err
			}
			written = append(written, rel)
			fileBodies[rel] = body
			counts["worker_diaries"]++
		}
	}

	// --- leader diaries ---
	leaderDiaries, err := s.engine.ListLeaderDiaries(ctx, nsID)
	if err != nil {
		return nil, err
	}
	for _, ld := range leaderDiaries {
		rel := filepath.ToSlash(filepath.Join("leader", "diary", sanitizePathSegment(ld.Date)+".md"))
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return nil, err
		}
		body := leaderDiaryFile(ld)
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			return nil, err
		}
		written = append(written, rel)
		fileBodies[rel] = body
		counts["leader_diaries"]++
	}

	sort.Strings(written)
	treeHash := contentTreeHash(fileBodies)
	manifest := map[string]any{
		"namespace_id":       nsID,
		"exported_at":        time.Now().UTC().Format(time.RFC3339Nano),
		"counts":             counts,
		"files":              written,
		"source":             "agentflow",
		"layout":             "v1",
		"content_tree_hash":  treeHash,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, "MANIFEST.json"), append(manifestBytes, '\n'), 0o644); err != nil {
		return nil, err
	}
	written = append(written, "MANIFEST.json")

	return map[string]any{
		"counts":            counts,
		"files":             written,
		"root":              root,
		"written":           len(written),
		"content_tree_hash": treeHash,
	}, nil
}

// contentTreeHash returns sha256 hex of sorted "relpath\0body\n" entries.
// Stable across machines for the same export content; MANIFEST itself is excluded.
func contentTreeHash(bodies map[string]string) string {
	if len(bodies) == 0 {
		h := sha256.Sum256(nil)
		return hex.EncodeToString(h[:])
	}
	keys := make([]string, 0, len(bodies))
	for k := range bodies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte(0)
		b.WriteString(bodies[k])
		if !strings.HasSuffix(bodies[k], "\n") {
			b.WriteByte('\n')
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func (s *Server) importDocsFromWorkdir(ctx context.Context, nsID, root string) (map[string]any, error) {
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("docs_sync import: root %q not found — run export first or create files under .mycompany/agentflow/", root)
	}

	counts := map[string]int{
		"docs":           0,
		"handbooks":      0,
		"worker_diaries": 0,
		"leader_diaries": 0,
	}
	imported := make([]string, 0)

	// docs/**/*.md
	docsDir := filepath.Join(root, "docs")
	if st, err := os.Stat(docsDir); err == nil && st.IsDir() {
		err := filepath.WalkDir(docsDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			doc, err := parseProjectDocFile(string(raw), path, docsDir)
			if err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			if _, err := s.engine.WriteProjectDoc(ctx, nsID, doc); err != nil {
				// id may not exist locally — fall back to create
				doc.ID = 0
				if _, err2 := s.engine.WriteProjectDoc(ctx, nsID, doc); err2 != nil {
					return err2
				}
			}
			rel, _ := filepath.Rel(root, path)
			imported = append(imported, filepath.ToSlash(rel))
			counts["docs"]++
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// workers/*/handbook.json
	workersDir := filepath.Join(root, "workers")
	if st, err := os.Stat(workersDir); err == nil && st.IsDir() {
		entries, err := os.ReadDir(workersDir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			workerID := e.Name()
			hbPath := filepath.Join(workersDir, workerID, "handbook.json")
			if raw, err := os.ReadFile(hbPath); err == nil {
				var hb engine.WorkerHandbook
				if err := json.Unmarshal(raw, &hb); err != nil {
					return nil, fmt.Errorf("handbook %s: %w", hbPath, err)
				}
				if hb.WorkerID == "" {
					hb.WorkerID = workerID
				}
				// Ensure worker exists (skip if not registered)
				if _, err := s.engine.GetWorker(ctx, nsID, hb.WorkerID); err != nil {
					continue
				}
				if _, err := s.engine.WriteHandbook(ctx, engine.WriteHandbookRequest{
					NamespaceID: nsID,
					WorkerID:    hb.WorkerID,
					Scope:       hb.Scope,
					TechStack:   hb.TechStack,
					Knowledge:   hb.Knowledge,
					Pitfalls:    hb.Pitfalls,
				}); err != nil {
					return nil, err
				}
				imported = append(imported, filepath.ToSlash(filepath.Join("workers", workerID, "handbook.json")))
				counts["handbooks"]++
			}

			diaryDir := filepath.Join(workersDir, workerID, "diary")
			if st, err := os.Stat(diaryDir); err == nil && st.IsDir() {
				files, err := os.ReadDir(diaryDir)
				if err != nil {
					return nil, err
				}
				for _, f := range files {
					if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".md") {
						continue
					}
					raw, err := os.ReadFile(filepath.Join(diaryDir, f.Name()))
					if err != nil {
						return nil, err
					}
					date := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))
					content, taskID, tags := parseDiaryMarkdown(string(raw))
					if _, err := s.engine.GetWorker(ctx, nsID, workerID); err != nil {
						continue
					}
					if _, err := s.engine.WriteWorkerDiary(ctx, engine.WriteDiaryRequest{
						NamespaceID: nsID,
						WorkerID:    workerID,
						Date:        date,
						TaskID:      taskID,
						Content:     content,
						Tags:        tags,
					}); err != nil {
						return nil, err
					}
					imported = append(imported, filepath.ToSlash(filepath.Join("workers", workerID, "diary", f.Name())))
					counts["worker_diaries"]++
				}
			}
		}
	}

	// leader/diary/*.md
	leaderDiaryDir := filepath.Join(root, "leader", "diary")
	if st, err := os.Stat(leaderDiaryDir); err == nil && st.IsDir() {
		files, err := os.ReadDir(leaderDiaryDir)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(strings.ToLower(f.Name()), ".md") {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(leaderDiaryDir, f.Name()))
			if err != nil {
				return nil, err
			}
			date := strings.TrimSuffix(f.Name(), filepath.Ext(f.Name()))
			content, _, tags := parseDiaryMarkdown(string(raw))
			if _, err := s.engine.WriteLeaderDiary(ctx, engine.WriteLeaderDiaryRequest{
				NamespaceID: nsID,
				Date:        date,
				Entry: engine.DiaryEntry{
					Type:    "note",
					Title:   "imported",
					Content: content,
					Tags:    tags,
				},
			}); err != nil {
				return nil, err
			}
			imported = append(imported, filepath.ToSlash(filepath.Join("leader", "diary", f.Name())))
			counts["leader_diaries"]++
		}
	}

	sort.Strings(imported)
	return map[string]any{
		"counts":   counts,
		"files":    imported,
		"imported": len(imported),
		"root":     root,
	}, nil
}

func projectDocFile(doc engine.ProjectDoc) (rel string, body string) {
	section := sanitizePathSegment(doc.Section)
	if section == "" {
		section = "general"
	}
	name := strings.TrimSpace(doc.Path)
	if name == "" {
		name = fmt.Sprintf("doc-%d.md", doc.ID)
	}
	name = filepath.ToSlash(name)
	name = strings.TrimPrefix(name, "/")
	// strip dangerous segments
	parts := strings.Split(name, "/")
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = sanitizePathSegment(p)
		if p == "" || p == "." || p == ".." {
			continue
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		clean = []string{fmt.Sprintf("doc-%d.md", doc.ID)}
	}
	// ensure .md extension on last segment
	last := clean[len(clean)-1]
	if !strings.HasSuffix(strings.ToLower(last), ".md") {
		clean[len(clean)-1] = last + ".md"
	}
	rel = section + "/" + strings.Join(clean, "/")

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("id: %d\n", doc.ID))
	b.WriteString(fmt.Sprintf("section: %q\n", doc.Section))
	b.WriteString(fmt.Sprintf("path: %q\n", doc.Path))
	b.WriteString(fmt.Sprintf("title: %q\n", doc.Title))
	b.WriteString(fmt.Sprintf("version: %d\n", doc.Version))
	if len(doc.Tags) > 0 {
		tagJSON, _ := json.Marshal(doc.Tags)
		b.WriteString(fmt.Sprintf("tags: %s\n", string(tagJSON)))
	}
	b.WriteString("---\n\n")
	b.WriteString(doc.Content)
	if !strings.HasSuffix(doc.Content, "\n") {
		b.WriteString("\n")
	}
	return rel, b.String()
}

func workerDiaryFile(d engine.WorkerDiary) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("worker_id: %q\n", d.WorkerID))
	b.WriteString(fmt.Sprintf("date: %q\n", d.Date))
	if d.TaskID != "" {
		b.WriteString(fmt.Sprintf("task_id: %q\n", d.TaskID))
	}
	if len(d.Tags) > 0 {
		tagJSON, _ := json.Marshal(d.Tags)
		b.WriteString(fmt.Sprintf("tags: %s\n", string(tagJSON)))
	}
	b.WriteString("---\n\n")
	b.WriteString(d.Content)
	if !strings.HasSuffix(d.Content, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

func leaderDiaryFile(ld engine.LeaderDiary) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("date: %q\n", ld.Date))
	b.WriteString(fmt.Sprintf("entries: %d\n", len(ld.Entries)))
	b.WriteString("---\n\n")
	for i, e := range ld.Entries {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		if e.Title != "" {
			b.WriteString("## ")
			b.WriteString(e.Title)
			b.WriteString("\n\n")
		}
		if e.Type != "" {
			b.WriteString(fmt.Sprintf("_type: %s_", e.Type))
			if e.TaskID != "" {
				b.WriteString(fmt.Sprintf(" · task `%s`", e.TaskID))
			}
			if e.DAGID != "" {
				b.WriteString(fmt.Sprintf(" · dag `%s`", e.DAGID))
			}
			b.WriteString("\n\n")
		}
		b.WriteString(e.Content)
		if !strings.HasSuffix(e.Content, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func parseProjectDocFile(raw, absPath, docsDir string) (engine.ProjectDoc, error) {
	meta, body := splitFrontmatter(raw)
	doc := engine.ProjectDoc{
		Title:   meta["title"],
		Section: meta["section"],
		Path:    meta["path"],
		Content: body,
	}
	if idStr := meta["id"]; idStr != "" {
		var id int64
		if _, err := fmt.Sscanf(idStr, "%d", &id); err == nil {
			doc.ID = id
		}
	}
	if tagsRaw := meta["tags"]; tagsRaw != "" {
		var tags []string
		if err := json.Unmarshal([]byte(tagsRaw), &tags); err == nil {
			doc.Tags = tags
		}
	}
	// fill path/section from filesystem if missing
	if doc.Section == "" || doc.Path == "" {
		rel, err := filepath.Rel(docsDir, absPath)
		if err == nil {
			rel = filepath.ToSlash(rel)
			parts := strings.Split(rel, "/")
			if doc.Section == "" && len(parts) > 0 {
				doc.Section = parts[0]
			}
			if doc.Path == "" {
				if len(parts) > 1 {
					doc.Path = strings.Join(parts[1:], "/")
				} else {
					doc.Path = rel
				}
			}
		}
	}
	if doc.Title == "" {
		doc.Title = strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	}
	if strings.TrimSpace(doc.Content) == "" {
		return doc, fmt.Errorf("empty content")
	}
	return doc, nil
}

func parseDiaryMarkdown(raw string) (content, taskID string, tags []string) {
	meta, body := splitFrontmatter(raw)
	taskID = meta["task_id"]
	if tagsRaw := meta["tags"]; tagsRaw != "" {
		_ = json.Unmarshal([]byte(tagsRaw), &tags)
	}
	content = body
	if content == "" {
		content = raw
	}
	return content, taskID, tags
}

// splitFrontmatter parses simple YAML-ish frontmatter between --- fences.
// Values may be quoted with "..." or bare; tags may be a JSON array.
func splitFrontmatter(raw string) (map[string]string, string) {
	meta := map[string]string{}
	s := strings.TrimPrefix(raw, "\uFEFF")
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return meta, strings.TrimSpace(raw)
	}
	rest := s
	if strings.HasPrefix(rest, "---\r\n") {
		rest = strings.TrimPrefix(rest, "---\r\n")
	} else {
		rest = strings.TrimPrefix(rest, "---\n")
	}
	end := strings.Index(rest, "\n---\n")
	endCR := strings.Index(rest, "\r\n---\r\n")
	sepLen := len("\n---\n")
	if end < 0 || (endCR >= 0 && endCR < end) {
		if endCR < 0 {
			return meta, strings.TrimSpace(raw)
		}
		end = endCR
		sepLen = len("\r\n---\r\n")
	}
	header := rest[:end]
	body := strings.TrimSpace(rest[end+sepLen:])
	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:colon])
		val := strings.TrimSpace(line[colon+1:])
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = val[1 : len(val)-1]
		}
		meta[key] = val
	}
	return meta, body
}

func sanitizePathSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\\", "/")
	s = filepath.Base(s) // drop any path components
	s = strings.ReplaceAll(s, "..", "")
	// keep alnum, dash, underscore, dot
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	out = strings.Trim(out, ".")
	return out
}
