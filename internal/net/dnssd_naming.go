package net

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// maxDNSLabelLen is the longest single DNS label in bytes.
	maxDNSLabelLen = 63
	// shortUUIDLen is the UUID prefix that disambiguates renamed instances.
	shortUUIDLen = 8
	// dnsEscapedChars are the ASCII characters escaped with a backslash in
	// a presented DNS label.
	dnsEscapedChars = ". '@;()\"\\"
)

// PrinterUUID is the stable per-printer identity iOS sees across
// restarts, scoped by host like the Python bridge (uuid5 of
// printer@host) so two PCs sharing same-named printers never collide.
func PrinterUUID(printerID, host string) string {
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(printerID+"@"+host)).String()
}

// flattenTXT renders a TXT map as sorted k=v strings.
func flattenTXT(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// truncateToBytes trims whole runes from the end of s until it fits in
// limit bytes.
func truncateToBytes(s string, limit int) string {
	for len(s) > limit && len(s) > 0 {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	return s
}

func conflictHost(base, ip string, attempt int) string {
	sum := sha256.Sum256([]byte(ip))
	suffix := fmt.Sprintf("-ap%x", sum[:4])
	if attempt > 1 {
		suffix += fmt.Sprintf("-%d", attempt)
	}
	return truncateToBytes(base, maxDNSLabelLen-len(suffix)) + suffix
}

func instanceLabel(display, id, host string, attempt int) string {
	display = strings.ToValidUTF8(display, "?")
	display = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return '_'
		}
		return r
	}, display)
	suffix := ""
	if attempt > 0 {
		suffix = " (" + PrinterUUID(id, host)[:shortUUIDLen]
		if attempt > 1 {
			suffix += fmt.Sprintf("-%d", attempt)
		}
		suffix += ")"
	}
	return truncateToBytes(display, maxDNSLabelLen-len(suffix)) + suffix
}

func instanceName(display, id, host string, attempt int) string {
	// Match miekg/dns's unpacked presentation so incoming queries compare
	// equal while the wire name remains one UTF-8 DNS label.
	var label strings.Builder
	for _, b := range []byte(instanceLabel(display, id, host, attempt)) {
		switch {
		case strings.ContainsRune(dnsEscapedChars, rune(b)):
			label.WriteByte('\\')
			label.WriteByte(b)
		case b < ' ' || b > '~':
			fmt.Fprintf(&label, "\\%03d", b)
		default:
			label.WriteByte(b)
		}
	}
	return label.String()
}
