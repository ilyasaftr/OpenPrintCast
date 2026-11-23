//go:build windows

package core

import (
	"errors"

	"golang.org/x/sys/windows"
)

// replaceFlags make MoveFileEx overwrite dst and return only once the move
// has been flushed to disk.
const replaceFlags = windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH

// replaceFile moves src over dst with MoveFileEx.
func replaceFile(src, dst string) error {
	from, err := windows.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, replaceFlags)
}

// isRetryableReplaceError reports whether err is the transient failure seen
// while another process (typically an antivirus scanner) holds dst open.
func isRetryableReplaceError(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
