package winspool

import (
	"errors"
	"net/netip"
	"strings"
)

// errNoSupplySource means the printer has no network port whose SNMP
// settings allow reading it (a USB, WSD, IPP or file port, a port with SNMP
// switched off, or a queue the port monitor does not know). It is the
// normal answer for most local printers, not a failure.
var errNoSupplySource = errors.New("printer has no SNMP-enabled TCP/IP port")

// Defaults of the Standard TCP/IP port monitor's SNMP settings.
const (
	defaultSNMPCommunity   = "public"
	defaultSNMPDeviceIndex = 1
)

// snmpTarget is the printer's own address and the SNMP settings its
// Windows TCP/IP port is configured with. It is built from that port alone,
// never from anything a print client sent.
type snmpTarget struct {
	Host        string
	Community   string
	DeviceIndex int // hrDeviceIndex of the printer, the port's "SNMP Index"
}

// tcpipPortSettings holds the values of a Standard TCP/IP port's registry
// key that matter for SNMP. The text fields are "" when the value is
// absent or empty; SNMPEnabled is true only when "SNMP Enabled" is present
// and not 0, and SNMPIndex is 0 when absent.
type tcpipPortSettings struct {
	PortName      string
	HostName      string
	IPAddress     string
	SNMPEnabled   bool
	SNMPCommunity string
	SNMPIndex     int
}

// targetFromPortSettings decides whether and where to read a printer's
// supplies from its port settings. SNMP must be switched on in the port: an
// administrator who turned it off wants no SNMP traffic from this PC. The
// community name is the port's own; the default "public" applies only when
// SNMP is on and the port names none.
func targetFromPortSettings(s tcpipPortSettings) (snmpTarget, bool) {
	host := portHost(s)
	if !s.SNMPEnabled || !usableSNMPHost(host) {
		return snmpTarget{}, false
	}
	target := snmpTarget{Host: host, Community: s.SNMPCommunity, DeviceIndex: s.SNMPIndex}
	if target.Community == "" {
		target.Community = defaultSNMPCommunity
	}
	if target.DeviceIndex < 1 {
		target.DeviceIndex = defaultSNMPDeviceIndex
	}
	return target, true
}

// portHost is the printer's host name or address: the port's HostName, else
// its IPAddress, else the port's name when that is itself an address.
func portHost(s tcpipPortSettings) string {
	for _, candidate := range []string{s.HostName, s.IPAddress} {
		if host := strings.TrimSpace(candidate); host != "" {
			return host
		}
	}
	if _, err := netip.ParseAddr(s.PortName); err == nil {
		return s.PortName
	}
	return ""
}

// usableSNMPHost accepts a host name or an address a printer can have: an
// IP address that is not unspecified, loopback or multicast, or a DNS name
// of letters, digits, dots, hyphens and underscores.
func usableSNMPHost(host string) bool {
	if host == "" {
		return false
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		return !addr.IsUnspecified() && !addr.IsLoopback() && !addr.IsMulticast()
	}
	return strings.IndexFunc(host, func(r rune) bool { return !hostNameRune(r) }) < 0
}

func hostNameRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_'
}
