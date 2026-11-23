package winspool

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// stagedPDF is the output of one folder job while it is being written. The
// file is created in a private directory inside the destination folder on
// the first write, so a job that fails before it has any output leaves
// nothing behind, and it only gets its final name once complete.
type stagedPDF struct {
	folder string
	path   string
	file   *os.File
}

func (s *stagedPDF) Write(p []byte) (int, error) {
	if s.file == nil {
		if err := s.create(); err != nil {
			return 0, err
		}
	}
	return s.file.Write(p)
}

func (s *stagedPDF) create() error {
	path, err := allocatePDFJobOutput(s.folder)
	if err != nil {
		return err
	}
	s.path = path
	s.file, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	return err
}

// publish closes the file, checks that it is a complete PDF and gives it its
// final name in the folder, derived from jobName. It returns the final path.
func (s *stagedPDF) publish(jobName string) (string, error) {
	if s.file == nil {
		return "", fmt.Errorf("PDF output was not written")
	}
	syncErr := s.file.Sync()
	closeErr := s.file.Close()
	s.file = nil
	if syncErr != nil || closeErr != nil {
		return "", fmt.Errorf("write PDF output: %w", errors.Join(syncErr, closeErr))
	}
	if !completePDFOutput(s.path) {
		return "", fmt.Errorf("PDF output %q is incomplete", s.path)
	}
	saved, err := publishFolderPDF(s.path, s.folder, jobName)
	if err != nil {
		return "", fmt.Errorf("save PDF from %q: %w", s.path, err)
	}
	s.path = ""
	return saved, nil
}

// discard removes whatever the job left in the staging directory. It does
// nothing once the output has been published.
func (s *stagedPDF) discard() {
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	if s.path == "" {
		return
	}
	if err := os.RemoveAll(filepath.Dir(s.path)); err != nil {
		componentLogger(componentPDF).Warn("remove staged pdf failed", "path", s.path, "err", err)
	}
	s.path = ""
}
