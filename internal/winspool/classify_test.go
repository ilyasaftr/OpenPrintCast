package winspool

import (
	"strings"
	"testing"
)

func TestClassifyPrinterRealWorld(t *testing.T) {
	const wsd = "WSD-8f12b3c4-0a1b-4c2d-9e3f-5a6b7c8d9e0f"
	for _, tc := range []struct {
		label   string
		e       PrinterEvidence
		virtual bool
	}{
		// Virtual.
		{"print to pdf", PrinterEvidence{Name: "Microsoft Print to PDF", DriverName: "Microsoft Print To PDF", PortName: "PORTPROMPT:"}, true},
		{"xps writer", PrinterEvidence{Name: "Microsoft XPS Document Writer", DriverName: "Microsoft XPS Document Writer v4", PortName: "PORTPROMPT:"}, true},
		{"onenote 2010", PrinterEvidence{Name: "Send To OneNote 2010", DriverName: "Send To Microsoft OneNote 2010 Driver", PortName: "nul:"}, true},
		{"onenote 16 driver", PrinterEvidence{Name: "Send to OneNote 16", DriverName: "Send to Microsoft OneNote 16 Driver", PortName: "nul:"}, true},
		{"onenote 16 driver only", PrinterEvidence{Name: "Notes", DriverName: "Send to Microsoft OneNote 16 Driver"}, true},
		{"onenote for windows 10", PrinterEvidence{Name: "OneNote for Windows 10", DriverName: "Unknown Driver 1.0", PortName: "Whatever"}, true},
		{"onenote desktop", PrinterEvidence{Name: "OneNote (Desktop)"}, true},
		{"onenote for windows 10 app port", PrinterEvidence{Name: "OneNote for Windows 10", PortName: "Microsoft.Office.OneNote_16001.14326.22594.0_x64__8wekyb3d8bbwe_microsoft.onenoteim_S-1-5-21-3060330206-3333920839-2431524572-1001"}, true},
		{"app port wins even with a neutral name", PrinterEvidence{Name: "Notes", DriverName: "Microsoft Software Printer Driver", PortName: "Microsoft.Office.OneNote_16001.1.0_x64__8wekyb3d8bbwe_microsoft.onenotepsa_S-1-5-21-1-2-3-1001"}, true},
		{"onenote protected", PrinterEvidence{Name: "OneNote (Desktop) - Protected", DriverName: "Microsoft IPP Class Driver", PortName: "OneNote Protected"}, true},
		{"shared fax", PrinterEvidence{Name: "Fax", DriverName: "Microsoft Shared Fax Driver", PortName: "SHRFAX:"}, true},
		{"fax flag", PrinterEvidence{Name: "Office", DriverName: "Some Driver", IsFax: true}, true},
		{"adobe pdf", PrinterEvidence{Name: "Adobe PDF", DriverName: "Adobe PDF Converter", PortName: `Documents\*.pdf`}, true},
		{"amyuni on lpt", PrinterEvidence{Name: "ABS PDF Driver v400", DriverName: "Amyuni Document Converter", PortName: "LPT1:"}, true},
		{"remote desktop", PrinterEvidence{Name: "HP LaserJet (redirected 2)", DriverName: "Remote Desktop Easy Print", PortName: "TS002", IsRedirected: true}, true},
		{"remote desktop port only", PrinterEvidence{Name: "Printer (from PC)", DriverName: "HP Universal", PortName: "TS001"}, true},
		{"rustdesk", PrinterEvidence{Name: "RustDesk Printer", DriverName: "RustDesk v4 Printer Driver", PortName: "RustDesk Printer"}, true},
		{"local port named like printer", PrinterEvidence{Name: "Quiet Printer", DriverName: "Generic", PortName: "quiet printer"}, true},
		{"file port", PrinterEvidence{Name: "To File", PortName: "FILE:"}, true},
		{"nul port", PrinterEvidence{Name: "Sink", PortName: "NUL"}, true},
		{"file path port", PrinterEvidence{Name: "Out", PortName: `C:\Users\me\out.prn`}, true},
		{"pdf extension port", PrinterEvidence{Name: "Out", PortName: "out.pdf"}, true},
		// Physical.
		{"ricoh", PrinterEvidence{Name: "Ricoh Aficio MP C2500", DriverName: "RICOH Aficio MP C2500 PCL5c", PortName: "192.168.0.90"}, false},
		{"hp deskjet", PrinterEvidence{Name: "HP Deskjet 6940 series", DriverName: "HP Deskjet 6940 series", PortName: "192.168.0.9"}, false},
		{"brother wsd", PrinterEvidence{Name: "Brother HL-L2350DW series", DriverName: "Brother HL-L2350DW series", PortName: wsd}, false},
		{"usb", PrinterEvidence{Name: "Canon MG3600", DriverName: "Canon MG3600 series Printer", PortName: "USB001"}, false},
		{"ipv6", PrinterEvidence{Name: "Lab", DriverName: "Generic", PortName: "fe80::1ff:fe23:4567:890a"}, false},
		{"ip_ name", PrinterEvidence{Name: "Lab", DriverName: "Generic", PortName: "IP_192.168.1.5"}, false},
		{"ipp url", PrinterEvidence{Name: "Hall", DriverName: "Microsoft IPP Class Driver", PortName: "http://192.168.1.5:631/ipp/print"}, false},
		{"ipps url", PrinterEvidence{Name: "Hall", PortName: "ipps://printer.local/ipp"}, false},
		{"hostname port", PrinterEvidence{Name: "Hall", PortName: "printer01.office.lan"}, false},
		{"dot4", PrinterEvidence{Name: "Old Epson", PortName: "DOT4_001"}, false},
		{"bluetooth", PrinterEvidence{Name: "Mobile", PortName: "BTH001"}, false},
		{"network connection", PrinterEvidence{Name: `\\server\HP LaserJet`, DriverName: "HP Universal Printing PCL 6", IsNetwork: true}, false},
		{"unc port", PrinterEvidence{Name: "Shared", PortName: `\\server\queue`}, false},
		{"renamed fax room", PrinterEvidence{Name: "Fax Room", DriverName: "Ricoh PCL6", PortName: "10.0.0.7"}, false},
		{"renamed onenote lookalike on ip", PrinterEvidence{Name: "OneNote Printer", PortName: "10.0.0.8"}, false},
		{"epson on lpt", PrinterEvidence{Name: "Front Desk", DriverName: "Epson LQ-590", PortName: "LPT1:"}, false},
		{"mfp driver words", PrinterEvidence{Name: "Copy Room", DriverName: "Brother MFC-L8900CDW PCL6 PS PDF Fax", PortName: wsd}, false},
		{"mfp driver words without port", PrinterEvidence{Name: "Copy Room", DriverName: "Brother MFC PCL6 PS PDF Fax"}, false},
		// Documented outcome: "PDF Station" is no brand, so it follows its port.
		{"canon pdf station on ip", PrinterEvidence{Name: "Canon PDF Station", DriverName: "Canon Generic Plus PCL6", PortName: "192.168.0.44"}, false},
		{"canon pdf station without port", PrinterEvidence{Name: "Canon PDF Station", DriverName: "Canon Generic Plus PCL6"}, false},
		{"com port", PrinterEvidence{Name: "Receipt", DriverName: "Generic / Text Only", PortName: "COM1:"}, false},
		{"lpt port named like printer", PrinterEvidence{Name: "LPT1:", PortName: "LPT1:"}, false},
		{"empty", PrinterEvidence{}, false},
		{"blanks", PrinterEvidence{Name: "  ", DriverName: "\t", PortName: " "}, false},
		{"garbage", PrinterEvidence{Name: "\x00\xff??", DriverName: "...", PortName: "::::"}, false},
	} {
		t.Run(tc.label, func(t *testing.T) {
			got := ClassifyPrinter(tc.e)
			if got.Virtual != tc.virtual {
				t.Fatalf("ClassifyPrinter(%+v) = %+v, want virtual=%t", tc.e, got, tc.virtual)
			}
			if got.Reason == "" {
				t.Fatalf("empty reason for %+v", tc.e)
			}
		})
	}
}

func TestClassifyPrinterReasons(t *testing.T) {
	for _, tc := range []struct {
		e    PrinterEvidence
		want string
	}{
		{PrinterEvidence{Name: "Microsoft Print to PDF", PortName: "PORTPROMPT:"}, "port PORTPROMPT:"},
		{PrinterEvidence{Name: "OneNote for Windows 10", DriverName: "x"}, "name matches OneNote"},
		{PrinterEvidence{Name: "Notes", DriverName: "Send to Microsoft OneNote 16 Driver"}, "driver matches OneNote"},
		{PrinterEvidence{Name: "Fax", IsFax: true}, "Windows marks it as a fax printer"},
		{PrinterEvidence{Name: "Remote", IsRedirected: true}, "Remote Desktop redirected printer"},
		{PrinterEvidence{Name: "Remote", PortName: "TS003"}, "Remote Desktop port TS003"},
		{PrinterEvidence{Name: "Adobe", PortName: `Documents\*.pdf`}, `file port Documents\*.pdf`},
		{PrinterEvidence{Name: "RustDesk Printer", PortName: "RustDesk Printer"}, "Local Port named like the printer"},
		{PrinterEvidence{Name: "Ricoh", PortName: "192.168.0.90"}, "physical TCP/IP port 192.168.0.90"},
		{PrinterEvidence{Name: "Brother", PortName: "WSD-1234"}, "physical WSD port WSD-1234"},
		{PrinterEvidence{Name: "Canon", PortName: "USB001"}, "physical USB port USB001"},
		{PrinterEvidence{Name: `\\srv\HP`, IsNetwork: true}, "network printer connection"},
		{PrinterEvidence{Name: "HP LaserJet", PortName: "LPT1:"}, "no virtual signs found"},
	} {
		if got := ClassifyPrinter(tc.e).Reason; got != tc.want {
			t.Errorf("ClassifyPrinter(%+v).Reason = %q, want %q", tc.e, got, tc.want)
		}
	}
}

func TestClassifyPrinterTrimsBlanks(t *testing.T) {
	got := ClassifyPrinter(PrinterEvidence{Name: "  Fax ", PortName: " SHRFAX: "})
	if !got.Virtual || got.Reason != "port SHRFAX:" {
		t.Fatalf("got %+v", got)
	}
}

func TestRuleTablesHygiene(t *testing.T) {
	tables := map[string][]textRule{
		"appPortRules":       appPortRules,
		"virtualPortRules":   virtualPortRules,
		"physicalPortRules":  physicalPortRules,
		"neutralPortRules":   neutralPortRules,
		"softwareBrandRules": softwareBrandRules,
	}
	for name, rules := range tables {
		seen := map[textRule]bool{}
		for _, r := range rules {
			if r.label == "" {
				t.Errorf("%s: rule %+v has no label", name, r)
			}
			if r.kind.hasPattern() != (r.pattern != "") {
				t.Errorf("%s: rule %+v pattern does not fit its kind", name, r)
			}
			if r.pattern != strings.ToLower(r.pattern) || r.pattern != strings.TrimSpace(r.pattern) {
				t.Errorf("%s: pattern %q must be lower case and trimmed", name, r.pattern)
			}
			key := textRule{kind: r.kind, pattern: r.pattern}
			if seen[key] {
				t.Errorf("%s: duplicate rule %+v", name, r)
			}
			seen[key] = true
		}
	}
	for _, ext := range filePortExtensions {
		if !strings.HasPrefix(ext, ".") || ext != strings.ToLower(ext) {
			t.Errorf("bad file extension %q", ext)
		}
	}
}

// Every entry the retired VirtualPrinterPrefixes table held must still
// mark a printer of that name virtual.
func TestLegacyPrefixesStillVirtual(t *testing.T) {
	for _, name := range []string{
		"microsoft print to pdf", "microsoft xps document writer", "onenote", "send to onenote",
		"fax", "wondershare", "pdfelement", "adobe pdf", "foxit", "cutepdf", "pdf24", "dopdf",
		"bullzip", "primopdf", "nitro pdf", "snagit", "webex", "anydesk",
	} {
		if !IsVirtualPrinter(name) || !IsVirtualPrinter(strings.ToUpper(name)+" 2") {
			t.Errorf("%q must stay virtual", name)
		}
	}
}

func TestBrandKeywordsMatchNameAndDriver(t *testing.T) {
	for _, rule := range softwareBrandRules {
		if rule.kind != matchContains {
			continue
		}
		if !ClassifyPrinter(PrinterEvidence{Name: "My " + rule.pattern + " 1"}).Virtual ||
			!ClassifyPrinter(PrinterEvidence{Name: "Office", DriverName: "The " + strings.ToUpper(rule.pattern)}).Virtual {
			t.Errorf("brand %q not recognized in name or driver", rule.pattern)
		}
	}
}

func TestFaxIsOnlyAPrefix(t *testing.T) {
	if IsVirtualPrinter("Brother MFC Fax Room") {
		t.Fatal("fax in the middle of a name must not decide")
	}
	if !IsVirtualPrinter("Fax Room") {
		t.Fatal("name starting with fax is virtual without port evidence")
	}
}

func TestIsHostnameAndNumbered(t *testing.T) {
	for _, host := range []string{"printer.local", "hp-1.office.lan", "p_1.example.com"} {
		if !isHostname(host) {
			t.Errorf("isHostname(%q) = false", host)
		}
	}
	for _, text := range []string{"printer", ".local", "local.", "a b.local", "out.pdf", "x.ps", "1.2.3", `a\b.c`} {
		if isHostname(text) {
			t.Errorf("isHostname(%q) = true", text)
		}
	}
	if !isNumbered("lpt1:", "lpt") || isNumbered("lpt:", "lpt") || isNumbered("lptx", "lpt") || isNumbered("com1", "lpt") {
		t.Fatal("isNumbered misjudges legacy ports")
	}
}

func TestAppPortsAreNotMistakenForNetworkHosts(t *testing.T) {
	port := "Microsoft.Office.OneNote_16001.14326.22594.0_x64__8wekyb3d8bbwe_microsoft.onenoteim_S-1-5-21-3060330206-3333920839-2431524572-1001"
	got := ClassifyPrinter(PrinterEvidence{Name: "OneNote for Windows 10 (Agung-PC)", PortName: port})
	if !got.Virtual || !strings.HasPrefix(got.Reason, labelAppPort) {
		t.Fatalf("ClassifyPrinter = %+v, want virtual with a %q reason", got, labelAppPort)
	}
	for _, host := range []string{"printer.example.com", "HP-ABC123.corp.local", "ricoh_mp.lan"} {
		if got := ClassifyPrinter(PrinterEvidence{Name: "Office", PortName: host}); got.Virtual {
			t.Errorf("real network host %q classified as virtual: %+v", host, got)
		}
	}
}
