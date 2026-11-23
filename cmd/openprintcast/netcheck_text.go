package main

import (
	"fmt"
	"strings"

	"github.com/ilyasaftr/OpenPrintCast/internal/reach"
)

// Banner wording of the reachability check, one headline and one sentence
// per condition. The buttons carry the remedies.
const (
	netUnreachableHeadline  = "Other devices can't reach this PC"
	netNotListeningHeadline = "OpenPrintCast can't accept connections"

	netPublicBody       = "Windows treats this network as Public and blocks incoming connections. Switch it to Private, or allow OpenPrintCast on this network."
	netFirewallBody     = "Windows Firewall is blocking port 631."
	netRuleBlocksBody   = "The Windows Firewall rule “%s” is blocking port 631. Turn that rule off in Windows Security, then check again."
	netPortInUseBody    = "Port 631 is already in use by %s. Quit that program, then check again."
	netCouldNotListen   = "Couldn't start listening: %s. Quit any program that uses port 631, then check again."
	netCouldNotListenAt = "Couldn't start listening on port 631. Quit any program that uses it, then check again."
)

// Wording of the check's results outside the banner.
const (
	netReachableText   = "Other devices can reach this PC"
	netCheckingText    = "Checking…"
	netUnknownText     = "Couldn't check the network"
	netNeverCheckedMsg = "Not checked yet"
	netAttentionSuffix = " · Needs attention"
)

// Button labels of the banner and the Network check row.
const (
	netOpenSettingsLabel = "Open Network Settings"
	netAllowLabel        = "Allow in Windows Firewall"
	netAllowPublicLabel  = "Allow on This Network…"
	netCheckAgainLabel   = "Check Again"
	netCheckingLabel     = "Checking…"
	netCopyLabel         = "Copy Diagnostics"
	netDetailsLabel      = "Details"
	netCheckLabel        = "Network check"
)

// networkBanner is what the Printers page says about a problem.
type networkBanner struct {
	Headline, Body string
	// Primary is the main button's remedy, FixNone meaning Check Again;
	// Alt is the second remedy, if any.
	Primary, Alt reach.Fix
}

// describeNetwork is the banner for a verdict; ok is false when the
// verdict needs none (reachable, or not known).
func describeNetwork(a reach.Assessment) (b networkBanner, ok bool) {
	b = networkBanner{Headline: netUnreachableHeadline, Primary: a.Fix, Alt: a.AltFix}
	switch a.Cause {
	case reach.CausePublicNetwork:
		b.Body = netPublicBody
	case reach.CauseNoRule:
		b.Body = netFirewallBody
	case reach.CauseBlockRule:
		b.Body = netFirewallBody
		if a.Fix == reach.FixNone {
			b.Body = fmt.Sprintf(netRuleBlocksBody, a.Detail)
		}
	case reach.CauseOtherProgramOnPort:
		b.Headline, b.Body = netNotListeningHeadline, fmt.Sprintf(netPortInUseBody, a.Owner)
	case reach.CauseBindFailed:
		b.Headline, b.Body = netNotListeningHeadline, couldNotListen(a.Detail)
	default:
		return networkBanner{}, false
	}
	return b, a.Problem()
}

func couldNotListen(reason string) string {
	if reason == "" {
		return netCouldNotListenAt
	}
	return fmt.Sprintf(netCouldNotListen, reason)
}

// networkStatusText is the one-line result for the Network check row.
func networkStatusText(busy, hasResult bool, a reach.Assessment) string {
	switch {
	case busy:
		return netCheckingText
	case !hasResult:
		return netNeverCheckedMsg
	case a.Problem():
		b, _ := describeNetwork(a)
		return b.Headline
	case a.State == reach.StateOK:
		return netReachableText
	}
	return netUnknownText
}

// diagnosticsFacts are what Copy Diagnostics adds to the verdict; none of
// it names a user or a document.
type diagnosticsFacts struct {
	Version, Host, IP string
	Port              int
	Server            string
}

// diagnosticsText is the plain text Copy Diagnostics puts on the clipboard.
func diagnosticsText(f diagnosticsFacts, a reach.Assessment, checked bool) string {
	profiles, rules := "unknown", "none found"
	if len(a.ActiveProfiles) > 0 {
		profiles = strings.Join(a.ActiveProfiles, ", ")
	}
	if len(a.Rules) > 0 {
		rules = strings.Join(a.Rules, ", ")
	}
	lines := []string{
		"OpenPrintCast network diagnostics",
		"Version: " + f.Version,
		"Computer: " + f.Host,
		"Address: " + orDefault(f.IP, "none"),
		fmt.Sprintf("Port: %d", f.Port),
		"Server: " + f.Server,
		"Active network profiles: " + profiles,
		"Matching firewall rules: " + rules,
		"Result: " + resultLine(a, checked),
	}
	return strings.Join(lines, "\n") + "\n"
}

// resultLine is the verdict in one line of the diagnostics.
func resultLine(a reach.Assessment, checked bool) string {
	if !checked {
		return netNeverCheckedMsg
	}
	line := fmt.Sprintf("%s (%s)", a.State, a.Cause)
	if a.Profile != "" {
		line += ", profile " + a.Profile
	}
	if a.Owner != "" {
		line += ", port used by " + a.Owner
	}
	if a.Detail != "" {
		line += ", " + a.Detail
	}
	return line
}
