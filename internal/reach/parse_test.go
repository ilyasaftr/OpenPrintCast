package reach

import (
	"reflect"
	"strings"
	"testing"
)

const bridgeExe = `C:\Program Files\OpenPrintCast\openprintcast.exe`

func TestParseFactsShapes(t *testing.T) {
	tests := []struct {
		name   string
		json   string
		check  func(t *testing.T, f Facts)
		hasErr bool
	}{
		{"arrays", `{"Profiles":[{"InterfaceAlias":"Wi-Fi","NetworkCategory":"Public","IPv4Connectivity":"Internet"},{"InterfaceAlias":"Ethernet","NetworkCategory":"Private","IPv4Connectivity":"Internet"}],
			"Firewall":[{"Name":"Public","Enabled":"True","DefaultInboundAction":"Block"}],"Rules":[],"Owner":[]}`,
			func(t *testing.T, f Facts) {
				if len(f.Profiles) != 2 || f.Profiles[0].NetworkCategory != "Public" || len(f.Firewall) != 1 || len(f.Rules) != 0 {
					t.Fatalf("facts = %+v", f)
				}
			}, false},
		{"single objects are not wrapped", `{"Profiles":{"InterfaceAlias":"Wi-Fi","NetworkCategory":"Private","IPv4Connectivity":"Internet"},
			"Firewall":{"Name":"Private","Enabled":"True","DefaultInboundAction":"Block"},
			"Rules":{"Name":"r1","DisplayName":"OpenPrintCast (IPP)","Action":"Allow","Profile":2,"Protocol":"TCP","LocalPort":"631","RemoteAddress":"LocalSubnet","Program":"Any"},
			"Owner":{"ProcessName":"cupsd","ProcessId":4242}}`,
			func(t *testing.T, f Facts) {
				if len(f.Profiles) != 1 || len(f.Firewall) != 1 || len(f.Rules) != 1 || len(f.Owners) != 1 {
					t.Fatalf("facts = %+v", f)
				}
				want := Rule{Name: "r1", DisplayName: "OpenPrintCast (IPP)", Action: "Allow", Profile: 2, Protocol: "TCP",
					LocalPort: []string{"631"}, RemoteAddress: []string{"LocalSubnet"}, Program: "Any"}
				if !reflect.DeepEqual(f.Rules[0], want) {
					t.Fatalf("rule = %+v, want %+v", f.Rules[0], want)
				}
				if f.Owners[0] != (PortOwner{ProcessName: "cupsd", ProcessID: 4242}) {
					t.Fatalf("owner = %+v", f.Owners[0])
				}
			}, false},
		{"null lists", `{"Profiles":null,"Firewall":null,"Rules":null,"Owner":null}`,
			func(t *testing.T, f Facts) {
				if len(f.Profiles)+len(f.Firewall)+len(f.Rules)+len(f.Owners) != 0 {
					t.Fatalf("facts = %+v", f)
				}
			}, false},
		{"missing lists", `{}`, func(t *testing.T, f Facts) {}, false},
		{"byte order mark and padding", "\xef\xbb\xbf  {\"Profiles\":[]}\r\n", func(t *testing.T, f Facts) {}, false},
		{"numeric strings and action numbers", `{"Rules":[{"Name":"b","Action":"4","Profile":"2147483647","Protocol":"6","LocalPort":["631","80"],"RemoteAddress":["Any"],"Program":""}]}`,
			func(t *testing.T, f Facts) {
				r := f.Rules[0]
				if r.Action != "Block" || r.Profile != profileAny || len(r.LocalPort) != 2 {
					t.Fatalf("rule = %+v", r)
				}
			}, false},
		{"empty output", ``, nil, true},
		{"null output", `null`, nil, true},
		{"malformed", `{"Profiles":[`, nil, true},
		{"not an object", `[1,2]`, nil, true},
		{"wrong type", `{"Rules":[{"Profile":"many"}]}`, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			facts, err := ParseFacts([]byte(tc.json))
			if (err != nil) != tc.hasErr {
				t.Fatalf("err = %v, want error: %v", err, tc.hasErr)
			}
			if err == nil {
				tc.check(t, facts)
			}
		})
	}
}

func TestCollectScriptIsAConstantWithoutUserData(t *testing.T) {
	for _, want := range []string{"HNetCfg.FwPolicy2", "DCB00C01-570F-4A9B-8D69-199FDBA5723B", "GetNetworks(1)",
		"FirewallEnabled(", "DefaultInboundAction(", "GetCategory()",
		"Get-NetTCPConnection -LocalPort 631 -State Listen -ErrorAction SilentlyContinue", "Get-Process", "UTF8Encoding", "ConvertTo-Json"} {
		if !strings.Contains(collectScript, want) {
			t.Errorf("script lacks %q", want)
		}
	}
	for _, bad := range []string{"%s", "%v", "$env:", "Invoke-Expression", "iex "} {
		if strings.Contains(collectScript, bad) {
			t.Errorf("script contains %q", bad)
		}
	}
}
