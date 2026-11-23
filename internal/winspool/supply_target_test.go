package winspool

import "testing"

func TestTargetFromPortSettings(t *testing.T) {
	for name, tc := range map[string]struct {
		settings tcpipPortSettings
		want     snmpTarget
		ok       bool
	}{
		"address and the port's own community": {
			tcpipPortSettings{IPAddress: "192.168.1.50", SNMPEnabled: true, SNMPCommunity: "office", SNMPIndex: 2},
			snmpTarget{Host: "192.168.1.50", Community: "office", DeviceIndex: 2}, true},
		"host name wins over address": {
			tcpipPortSettings{HostName: " printer.lan ", IPAddress: "192.168.1.50", SNMPEnabled: true},
			snmpTarget{Host: "printer.lan", Community: "public", DeviceIndex: 1}, true},
		"missing community defaults to public when SNMP is on": {
			tcpipPortSettings{IPAddress: "10.0.0.7", SNMPEnabled: true, SNMPIndex: 0},
			snmpTarget{Host: "10.0.0.7", Community: "public", DeviceIndex: 1}, true},
		"port named like an address": {
			tcpipPortSettings{PortName: "192.168.1.60", SNMPEnabled: true},
			snmpTarget{Host: "192.168.1.60", Community: "public", DeviceIndex: 1}, true},
		"IPv6 address": {
			tcpipPortSettings{IPAddress: "fe80::1", SNMPEnabled: true},
			snmpTarget{Host: "fe80::1", Community: "public", DeviceIndex: 1}, true},
		"SNMP switched off in the port": {
			tcpipPortSettings{IPAddress: "192.168.1.50", SNMPEnabled: false, SNMPCommunity: "public"},
			snmpTarget{}, false},
		"port name that is not an address and no host": {
			tcpipPortSettings{PortName: "IP_Office", SNMPEnabled: true}, snmpTarget{}, false},
		"unspecified address":           {tcpipPortSettings{IPAddress: "0.0.0.0", SNMPEnabled: true}, snmpTarget{}, false},
		"loopback address":              {tcpipPortSettings{IPAddress: "127.0.0.1", SNMPEnabled: true}, snmpTarget{}, false},
		"multicast address":             {tcpipPortSettings{IPAddress: "224.0.0.251", SNMPEnabled: true}, snmpTarget{}, false},
		"host name with odd characters": {tcpipPortSettings{HostName: "a b/c", SNMPEnabled: true}, snmpTarget{}, false},
		"nothing":                       {tcpipPortSettings{SNMPEnabled: true}, snmpTarget{}, false},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := targetFromPortSettings(tc.settings)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("got %+v, %v; want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Off Windows no printer has a port to read, so no network is ever used.
func TestSystemBackendWithoutSupplySource(t *testing.T) {
	backend := NewSystemBackend()
	backend.SetReadSupplyLevels(true)
	if supplies, host := backend.SupplyLevels("Any Printer"); supplies != nil || host != "" {
		t.Fatalf("got %+v, %q", supplies, host)
	}
	if supplies, host := (SystemBackend{}).SupplyLevels("Any Printer"); supplies != nil || host != "" {
		t.Fatalf("zero backend gave %+v, %q", supplies, host)
	}
	(SystemBackend{}).SetReadSupplyLevels(true)
}
