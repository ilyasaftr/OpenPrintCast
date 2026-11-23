package reach

import (
	"net"
	"testing"
)

func TestMatchesPort(t *testing.T) {
	tests := []struct {
		entries []string
		want    bool
	}{
		{[]string{"631"}, true},
		{[]string{"Any"}, true},
		{[]string{"any"}, true},
		{[]string{"80", "631"}, true},
		{[]string{"80,631,443"}, true},
		{[]string{"80, 631"}, true},
		{[]string{"600-700"}, true},
		{[]string{"631-631"}, true},
		{[]string{"632-700"}, false},
		{[]string{"1-630"}, false},
		{[]string{"6310"}, false},
		{[]string{"RPC", "RPC-EPMap"}, false},
		{[]string{"5353"}, false},
		{[]string{"a-b"}, false},
		{nil, false},
	}
	for _, tc := range tests {
		if got := matchesPort(tc.entries, 631); got != tc.want {
			t.Errorf("matchesPort(%q) = %v, want %v", tc.entries, got, tc.want)
		}
	}
}

func TestMatchesTCP(t *testing.T) {
	for protocol, want := range map[string]bool{"TCP": true, "tcp": true, "Any": true, "6": true, "256": true, "UDP": false, "17": false, "": false} {
		if got := matchesTCP(protocol); got != want {
			t.Errorf("matchesTCP(%q) = %v, want %v", protocol, got, want)
		}
	}
}

func TestProfileCovers(t *testing.T) {
	tests := []struct {
		mask, bit int
		want      bool
	}{
		{profileAny, profilePublic, true}, {0, profilePrivate, true}, {profilePrivate, profilePrivate, true},
		{profilePrivate, profilePublic, false}, {profileDomain | profilePrivate, profilePrivate, true},
		{profileDomain | profilePrivate, profilePublic, false}, {profilePublic, profileDomain, false},
	}
	for _, tc := range tests {
		if got := profileCovers(tc.mask, tc.bit); got != tc.want {
			t.Errorf("profileCovers(%d, %d) = %v, want %v", tc.mask, tc.bit, got, tc.want)
		}
	}
}

func TestMatchesProgram(t *testing.T) {
	tests := []struct {
		program, exe string
		want         tri
	}{
		{"Any", bridgeExe, yes}, {"", bridgeExe, yes}, {bridgeExe, bridgeExe, yes},
		{`c:\program files\openprintcast\OPENPRINTCAST.EXE`, bridgeExe, yes},
		{"C:/Program Files/OpenPrintCast/openprintcast.exe", bridgeExe, yes},
		{`C:\Other\cupsd.exe`, bridgeExe, no},
		{`C:\Other\cupsd.exe`, "", unsure}, {"Any", "", yes},
	}
	for _, tc := range tests {
		if got := matchesProgram(tc.program, tc.exe); got != tc.want {
			t.Errorf("matchesProgram(%q, %q) = %v, want %v", tc.program, tc.exe, got, tc.want)
		}
	}
}

func mustNet(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	n.IP = ip.Mask(n.Mask)
	return n
}

func TestCoversSubnet(t *testing.T) {
	local := mustNet(t, "192.168.1.5/24")
	tests := []struct {
		name    string
		entries []string
		local   *net.IPNet
		want    tri
	}{
		{"no filter", nil, local, yes},
		{"any", []string{"Any"}, local, yes},
		{"local subnet", []string{"LocalSubnet"}, nil, yes},
		{"cidr containing", []string{"192.168.0.0/16"}, local, yes},
		{"cidr equal", []string{"192.168.1.0/24"}, local, yes},
		{"mask form", []string{"192.168.1.0/255.255.255.0"}, local, yes},
		{"range containing", []string{"192.168.1.1-192.168.1.254"}, local, yes},
		{"range inside the subnet only", []string{"192.168.1.10-192.168.1.20"}, local, unsure},
		{"single host", []string{"192.168.1.9"}, local, unsure},
		{"other subnet", []string{"10.0.0.0/8"}, local, no},
		{"other range", []string{"10.0.0.1-10.0.0.9"}, local, no},
		{"one of several matches", []string{"10.0.0.0/8", "LocalSubnet"}, local, yes},
		{"ipv6 only", []string{"fe80::/64"}, local, no},
		{"named set", []string{"DefaultGateway"}, local, unsure},
		{"unknown local net", []string{"192.168.1.0/24"}, nil, unsure},
		{"garbage", []string{"not an address"}, local, unsure},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := coversSubnet(tc.entries, tc.local); got != tc.want {
				t.Fatalf("coversSubnet(%q) = %v, want %v", tc.entries, got, tc.want)
			}
		})
	}
}
