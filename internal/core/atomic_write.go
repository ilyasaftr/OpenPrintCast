package core

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Replace retry policy: an antivirus scanner or indexer may briefly hold the
// destination open on Windows, so retryable replace errors are retried for
// up to replaceRetryTotal in replaceRetryStep increments.
const (
	replaceRetryTotal = time.Second
	replaceRetryStep  = 10 * time.Millisecond
)

// tempFilePattern is appended to the target's base name for the temp file
// (os.CreateTemp replaces the "*" with a random string).
const tempFilePattern = ".tmp-*"

// fileReplacer atomically moves src over dst, replacing dst if it exists.
type fileReplacer func(src, dst string) error

// tempFile is the part of *os.File the atomic writer needs.
type tempFile interface {
	Write(p []byte) (int, error)
	Sync() error
	Close() error
	Chmod(mode fs.FileMode) error
	Name() string
}

// atomicWriter writes files via temp file + replace. Every collaborator is a
// field so tests can inject failures and avoid real sleeping.
type atomicWriter struct {
	createTemp func(dir, pattern string) (tempFile, error)
	replace    fileReplacer
	isRetry    func(error) bool
	sleep      func(time.Duration)
}

// defaultAtomicWriter uses the real filesystem and the platform replacer.
func defaultAtomicWriter() atomicWriter {
	return atomicWriter{
		createTemp: func(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) },
		replace:    replaceFile,
		isRetry:    isRetryableReplaceError,
		sleep:      time.Sleep,
	}
}

// WriteFileAtomic is the exported form of the package's atomic file write
// for other packages that persist state: readers of path see either the old
// content or the new content, never a partial file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return writeFileAtomic(path, data, perm)
}

// writeFileAtomic writes data to path so that readers see either the old
// content or the new content, never a partial file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	return defaultAtomicWriter().writeFile(path, data, perm)
}

// writeFile stages data in a temp file beside path, then replaces path with
// it. On any failure the temp file is removed and path is left untouched.
func (w atomicWriter) writeFile(path string, data []byte, perm os.FileMode) error {
	tmpPath, err := w.stage(path, data, perm)
	if err != nil {
		return err
	}
	if err := w.replaceWithRetry(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// stage writes data durably to a new temp file in path's directory and
// returns its name. The temp file is removed if staging fails.
func (w atomicWriter) stage(path string, data []byte, perm os.FileMode) (string, error) {
	tmp, err := w.createTemp(filepath.Dir(path), filepath.Base(path)+tempFilePattern)
	if err != nil {
		return "", err
	}
	if err := fillTempFile(tmp, data, perm); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// fillTempFile writes data, applies perm, fsyncs and closes tmp. tmp is
// closed even when an earlier step fails; the first error wins.
func fillTempFile(tmp tempFile, data []byte, perm os.FileMode) error {
	err := writeSyncChmod(tmp, data, perm)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	return err
}

// writeSyncChmod performs the steps of fillTempFile that precede Close.
func writeSyncChmod(tmp tempFile, data []byte, perm os.FileMode) error {
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	return tmp.Sync()
}

// replaceWithRetry replaces dst with src, retrying while the error is
// retryable until replaceRetryTotal has elapsed (counted in steps, so the
// injected sleeper decides real time).
func (w atomicWriter) replaceWithRetry(src, dst string) error {
	for waited := time.Duration(0); ; waited += replaceRetryStep {
		err := w.replace(src, dst)
		if err == nil || !w.isRetry(err) || waited >= replaceRetryTotal {
			return err
		}
		w.sleep(replaceRetryStep)
	}
}
