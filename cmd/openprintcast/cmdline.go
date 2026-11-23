package main

import (
	"fmt"
	"os"

	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
	"github.com/ilyasaftr/OpenPrintCast/internal/winfw"
)

// Exit codes of the elevated firewall setup.
const (
	exitOK       = 0
	exitFailed   = 1
	exitBadUsage = 2
)

// setupFirewall is the elevated step; tests replace it. It logs to the
// log file like the app does.
var setupFirewall = func(allowPublic bool) error {
	logs := startLogging()
	defer func() { _ = logs.Close() }()
	componentLogger(componentFirewall).Info("firewall setup starting", "allow_public", allowPublic)
	return winfw.RunSetup(bridgenet.DefaultPort, allowPublic)
}

// runCommandLine handles the command lines that are not "start the app":
// today only the elevated firewall setup. It reports whether the line was
// handled, and the exit code then. It must run before anything that could
// turn the process away (the single-instance lock) or start the UI.
func runCommandLine(args []string) (handled bool, code int) {
	cmd := winfw.ParseCommand(args)
	switch {
	case cmd.Invalid:
		fmt.Fprintf(os.Stderr, "usage: %s [%s]\n", winfw.SetupFlag, winfw.AllowPublicFlag)
		return true, exitBadUsage
	case !cmd.Setup:
		return false, exitOK
	case setupFirewall(cmd.AllowPublic) != nil:
		return true, exitFailed
	}
	return true, exitOK
}
