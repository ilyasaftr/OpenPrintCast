package winspool

import (
	"net/netip"
	"strings"
	"unicode"
)

// PrinterEvidence is everything the classifier looks at. Windows has no
// "virtual printer" flag, so the verdict is built from layered evidence:
// the spooler flags, the port, the driver name and the queue name.
type PrinterEvidence struct {
	// Name is the queue name shown in Windows.
	Name string
	// DriverName is the spooler driver name.
	DriverName string
	// PortName is the first port of the queue (pPortName).
	PortName string
	// IsFax is set when Windows marks the queue as a fax printer.
	IsFax bool
	// IsNetwork is set for connections to a printer on another machine.
	IsNetwork bool
	// IsRedirected is set for Remote Desktop redirected printers.
	IsRedirected bool
}

// Classification is the classifier's verdict with a human-readable reason
// that the UI shows to explain it.
type Classification struct {
	// Virtual is true for software printers (PDF, XPS, OneNote, fax,
	// remote-desktop redirection) and false for real devices.
	Virtual bool
	// Reason names the evidence the verdict rests on.
	Reason string
}

// Reasons that carry no per-rule detail.
const (
	reasonFax        = "Windows marks it as a fax printer"
	reasonRedirected = "Remote Desktop redirected printer"
	reasonNetwork    = "network printer connection"
	reasonSamePort   = "Local Port named like the printer"
	reasonNoSigns    = "no virtual signs found"
)

// Reason prefixes for rules that name a matched value.
const (
	reasonNameMatches   = "name matches "
	reasonDriverMatches = "driver matches "
)

// matchKind selects how a textRule compares its pattern with a text.
type matchKind int

const (
	matchExact    matchKind = iota // whole text equals pattern
	matchPrefix                    // text starts with pattern
	matchContains                  // text contains pattern
	matchNumbered                  // pattern followed by digits and an optional ":"
	// The kinds below take no pattern.
	matchIPAddress // IPv4 or IPv6 address
	matchHostname  // dotted DNS host name
	matchFilePath  // path separator or a print-to-file extension
)

// hasPattern reports whether the kind compares against textRule.pattern.
func (k matchKind) hasPattern() bool { return k <= matchNumbered }

// textRule is one entry of a rule table. Patterns are lower case; texts
// are lowered before comparison.
type textRule struct {
	kind    matchKind
	pattern string
	// label is the human name of the rule: a brand for software rules, a
	// port description for port rules.
	label string
}

// matches reports whether the lowered text satisfies the rule.
func (r textRule) matches(lower string) bool {
	switch r.kind {
	case matchExact:
		return lower == r.pattern
	case matchPrefix:
		return strings.HasPrefix(lower, r.pattern)
	case matchContains:
		return strings.Contains(lower, r.pattern)
	case matchNumbered:
		return isNumbered(lower, r.pattern)
	case matchIPAddress:
		_, err := netip.ParseAddr(lower)
		return err == nil
	case matchHostname:
		return isHostname(lower)
	case matchFilePath:
		return isFilePath(lower)
	}
	return false
}

// Port descriptions used as rule labels.
const (
	labelPort        = "port"
	labelFilePort    = "file port"
	labelRemotePort  = "Remote Desktop port"
	labelAppPort     = "Microsoft Store app port"
	labelTCPIP       = "physical TCP/IP port"
	labelWSD         = "physical WSD port"
	labelUSB         = "physical USB port"
	labelIPP         = "physical IPP port"
	labelShare       = "physical network share port"
	labelDirectPort  = "physical direct-connect port"
	labelLegacyPorts = "legacy port"
)

// virtualPortRules lists ports that never lead to a device. Order is the
// order of evaluation; the first match names the reason.
// appPortRules recognize the ports of Microsoft Store and Print Support
// apps, such as "OneNote for Windows 10": a package name, the publisher ID
// and the user's SID, for example
// "Microsoft.Office.OneNote_16001.14326.22594.0_x64__8wekyb3d8bbwe_microsoft.onenoteim_S-1-5-21-...-1001".
// They look like dotted host names but can never reach a device, so they are
// checked before the physical-port rules.
var appPortRules = []textRule{
	{matchContains, "_s-1-5-", labelAppPort},       // per-user app port: the SID suffix
	{matchContains, "8wekyb3d8bbwe", labelAppPort}, // Microsoft's package publisher ID
}

var virtualPortRules = []textRule{
	{matchExact, "portprompt:", labelPort}, // Microsoft Print to PDF, XPS Writer
	{matchExact, "nul:", labelPort},
	{matchExact, "nul", labelPort},
	{matchExact, "file:", labelPort},
	{matchExact, "shrfax:", labelPort}, // Microsoft Shared Fax
	{matchNumbered, "ts", labelRemotePort},
	{matchFilePath, "", labelFilePort},
}

// physicalPortRules lists ports that reach a device or a print server. A
// match settles the verdict whatever the queue or driver is called.
var physicalPortRules = []textRule{
	{matchIPAddress, "", labelTCPIP},
	{matchPrefix, "ip_", labelTCPIP},
	{matchPrefix, "wsd-", labelWSD},
	{matchNumbered, "usb", labelUSB},
	{matchPrefix, "http://", labelIPP},
	{matchPrefix, "https://", labelIPP},
	{matchPrefix, "ipp://", labelIPP},
	{matchPrefix, "ipps://", labelIPP},
	{matchPrefix, `\\`, labelShare},
	{matchPrefix, "dot4_", labelDirectPort},
	{matchPrefix, "bth", labelDirectPort},
	{matchHostname, "", labelTCPIP},
}

// neutralPortRules lists ports that say nothing either way: real legacy
// printers and some software drivers use them. They only keep the "port
// named like the printer" check from firing on them. USB ports need no
// entry: physicalPortRules decide before that check runs.
var neutralPortRules = []textRule{
	{matchNumbered, "lpt", labelLegacyPorts},
	{matchNumbered, "com", labelLegacyPorts},
}

// softwareBrandRules lists distinctive software-printer brands, matched in
// the queue name and the driver name. Generic words (PDF, PCL, PS, Fax)
// also occur in real multifunction drivers, so only distinctive brands use
// "contains" and generic words use "prefix".
var softwareBrandRules = []textRule{
	{matchContains, "onenote", "OneNote"},
	{matchContains, "rustdesk", "RustDesk"},
	{matchContains, "anydesk", "AnyDesk"},
	{matchContains, "teamviewer", "TeamViewer"},
	{matchContains, "splashtop", "Splashtop"},
	{matchContains, "snagit", "Snagit"},
	{matchContains, "webex", "Webex"},
	{matchContains, "wondershare", "Wondershare"},
	{matchContains, "pdfelement", "PDFelement"},
	{matchContains, "print to pdf", "Print to PDF"},
	{matchContains, "pdf writer", "PDF writer"},
	{matchContains, "pdf printer", "PDF printer"},
	{matchContains, "pdfcreator", "PDFCreator"},
	{matchContains, "pdf24", "PDF24"},
	{matchContains, "dopdf", "doPDF"},
	{matchContains, "cutepdf", "CutePDF"},
	{matchContains, "bullzip", "Bullzip"},
	{matchContains, "primopdf", "PrimoPDF"},
	{matchContains, "nitro pdf", "Nitro PDF"},
	{matchContains, "pdf-xchange", "PDF-XChange"},
	{matchContains, "pdf architect", "PDF Architect"},
	{matchContains, "adobe pdf", "Adobe PDF"},
	{matchContains, "foxit", "Foxit"},
	{matchContains, "xps document writer", "XPS Document Writer"},
	{matchContains, "remote desktop easy print", "Remote Desktop Easy Print"},
	{matchContains, "shared fax", "Microsoft Shared Fax"},
	{matchContains, "universal document converter", "Universal Document Converter"},
	{matchContains, "amyuni", "Amyuni"},
	{matchPrefix, "fax", "Fax"},
}

// filePortExtensions are the file types a print-to-file port ends in.
var filePortExtensions = []string{".pdf", ".xps", ".oxps", ".prn", ".ps"}

// ClassifyPrinter decides whether a printer is virtual, from the spooler
// flags, then the port, then the names. Physical-port evidence outranks
// names, so a real printer renamed "Fax Room" stays physical.
func ClassifyPrinter(e PrinterEvidence) Classification {
	e = e.trimmed()
	for _, step := range classificationSteps {
		if verdict, decided := step(e); decided {
			return verdict
		}
	}
	return Classification{Reason: reasonNoSigns}
}

// classificationSteps are consulted in order; the first to decide wins.
var classificationSteps = []func(PrinterEvidence) (Classification, bool){
	classifyByFlags,
	classifyByAppPort,
	classifyByPhysicalPort,
	classifyByVirtualEvidence,
}

// trimmed returns the evidence with surrounding blanks removed.
func (e PrinterEvidence) trimmed() PrinterEvidence {
	e.Name = strings.TrimSpace(e.Name)
	e.DriverName = strings.TrimSpace(e.DriverName)
	e.PortName = strings.TrimSpace(e.PortName)
	return e
}

// classifyByFlags trusts the spooler's own fax and Remote Desktop marks.
func classifyByFlags(e PrinterEvidence) (Classification, bool) {
	switch {
	case e.IsFax:
		return Classification{Virtual: true, Reason: reasonFax}, true
	case e.IsRedirected:
		return Classification{Virtual: true, Reason: reasonRedirected}, true
	}
	return Classification{}, false
}

// classifyByAppPort recognizes Store and Print Support app ports before the
// host-name rule can mistake them for a network host.
func classifyByAppPort(e PrinterEvidence) (Classification, bool) {
	if rule, ok := firstMatch(appPortRules, e.PortName); ok {
		return Classification{Virtual: true, Reason: rule.label + " " + e.PortName}, true
	}
	return Classification{}, false
}

// classifyByPhysicalPort recognizes network connections and ports that
// reach a device.
func classifyByPhysicalPort(e PrinterEvidence) (Classification, bool) {
	if e.IsNetwork {
		return Classification{Reason: reasonNetwork}, true
	}
	if rule, ok := firstMatch(physicalPortRules, e.PortName); ok {
		return Classification{Reason: rule.label + " " + e.PortName}, true
	}
	return Classification{}, false
}

// classifyByVirtualEvidence looks for a virtual port, a Local Port named
// like the printer, or a software brand in the names.
func classifyByVirtualEvidence(e PrinterEvidence) (Classification, bool) {
	if rule, ok := firstMatch(virtualPortRules, e.PortName); ok {
		return Classification{Virtual: true, Reason: rule.label + " " + e.PortName}, true
	}
	if portNamedLikePrinter(e) {
		return Classification{Virtual: true, Reason: reasonSamePort}, true
	}
	if rule, ok := firstMatch(softwareBrandRules, e.Name); ok {
		return Classification{Virtual: true, Reason: reasonNameMatches + rule.label}, true
	}
	if rule, ok := firstMatch(softwareBrandRules, e.DriverName); ok {
		return Classification{Virtual: true, Reason: reasonDriverMatches + rule.label}, true
	}
	return Classification{}, false
}

// firstMatch returns the first rule of the table that matches text.
func firstMatch(rules []textRule, text string) (textRule, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return textRule{}, false
	}
	for _, rule := range rules {
		if rule.matches(lower) {
			return rule, true
		}
	}
	return textRule{}, false
}

// portNamedLikePrinter reports whether the queue uses a Local Port carrying
// its own name, which is how software printers such as RustDesk register.
func portNamedLikePrinter(e PrinterEvidence) bool {
	if e.PortName == "" || !strings.EqualFold(e.PortName, e.Name) {
		return false
	}
	_, neutral := firstMatch(neutralPortRules, e.PortName)
	return !neutral
}

// isNumbered reports whether text is prefix, one or more digits and an
// optional trailing colon ("lpt1:", "ts002", "usb001").
func isNumbered(text, prefix string) bool {
	digits, found := strings.CutPrefix(strings.TrimSuffix(text, ":"), prefix)
	return found && isDigits(digits)
}

// isDigits reports whether s is a non-empty run of ASCII digits.
func isDigits(s string) bool {
	return s != "" && strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' }) < 0
}

// isFilePath reports whether a port names a file: it has a path separator
// or ends in a print-to-file extension.
func isFilePath(lower string) bool {
	return strings.ContainsAny(lower, `\/`) || hasFileExtension(lower)
}

// hasFileExtension reports whether lower ends in a print-to-file extension.
func hasFileExtension(lower string) bool {
	for _, ext := range filePortExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// isHostname reports whether lower looks like a dotted DNS host name, as
// Standard TCP/IP ports are often named. Names ending in a file extension
// or a number are not host names.
func isHostname(lower string) bool {
	if !strings.Contains(lower, ".") || strings.HasPrefix(lower, ".") || strings.HasSuffix(lower, ".") {
		return false
	}
	if hasFileExtension(lower) || strings.IndexFunc(lower, notHostnameRune) >= 0 {
		return false
	}
	return !isDigits(lower[strings.LastIndex(lower, ".")+1:])
}

// notHostnameRune reports whether r cannot appear in a host name.
func notHostnameRune(r rune) bool {
	return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.')
}
