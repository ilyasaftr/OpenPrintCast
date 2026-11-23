package reach

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"
)

// tri is a yes/no answer that may be "can't tell".
type tri int

const (
	no tri = iota
	yes
	unsure
)

// protocol numbers and names that mean TCP or any protocol.
const (
	protoAnyNumber = "256"
	protoTCPNumber = "6"
)

// matchesTCP reports whether a rule's protocol covers TCP.
func matchesTCP(protocol string) bool {
	switch strings.ToUpper(strings.TrimSpace(protocol)) {
	case "TCP", "ANY", protoTCPNumber, protoAnyNumber:
		return true
	}
	return false
}

// matchesPort reports whether any entry of a LocalPort list contains port.
// An entry is "Any", a number, a range "600-700" or a comma list of those.
// Keywords such as "RPC" name other ports.
func matchesPort(entries []string, port int) bool {
	for _, entry := range entries {
		for _, part := range strings.Split(entry, ",") {
			if portPartContains(strings.TrimSpace(part), port) {
				return true
			}
		}
	}
	return false
}

func portPartContains(part string, port int) bool {
	if strings.EqualFold(part, "any") {
		return true
	}
	lo, hi, isRange := strings.Cut(part, "-")
	first, err := strconv.Atoi(strings.TrimSpace(lo))
	if err != nil {
		return false
	}
	if !isRange {
		return first == port
	}
	last, err := strconv.Atoi(strings.TrimSpace(hi))
	return err == nil && first <= port && port <= last
}

// profileCovers reports whether a rule's profile bitmask includes bit.
func profileCovers(mask, bit int) bool {
	return mask == 0 || mask == profileAny || mask&bit != 0
}

// matchesProgram reports whether a rule applies to the bridge's own
// executable: it names no program ("Any" or empty), or names this one.
func matchesProgram(program, exe string) tri {
	program = strings.TrimSpace(program)
	if program == "" || strings.EqualFold(program, "any") {
		return yes
	}
	if exe == "" {
		return unsure
	}
	if strings.EqualFold(normalizePath(program), normalizePath(exe)) {
		return yes
	}
	return no
}

func normalizePath(p string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(p), "/", `\`))
}

// coversSubnet reports whether a RemoteAddress list includes the whole
// local subnet, the peers on the LAN.
func coversSubnet(entries []string, local *net.IPNet) tri {
	result := no
	if len(entries) == 0 {
		return yes
	}
	for _, entry := range entries {
		switch entryCovers(strings.TrimSpace(entry), local) {
		case yes:
			return yes
		case unsure:
			result = unsure
		}
	}
	return result
}

func entryCovers(entry string, local *net.IPNet) tri {
	switch strings.ToLower(entry) {
	case "", "any", "*", "localsubnet", "localsubnet4", "localsubnet6":
		return yes
	case "defaultgateway", "dhcp", "dns", "wins", "intranet", "internet", "ply", "rmt":
		return unsure
	}
	if strings.Contains(entry, ":") {
		return no // IPv6 does not carry the IPv4 peers
	}
	if local == nil {
		return unsure
	}
	first, last, ok := entryRange(entry)
	if !ok {
		return unsure
	}
	return rangeCoversNet(first, last, local)
}

// entryRange is the first and last address an entry names: a single
// address, "a-b", "address/prefix" or "address/mask".
func entryRange(entry string) (first, last net.IP, ok bool) {
	if lo, hi, isRange := strings.Cut(entry, "-"); isRange {
		first, last = net.ParseIP(lo).To4(), net.ParseIP(hi).To4()
		return first, last, first != nil && last != nil
	}
	if addr, maskText, hasMask := strings.Cut(entry, "/"); hasMask {
		return maskedRange(addr, maskText)
	}
	ip := net.ParseIP(entry).To4()
	return ip, ip, ip != nil
}

func maskedRange(addr, maskText string) (first, last net.IP, ok bool) {
	ip := net.ParseIP(addr).To4()
	if ip == nil {
		return nil, nil, false
	}
	mask := parseMask(maskText)
	if mask == nil {
		return nil, nil, false
	}
	first, last = make(net.IP, 4), make(net.IP, 4)
	for i := range ip {
		first[i] = ip[i] & mask[i]
		last[i] = ip[i] | ^mask[i]
	}
	return first, last, true
}

// parseMask reads "24" or "255.255.255.0".
func parseMask(text string) net.IPMask {
	if bits, err := strconv.Atoi(text); err == nil {
		if bits < 0 || bits > 32 {
			return nil
		}
		return net.CIDRMask(bits, 32)
	}
	if ip := net.ParseIP(text).To4(); ip != nil {
		return net.IPMask(ip)
	}
	return nil
}

// rangeCoversNet reports whether first..last includes every address of n.
func rangeCoversNet(first, last net.IP, n *net.IPNet) tri {
	base := n.IP.To4()
	if base == nil || len(n.Mask) != net.IPv4len {
		return unsure
	}
	netFirst, netLast := make(net.IP, 4), make(net.IP, 4)
	for i := range base {
		netFirst[i] = base[i] & n.Mask[i]
		netLast[i] = base[i] | ^n.Mask[i]
	}
	if ones, bits := n.Mask.Size(); bits-ones > 1 {
		netFirst, netLast = stepIP(netFirst, +1), stepIP(netLast, -1) // hosts only
	}
	switch {
	case bytesLE(first, netFirst) && bytesLE(netLast, last):
		return yes
	case bytesLE(first, netLast) && bytesLE(netFirst, last):
		return unsure // only part of the subnet: some peers pass, others do not
	}
	return no
}

// stepIP is ip moved by delta (+1 or -1) as a 32-bit number.
func stepIP(ip net.IP, delta int) net.IP {
	n := binary.BigEndian.Uint32(ip) + uint32(delta)
	out := make(net.IP, 4)
	binary.BigEndian.PutUint32(out, n)
	return out
}

func bytesLE(a, b net.IP) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return true
}
