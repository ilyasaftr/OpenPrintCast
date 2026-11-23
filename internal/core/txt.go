package core

import (
	"strconv"
	"strings"
)

// Document formats advertised in the TXT `pdl` key.
var AdvertisedPDL = strings.Join(AdvertisedMIMEs(), ",")

// URF capabilities match the supported 8-bit grayscale and 24-bit RGB formats.
// DM1 is the URF duplex mode "back side normal": clients send every back side
// upright and the Windows driver imposes the sheet (CUPS reads DM1 as
// cupsBackSide=Normal). It is advertised unconditionally; whether two-sided
// output is offered is signalled by the TXT `Duplex` key and `sides-supported`.
const URFCaps = "W8,SRGB24,V1.4,RS300,DM1"

// IPPServiceType is the DNS-SD type browsed by IPP clients.
const IPPServiceType = "_ipp._tcp.local."

// AirPrintSubtype is browsed specifically by iOS AirPrint.
const AirPrintSubtype = "_universal._sub._ipp._tcp.local."

// ResourcePath is the IPP resource path served for a printer id.
// The HTTP router must serve exactly this path.
func ResourcePath(printerID string) string { return "ipp/print/" + printerID }

// BuildAirPrintTXT builds the mDNS TXT record for info, mirroring the
// Python bridge. host is the single-label mDNS host (no .local suffix);
// ip is the LAN address iOS must reach; port is the IPP port.
func BuildAirPrintTXT(info PrinterInfo, uuid, host, ip string, port int) map[string]string {
	note := info.Location
	if note == "" {
		note = "OpenPrintCast on " + host
	}
	display := info.DisplayName()
	color, urf := "T", URFCaps
	if !info.SupportsColor() {
		color, urf = "F", "W8,V1.4,RS300,DM1"
	}
	return map[string]string{
		"txtvers":  "1",
		"qtotal":   "1",
		"rp":       ResourcePath(info.ID),
		"ty":       display,
		"product":  "(" + info.ModelName() + ")",
		"note":     note,
		"pdl":      AdvertisedPDL,
		"URF":      urf,
		"Color":    color,
		"Duplex":   duplexFlag(info),
		"adminurl": "http://" + ip + ":" + strconv.Itoa(port) + "/",
		"priority": "50",
		"UUID":     uuid,
	}
}

// duplexFlag is the TXT `Duplex` value (Bonjour Printing Specification
// table 3): "T" only when the printer can generate two-sided output.
func duplexFlag(info PrinterInfo) string {
	if info.SupportsDuplex() {
		return "T"
	}
	return "F"
}
