//go:build windows

package winfw

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ShellExecuteEx flags and the wait for the elevated copy.
const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	swHide                = 0
	waitStep              = 200 * time.Millisecond
	// elevatedRunLimit bounds the elevated copy's run once it has started:
	// the prompt itself is answered before ShellExecuteEx returns.
	elevatedRunLimit = time.Minute
)

var procShellExecuteEx = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	size       uint32
	mask       uint32
	hwnd       windows.Handle
	verb       *uint16
	file       *uint16
	parameters *uint16
	directory  *uint16
	show       int32
	instApp    windows.Handle
	idList     uintptr
	class      *uint16
	keyClass   windows.Handle
	hotKey     uint32
	icon       windows.Handle
	process    windows.Handle
}

// netshRunner runs netsh without a console window.
type netshRunner struct{}

func (netshRunner) Run(args ...string) (string, error) {
	cmd := exec.Command("netsh", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func isElevated() bool { return windows.GetCurrentProcessToken().IsElevated() }

// runElevated starts this executable with the "runas" verb and args, waits
// for it and returns its exit code. Declining the prompt is ErrCancelled.
func runElevated(ctx context.Context, args string) (uint32, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, err
	}
	process, err := shellExecuteRunAs(exe, args)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(process)
	return waitForProcess(ctx, process)
}

func shellExecuteRunAs(file, args string) (windows.Handle, error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	filePtr, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return 0, err
	}
	paramPtr, err := windows.UTF16PtrFromString(args)
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{mask: seeMaskNoCloseProcess | seeMaskNoAsync, verb: verb, file: filePtr, parameters: paramPtr, show: swHide}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return 0, ErrCancelled
		}
		return 0, fmt.Errorf("could not start the elevated setup: %w", callErr)
	}
	return info.process, nil
}

// waitForProcess waits for the process to end, for ctx, or for the limit.
func waitForProcess(ctx context.Context, process windows.Handle) (uint32, error) {
	deadline := time.Now().Add(elevatedRunLimit)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return 0, ctx.Err()
		}
		event, err := windows.WaitForSingleObject(process, uint32(waitStep/time.Millisecond))
		if err != nil {
			return 0, err
		}
		if event == windows.WAIT_OBJECT_0 {
			var code uint32
			err := windows.GetExitCodeProcess(process, &code)
			return code, err
		}
	}
	return 0, errors.New("the elevated setup did not finish in time")
}
