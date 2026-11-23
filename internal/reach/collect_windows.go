//go:build windows

package reach

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// powershellArgs run collectScript without profile, prompts or policy.
func powershellArgs() []string {
	return []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", collectScript}
}

// PowerShell is the Collector that asks Windows through powershell.exe.
type PowerShell struct{}

// Collect runs the script with ctx's deadline and parses its output. Any
// failure (PowerShell missing, constrained language mode, timeout, group
// policy) is an error, which the caller treats as "couldn't check".
func (PowerShell) Collect(ctx context.Context) (Facts, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe", powershellArgs()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Facts{}, fmt.Errorf("the network query took too long: %w", ctx.Err())
		}
		return Facts{}, fmt.Errorf("the network query failed: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return ParseFacts(stdout.Bytes())
}

// NewCollector is the production Collector.
func NewCollector() Collector { return PowerShell{} }
