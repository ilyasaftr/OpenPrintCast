//go:build !windows

package winfw

import "context"

// netshRunner has no netsh to run off Windows.
type netshRunner struct{}

func (netshRunner) Run(...string) (string, error) { return "", ErrUnsupported }

func isElevated() bool { return false }

func runElevated(context.Context, string) (uint32, error) { return 0, ErrUnsupported }
