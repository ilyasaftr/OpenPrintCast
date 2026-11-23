package winfw

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// ErrCancelled means the user declined the administrator prompt.
var ErrCancelled = errors.New("the administrator prompt was declined")

// ErrUnsupported means the firewall cannot be changed on this system.
var ErrUnsupported = errors.New("changing Windows Firewall is only possible on Windows")

// RunSetup applies Setup with the real netsh in this process, which must be
// elevated for the changes to take effect.
func RunSetup(port int, allowPublic bool) error {
	exe, _ := os.Executable()
	return Setup(netshRunner{}, exe, port, allowPublic)
}

// Fixer applies the firewall fix: directly when this process already has
// administrator rights, else through an elevated copy of the app that the
// user approves in the Windows prompt.
type Fixer struct {
	Port int
}

// Elevated reports whether this process runs with administrator rights.
func (Fixer) Elevated() bool { return isElevated() }

// Apply runs the fix and returns when it is done. ErrCancelled means the
// user declined the prompt.
func (f Fixer) Apply(ctx context.Context, allowPublic bool) error {
	if isElevated() {
		return RunSetup(f.Port, allowPublic)
	}
	code, err := runElevated(ctx, elevatedArgs(allowPublic))
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the firewall setup failed (exit code %d)", code)
	}
	return nil
}

// elevatedArgs is the constant command line of the elevated copy.
func elevatedArgs(allowPublic bool) string {
	if allowPublic {
		return SetupFlag + " " + AllowPublicFlag
	}
	return SetupFlag
}
