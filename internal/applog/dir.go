package applog

import (
	"os"
	"path/filepath"
)

const appDirName = "OpenPrintCast"

// tempDir is a seam for tests; production code uses os.TempDir.
var tempDir = os.TempDir

// DefaultDir is %LOCALAPPDATA%\OpenPrintCast when LOCALAPPDATA is set,
// otherwise OpenPrintCast under the OS temp dir. It never panics.
func DefaultDir() string {
	if base := os.Getenv("LOCALAPPDATA"); base != "" {
		return filepath.Join(base, appDirName)
	}
	return fallbackDir()
}

func fallbackDir() string { return filepath.Join(tempDir(), appDirName) }
