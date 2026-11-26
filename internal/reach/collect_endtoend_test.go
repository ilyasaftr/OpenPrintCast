package reach

import (
	"net"
	"strings"
	"testing"
)

// comFactsFixture is what the collection script prints on a typical PC:
// the network category from the Network List Manager, the per-profile
// firewall state and the relevant rules from HNetCfg.FwPolicy2. The rules
// cover the spellings COM returns: "*" normalized to Any, null ports and
// addresses as Any, a comma-separated port list and numeric protocol and
// action values already mapped by the script.
const comFactsFixture = `{
	"Profiles": [{"InterfaceAlias": "Wi-Fi", "NetworkCategory": "Private", "IPv4Connectivity": "Internet"}],
	"Firewall": [
		{"Name": "Domain", "Enabled": "True", "DefaultInboundAction": "Block"},
		{"Name": "Private", "Enabled": "True", "DefaultInboundAction": "Block"},
		{"Name": "Public", "Enabled": "True", "DefaultInboundAction": "Block"}
	],
	"Rules": [
		{"Name": "OpenPrintCast (IPP)", "DisplayName": "OpenPrintCast (IPP)", "Action": "Allow", "Profile": 2147483647,
			"Protocol": "6", "LocalPort": ["631"], "RemoteAddress": ["LocalSubnet"],
			"Program": "C:\\Program Files\\OpenPrintCast\\openprintcast.exe"},
		{"Name": "Keyword RPC rule", "DisplayName": "Keyword RPC rule", "Action": "Allow", "Profile": 2,
			"Protocol": "6", "LocalPort": ["Any"], "RemoteAddress": ["Any"], "Program": ""}
	],
	"Owner": []
}`

// The output of the collection script parses and classifies end to end:
// the program rule for the bridge's own executable makes the private
// profile reachable.
func TestComFactsParseAndAssess(t *testing.T) {
	facts, err := ParseFacts([]byte(comFactsFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(facts.Profiles) != 1 || facts.Profiles[0].NetworkCategory != "Private" {
		t.Fatalf("profiles = %+v", facts.Profiles)
	}
	if len(facts.Firewall) != 3 || facts.Firewall[1].DefaultInboundAction != "Block" {
		t.Fatalf("firewall = %+v", facts.Firewall)
	}
	exe := `C:\Program Files\OpenPrintCast\openprintcast.exe`
	rules := 0
	for _, r := range facts.Rules {
		if strings.EqualFold(normalizePath(r.Program), normalizePath(exe)) {
			rules++
		}
	}
	if rules != 1 {
		t.Fatalf("program rules = %d, want 1: %+v", rules, facts.Rules)
	}
	_, subnet, _ := net.ParseCIDR("192.168.1.0/24")
	a := Assess(Inputs{
		Facts: facts, Port: 631, ExePath: exe, LocalNet: subnet,
		Listen: Listen{Attempted: true, Listening: true},
	})
	if a.State != StateOK {
		t.Fatalf("state = %v (%s), want ok: the program rule allows the port", a.State, a.Detail)
	}
}

// An empty facts document, printed when every COM read failed, parses
// into a clean "unknown" verdict rather than an error.
func TestEmptyFactsMeanUnknown(t *testing.T) {
	facts, err := ParseFacts([]byte(`{"Profiles":[],"Firewall":[],"Rules":[],"Owner":[]}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	a := Assess(Inputs{Facts: facts, Port: 631, Listen: Listen{Attempted: true, Listening: true}})
	if a.State != StateUnknown {
		t.Fatalf("state = %v, want unknown", a.State)
	}
}
