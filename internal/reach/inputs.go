package reach

import (
	"errors"
	"net"
)

// Windows firewall profile bits as Get-NetFirewallRule reports them.
const (
	profileDomain  = 1
	profilePrivate = 2
	profilePublic  = 4
	// profileAny is "all profiles"; the plain zero value means the same.
	profileAny = 2147483647
)

// Network profile names, as Get-NetFirewallProfile and the UI name them.
const (
	ProfileDomain  = "Domain"
	ProfilePrivate = "Private"
	ProfilePublic  = "Public"
)

// ConnProfile is one connected network (Get-NetConnectionProfile).
type ConnProfile struct {
	InterfaceAlias   string
	NetworkCategory  string // Public, Private or DomainAuthenticated
	IPv4Connectivity string
}

// FirewallProfile is the firewall's setting for one profile
// (Get-NetFirewallProfile).
type FirewallProfile struct {
	Name                 string // Domain, Private or Public
	Enabled              string // True, False or NotConfigured
	DefaultInboundAction string // Block, Allow or NotConfigured
}

// Rule is one enabled inbound firewall rule with its filters joined in.
type Rule struct {
	Name          string
	DisplayName   string
	Action        string // Allow or Block
	Profile       int    // bitmask; see the profile constants
	Protocol      string
	LocalPort     []string
	RemoteAddress []string
	Program       string
}

// PortOwner is a process listening on the port: its name and ID only.
type PortOwner struct {
	ProcessName string
	ProcessID   int
}

// Listen is what the bridge itself knows about its server.
type Listen struct {
	Attempted bool
	Listening bool
	Addr      string
	Err       string
}

// Inputs is everything the classifier looks at. Facts are what the
// collector read from Windows; the rest comes from the app itself.
type Inputs struct {
	Facts
	Port     int
	ExePath  string
	OwnPID   int
	LocalNet *net.IPNet // the subnet of the LAN address, nil when unknown
	Listen   Listen
}

// Facts are the raw reads of the Windows side.
type Facts struct {
	Profiles []ConnProfile
	Firewall []FirewallProfile
	Rules    []Rule
	Owners   []PortOwner
}

// ErrUnavailable means the facts cannot be collected on this system.
var ErrUnavailable = errors.New("network facts are not available on this system")
