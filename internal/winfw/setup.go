package winfw

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// Names of the rules the bridge manages. They differ from the installer's
// ("OpenPrintCast - IPP (TCP 631)" and its mDNS twin), which stay as the
// installer made them.
const (
	RuleIPP  = "OpenPrintCast (IPP)"
	RuleMDNS = "OpenPrintCast (mDNS)"
)

// legacyRules are names earlier builds used; setup removes them.
var legacyRules = []string{"OpenPrintCast (TCP)", "OpenPrintCast (UDP)"}

const (
	mdnsPort        = 5353
	ruleDescription = "OpenPrintCast local printing"
	profilesPrivate = "domain,private"
	profilesPublic  = "domain,private,public"
)

// Runner runs one netsh command and returns its combined output.
type Runner interface {
	Run(args ...string) (output string, err error)
}

// Setup replaces the bridge's firewall rules with the current ones: IPP on
// port and mDNS, inbound, from the local subnet only, on the Domain and
// Private profiles, plus Public when allowPublic. exe is the bridge's own
// program, whose leftover rules (a dismissed Windows prompt creates block
// rules for it) are deleted too. Deleting what is not there is not an error.
func Setup(run Runner, exe string, port int, allowPublic bool) error {
	for _, args := range removalCommands(exe) {
		if out, err := run.Run(args...); err != nil {
			slog.Debug("firewall rule not removed", "component", "firewall", "args", strings.Join(args, " "), "err", err, "output", out)
		}
	}
	var failures []error
	for _, args := range addCommands(port, allowPublic) {
		if out, err := run.Run(args...); err != nil {
			slog.Error("firewall rule not added", "component", "firewall", "args", strings.Join(args, " "), "err", err, "output", strings.TrimSpace(out))
			failures = append(failures, fmt.Errorf("%s: %w", args[4], err))
			continue
		}
		slog.Info("firewall rule added", "component", "firewall", "rule", args[4])
	}
	return errors.Join(failures...)
}

func netsh(verb string, rest ...string) []string {
	return append([]string{"advfirewall", "firewall", verb, "rule"}, rest...)
}

// removalCommands delete the rules by exact name, then the rules tied to
// the bridge's own program.
func removalCommands(exe string) [][]string {
	var cmds [][]string
	for _, name := range append([]string{RuleIPP, RuleMDNS}, legacyRules...) {
		cmds = append(cmds, netsh("delete", "name="+name))
	}
	if exe != "" {
		cmds = append(cmds, netsh("delete", "name=all", "dir=in", "program="+exe))
	}
	return cmds
}

// addCommands add the two allow rules.
func addCommands(port int, allowPublic bool) [][]string {
	profiles := profilesPrivate
	if allowPublic {
		profiles = profilesPublic
	}
	rule := func(name, proto string, port int) []string {
		return netsh("add", "name="+name, "dir=in", "action=allow", "protocol="+proto,
			fmt.Sprintf("localport=%d", port), "remoteip=localsubnet", "profile="+profiles,
			"description="+ruleDescription)
	}
	return [][]string{rule(RuleIPP, "TCP", port), rule(RuleMDNS, "UDP", mdnsPort)}
}
