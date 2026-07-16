package server

import (
	"fmt"
	"os"
	"time"
)

type worktreePathState string

const (
	worktreePathMissing         worktreePathState = "missing"
	worktreePathValid           worktreePathState = "valid"
	worktreePathInvalidExisting worktreePathState = "invalid_existing"
)

func classifyWorktreePath(path string) worktreePathState {
	if _, err := os.Stat(path); err != nil {
		return worktreePathMissing
	}
	return worktreePathInvalidExisting
}

// repairInvalidWorktreePath quarantines an invalid worktree path so git can
// recreate a clean worktree at the original location. It never deletes.
func repairInvalidWorktreePath(path string, allowRepair bool) error {
	if !allowRepair {
		return fmt.Errorf("repair not allowed for invalid worktree path %q", path)
	}
	backup := path + ".invalid-bak"
	if _, err := os.Stat(backup); err == nil {
		backup = fmt.Sprintf("%s.invalid-bak-%d", path, time.Now().Unix())
	}
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("quarantine invalid worktree path %q -> %q: %w", path, backup, err)
	}
	return nil
}
