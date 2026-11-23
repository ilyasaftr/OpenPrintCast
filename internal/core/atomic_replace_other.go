//go:build !windows

package core

import (
	"os"
	"path/filepath"
)

// replaceFile atomically renames src over dst (POSIX rename semantics) and
// then flushes the directory entry so the rename survives power loss.
func replaceFile(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return err
	}
	syncParentDir(dst)
	return nil
}

// syncParentDir fsyncs path's directory. Best-effort: errors are ignored
// because not every filesystem supports syncing a directory.
func syncParentDir(path string) {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return
	}
	_ = dir.Sync()
	_ = dir.Close()
}

// isRetryableReplaceError is always false: rename does not fail transiently
// because another process has the destination open.
func isRetryableReplaceError(error) bool { return false }
