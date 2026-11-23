//go:build windows

package core

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func TestIsRetryableReplaceError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{windows.ERROR_ACCESS_DENIED, true},
		{windows.ERROR_SHARING_VIOLATION, true},
		{windows.ERROR_FILE_NOT_FOUND, false},
		{errors.New("other"), false},
	}
	for _, tc := range tests {
		if got := isRetryableReplaceError(tc.err); got != tc.want {
			t.Errorf("isRetryableReplaceError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
