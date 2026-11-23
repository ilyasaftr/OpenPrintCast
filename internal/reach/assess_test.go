package reach

import (
	"net"
	"reflect"
	"testing"
)

func conn(category string) ConnProfile {
	return ConnProfile{InterfaceAlias: "Wi-Fi", NetworkCategory: category, IPv4Connectivity: "Internet"}
}

// firewallOn is the default: every profile on, inbound blocked.
func firewallOn() []FirewallProfile {
	return []FirewallProfile{
		{Name: "Domain", Enabled: "True", DefaultInboundAction: "Block"},
		{Name: "Private", Enabled: "True", DefaultInboundAction: "Block"},
		{Name: "Public", Enabled: "True", DefaultInboundAction: "Block"},
	}
}

// installerRule is what the installer's netsh line creates.
func installerRule() Rule {
	return Rule{Name: "{1}", DisplayName: "OpenPrintCast - IPP (TCP 631)", Action: "Allow", Profile: profileAny,
		Protocol: "TCP", LocalPort: []string{"631"}, RemoteAddress: []string{"LocalSubnet"}, Program: "Any"}
}

// oldPrivateRule is what earlier builds created: private only.
func oldPrivateRule() Rule {
	return Rule{Name: "{2}", DisplayName: "OpenPrintCast (TCP)", Action: "Allow", Profile: profilePrivate,
		Protocol: "TCP", LocalPort: []string{"631"}, RemoteAddress: []string{"Any"}, Program: "Any"}
}

// promptRule is what the Windows prompt creates for the program.
func promptRule(action string, profile int) Rule {
	return Rule{Name: "{3}", DisplayName: "openprintcast.exe", Action: action, Profile: profile,
		Protocol: "TCP", LocalPort: []string{"Any"}, RemoteAddress: []string{"Any"}, Program: bridgeExe}
}

func inputsWith(profiles []ConnProfile, rules ...Rule) Inputs {
	_, local, _ := net.ParseCIDR("192.168.1.0/24")
	return Inputs{
		Facts: Facts{Profiles: profiles, Firewall: firewallOn(), Rules: rules},
		Port:  631, ExePath: bridgeExe, OwnPID: 100, LocalNet: local,
		Listen: Listen{Attempted: true, Listening: true, Addr: "[::]:631"},
	}
}

func TestAssessVerdicts(t *testing.T) {
	tests := []struct {
		name  string
		in    Inputs
		state State
		cause Cause
		fix   Fix
		alt   Fix
		prof  string
	}{
		{"private network with the installer rule", inputsWith([]ConnProfile{conn("Private")}, installerRule()), StateOK, CauseNone, FixNone, FixNone, ""},
		{"public network with the installer rule (profile any)", inputsWith([]ConnProfile{conn("Public")}, installerRule()), StateOK, CauseNone, FixNone, FixNone, ""},
		{"public network with a private-only rule", inputsWith([]ConnProfile{conn("Public")}, oldPrivateRule()),
			StateBlocked, CausePublicNetwork, FixOpenNetworkSettings, FixAllowOnPublic, "Public"},
		{"private network with a private-only rule", inputsWith([]ConnProfile{conn("Private")}, oldPrivateRule()), StateOK, CauseNone, FixNone, FixNone, ""},
		{"private network without any rule", inputsWith([]ConnProfile{conn("Private")}), StateBlocked, CauseNoRule, FixAllowFirewall, FixNone, "Private"},
		{"domain network without any rule", inputsWith([]ConnProfile{conn("DomainAuthenticated")}), StateBlocked, CauseNoRule, FixAllowFirewall, FixNone, "Domain"},
		{"public network without any rule", inputsWith([]ConnProfile{conn("Public")}), StateBlocked, CausePublicNetwork, FixOpenNetworkSettings, FixAllowOnPublic, "Public"},
		{"program rule made by the Windows prompt", inputsWith([]ConnProfile{conn("Private")}, promptRule("Allow", profilePrivate|profilePublic)), StateOK, CauseNone, FixNone, FixNone, ""},
		{"block rules from a dismissed prompt", inputsWith([]ConnProfile{conn("Private")}, promptRule("Block", profilePrivate|profilePublic)),
			StateBlocked, CauseBlockRule, FixAllowFirewall, FixNone, "Private"},
		{"a block rule overrides the allow rule", inputsWith([]ConnProfile{conn("Private")}, installerRule(), promptRule("Block", profileAny)),
			StateBlocked, CauseBlockRule, FixAllowFirewall, FixNone, "Private"},
		{"a block rule for every program cannot be fixed here", inputsWith([]ConnProfile{conn("Private")}, installerRule(),
			Rule{Name: "b", DisplayName: "Block 631", Action: "Block", Profile: profileAny, Protocol: "TCP", LocalPort: []string{"631"}, RemoteAddress: []string{"Any"}, Program: "Any"}),
			StateBlocked, CauseBlockRule, FixNone, FixNone, "Private"},
		{"block rule on another profile does not matter", inputsWith([]ConnProfile{conn("Private")}, installerRule(), promptRule("Block", profilePublic)), StateOK, CauseNone, FixNone, FixNone, ""},
		{"rule of another program", inputsWith([]ConnProfile{conn("Private")}, Rule{Name: "x", Action: "Allow", Profile: profileAny, Protocol: "TCP",
			LocalPort: []string{"631"}, RemoteAddress: []string{"Any"}, Program: `C:\Other\cupsd.exe`}), StateBlocked, CauseNoRule, FixAllowFirewall, FixNone, "Private"},
		{"rule for udp only", inputsWith([]ConnProfile{conn("Private")}, Rule{Name: "u", Action: "Allow", Profile: profileAny, Protocol: "UDP",
			LocalPort: []string{"631"}, RemoteAddress: []string{"Any"}, Program: "Any"}), StateBlocked, CauseNoRule, FixAllowFirewall, FixNone, "Private"},
		{"rule for another port", inputsWith([]ConnProfile{conn("Private")}, Rule{Name: "p", Action: "Allow", Profile: profileAny, Protocol: "TCP",
			LocalPort: []string{"5353"}, RemoteAddress: []string{"Any"}, Program: "Any"}), StateBlocked, CauseNoRule, FixAllowFirewall, FixNone, "Private"},
		{"port range rule", inputsWith([]ConnProfile{conn("Private")}, withPorts(installerRule(), "600-700")), StateOK, CauseNone, FixNone, FixNone, ""},
		{"comma list rule", inputsWith([]ConnProfile{conn("Private")}, withPorts(installerRule(), "80,631,8080")), StateOK, CauseNone, FixNone, FixNone, ""},
		{"any port rule", inputsWith([]ConnProfile{conn("Private")}, withPorts(installerRule(), "Any")), StateOK, CauseNone, FixNone, FixNone, ""},
		{"rule for another subnet only", inputsWith([]ConnProfile{conn("Private")}, withRemote(installerRule(), "10.0.0.0/8")), StateBlocked, CauseNoRule, FixAllowFirewall, FixNone, "Private"},
		{"rule for a range around the local subnet", inputsWith([]ConnProfile{conn("Private")}, withRemote(installerRule(), "192.168.0.0/16")), StateOK, CauseNone, FixNone, FixNone, ""},
		{"rule for a host only is not decided", inputsWith([]ConnProfile{conn("Private")}, withRemote(installerRule(), "192.168.1.9")), StateUnknown, CauseUnknown, FixNone, FixNone, ""},
		{"firewall off", withFirewall(inputsWith([]ConnProfile{conn("Public")}), "Public", "False", "Block"), StateOK, CauseNone, FixNone, FixNone, ""},
		{"default inbound allow", withFirewall(inputsWith([]ConnProfile{conn("Private")}), "Private", "True", "Allow"), StateOK, CauseNone, FixNone, FixNone, ""},
		{"firewall off but a block rule", withFirewall(inputsWith([]ConnProfile{conn("Private")}, promptRule("Block", profileAny)), "Private", "False", "Block"), StateOK, CauseNone, FixNone, FixNone, ""},
		{"several interfaces, all covered", inputsWith([]ConnProfile{conn("Private"), conn("Public")}, installerRule()), StateOK, CauseNone, FixNone, FixNone, ""},
		{"several interfaces, public not covered", inputsWith([]ConnProfile{conn("Private"), conn("Public")}, oldPrivateRule()),
			StateBlocked, CausePublicNetwork, FixOpenNetworkSettings, FixAllowOnPublic, "Public"},
		{"several interfaces, one profile twice", inputsWith([]ConnProfile{conn("Private"), conn("Private")}, oldPrivateRule()), StateOK, CauseNone, FixNone, FixNone, ""},
		{"a disconnected interface is not active", inputsWith([]ConnProfile{conn("Private"), {InterfaceAlias: "VPN", NetworkCategory: "Public", IPv4Connectivity: "Disconnected"}}, oldPrivateRule()), StateOK, CauseNone, FixNone, FixNone, ""},
		{"no connected network", inputsWith(nil, installerRule()), StateUnknown, CauseUnknown, FixNone, FixNone, ""},
		{"unknown network category", inputsWith([]ConnProfile{conn("Strange")}, installerRule()), StateUnknown, CauseUnknown, FixNone, FixNone, ""},
		{"no firewall profiles listed", func() Inputs { in := inputsWith([]ConnProfile{conn("Private")}); in.Firewall = nil; return in }(), StateUnknown, CauseUnknown, FixNone, FixNone, ""},
		{"profile missing from the firewall list", func() Inputs {
			in := inputsWith([]ConnProfile{conn("Public")}, installerRule())
			in.Firewall = in.Firewall[:2]
			return in
		}(), StateUnknown, CauseUnknown, FixNone, FixNone, ""},
		{"program rule when our own path is unknown", func() Inputs {
			in := inputsWith([]ConnProfile{conn("Private")}, promptRule("Allow", profileAny))
			in.ExePath = ""
			return in
		}(), StateUnknown, CauseUnknown, FixNone, FixNone, ""},
		{"local subnet unknown with a specific range", func() Inputs {
			in := inputsWith([]ConnProfile{conn("Private")}, withRemote(installerRule(), "192.168.1.0/24"))
			in.LocalNet = nil
			return in
		}(), StateUnknown, CauseUnknown, FixNone, FixNone, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Assess(tc.in)
			if got.State != tc.state || got.Cause != tc.cause || got.Fix != tc.fix || got.AltFix != tc.alt || got.Profile != tc.prof {
				t.Fatalf("got %v fix=%d alt=%d, want state=%v cause=%v fix=%d alt=%d profile=%q",
					got, got.Fix, got.AltFix, tc.state, tc.cause, tc.fix, tc.alt, tc.prof)
			}
		})
	}
}

func withPorts(r Rule, ports ...string) Rule { r.LocalPort = ports; return r }

func withRemote(r Rule, addrs ...string) Rule { r.RemoteAddress = addrs; return r }

func withFirewall(in Inputs, name, enabled, action string) Inputs {
	in.Firewall = firewallOn()
	for i := range in.Firewall {
		if in.Firewall[i].Name == name {
			in.Firewall[i].Enabled, in.Firewall[i].DefaultInboundAction = enabled, action
		}
	}
	return in
}

func TestAssessNotListening(t *testing.T) {
	base := inputsWith([]ConnProfile{conn("Private")}, installerRule())
	base.Listen = Listen{Attempted: true, Err: "the port is already in use by another program"}

	t.Run("another program holds the port", func(t *testing.T) {
		in := base
		in.Owners = []PortOwner{{ProcessName: "cupsd", ProcessID: 4242}}
		got := Assess(in)
		if got.State != StateNotListening || got.Cause != CauseOtherProgramOnPort || got.Owner != "cupsd" {
			t.Fatalf("got %v owner=%q", got, got.Owner)
		}
	})
	t.Run("owner unknown", func(t *testing.T) {
		got := Assess(base)
		if got.State != StateNotListening || got.Cause != CauseBindFailed || got.Detail != base.Listen.Err {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("the owner is this very process", func(t *testing.T) {
		in := base
		in.Owners = []PortOwner{{ProcessName: "openprintcast", ProcessID: 100}}
		if got := Assess(in); got.Cause != CauseBindFailed || got.Owner != "" {
			t.Fatalf("got %v owner=%q", got, got.Owner)
		}
	})
	t.Run("an owner without a name is not named", func(t *testing.T) {
		in := base
		in.Owners = []PortOwner{{ProcessID: 4242}}
		if got := Assess(in); got.Cause != CauseBindFailed {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("not listening wins over a blocked firewall", func(t *testing.T) {
		in := inputsWith([]ConnProfile{conn("Public")})
		in.Listen = base.Listen
		if got := Assess(in); got.State != StateNotListening {
			t.Fatalf("got %v", got)
		}
	})
	t.Run("a server that has not tried yet is not accused", func(t *testing.T) {
		in := base
		in.Listen = Listen{}
		if got := Assess(in); got.State != StateOK {
			t.Fatalf("got %v", got)
		}
	})
}

func TestAssessReportsWhatItUsed(t *testing.T) {
	got := Assess(inputsWith([]ConnProfile{conn("Private"), conn("Public")}, installerRule(), oldPrivateRule()))
	if !reflect.DeepEqual(got.ActiveProfiles, []string{"Private", "Public"}) {
		t.Fatalf("profiles = %v", got.ActiveProfiles)
	}
	if !reflect.DeepEqual(got.Rules, []string{"OpenPrintCast - IPP (TCP 631)", "OpenPrintCast (TCP)"}) {
		t.Fatalf("rules = %v", got.Rules)
	}
}

func TestAssessmentProblem(t *testing.T) {
	for state, want := range map[State]bool{StateUnknown: false, StateOK: false, StateBlocked: true, StateNotListening: true} {
		if got := (Assessment{State: state}).Problem(); got != want {
			t.Errorf("Problem(%v) = %v, want %v", state, got, want)
		}
	}
	if (Assessment{}).State != StateUnknown {
		t.Fatal("the zero Assessment must claim nothing")
	}
}
