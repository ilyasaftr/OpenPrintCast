package net

import (
	"fmt"
	"os"
	"sort"
	"strings"

	stdnet "net"
)

const (
	defaultHostLabel = "openprintcast"
	maxHostLabelLen  = 60
	// routeProbeAddr is only dialed over UDP to learn the outbound route.
	routeProbeAddr = "8.8.8.8:80"
	// nulSeparator joins interface entries in a network snapshot and host seeds.
	nulSeparator = "\x00"
)

// HostLabel returns the single-label mDNS host name: the real machine
// hostname, sanitized to one DNS-safe label like the Python bridge
// (spaces, slashes and dots become dashes, capped at 60 chars).
func HostLabel() string {
	label := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '/', '\\', '.':
			return '-'
		default:
			return r
		}
	}, rawHostname())
	if runes := []rune(label); len(runes) > maxHostLabelLen {
		label = string(runes[:maxHostLabelLen])
	}
	if label = strings.Trim(label, "-"); label == "" {
		return defaultHostLabel
	}
	return label
}

// rawHostname is the machine name, falling back through the environment
// to a fixed default when the OS reports none.
func rawHostname() string {
	raw, err := os.Hostname()
	if err == nil && strings.TrimSpace(raw) != "" {
		return raw
	}
	if c := os.Getenv("COMPUTERNAME"); c != "" {
		return c
	}
	if h := os.Getenv("HOSTNAME"); h != "" {
		return h
	}
	return defaultHostLabel
}

// OutboundIP returns the LAN address reaching the internet, selected the
// way the Rust bridge does: connecting a UDP socket only picks the route,
// no traffic is sent. Binding mDNS to this address avoids Windows
// multicasting out the wrong interface. Empty when offline.
func OutboundIP() string {
	conn, err := stdnet.Dial("udp", routeProbeAddr)
	if err != nil {
		return ""
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*stdnet.UDPAddr)
	if !ok {
		return ""
	}
	return addr.IP.String()
}

// Include interface addresses as well as the default route, since a link can
// change without changing the interface selected for outbound traffic.
func networkSnapshot() (string, string) {
	ip := OutboundIP()
	ifaces, err := stdnet.Interfaces()
	if err != nil {
		return ip, ""
	}
	var entries []string
	for _, iface := range ifaces {
		if iface.Flags&stdnet.FlagUp == 0 || iface.Flags&stdnet.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			entries = append(entries, fmt.Sprintf("%d:%s:%s", iface.Index, iface.Name, addr.String()))
		}
	}
	sort.Strings(entries)
	return ip, strings.Join(entries, nulSeparator)
}

// ifaces restricts multicast to the interface owning the outbound IP,
// like the Python bridge binding Zeroconf to the Wi-Fi address (Windows
// otherwise multicasts out Ethernet/VPN first). Nil means all interfaces,
// the safe fallback when the route cannot be determined.
func (a *Advertiser) ifaces() []stdnet.Interface {
	if a.ip == "" {
		return nil
	}
	ifaces, err := stdnet.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil || iface.Flags&stdnet.FlagUp == 0 {
			continue
		}
		if hasAddress(addrs, a.ip) {
			return []stdnet.Interface{iface}
		}
	}
	return nil
}

// hasAddress reports whether any of addrs is the IP ip.
func hasAddress(addrs []stdnet.Addr, ip string) bool {
	for _, addr := range addrs {
		var found stdnet.IP
		switch v := addr.(type) {
		case *stdnet.IPNet:
			found = v.IP
		case *stdnet.IPAddr:
			found = v.IP
		}
		if found != nil && found.String() == ip {
			return true
		}
	}
	return false
}
