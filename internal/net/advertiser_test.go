package net

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/OpenPrinting/goipp"
	"github.com/miekg/dns"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestBrowseServiceUniversal(t *testing.T) {
	if got := browseService(); got != "_ipp._tcp,_universal" {
		t.Fatalf("browse service = %q", got)
	}
}

func TestTXTKeys(t *testing.T) {
	txt := flattenTXT(core.BuildAirPrintTXT(testPrinter(), PrinterUUID("hp", "mypc"), "mypc", "192.168.1.9", 631))
	got := map[string]string{}
	for _, kv := range txt {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = v
	}
	for k, want := range map[string]string{
		"txtvers": "1",
		"qtotal":  "1",
		"rp":      "ipp/print/hp",
		"ty":      "HP Air",
	} {
		if got[k] != want {
			t.Errorf("TXT %s = %q, want %q", k, got[k], want)
		}
	}
	if _, ok := got["TLS"]; ok {
		t.Error("TLS must be omitted when HTTP Upgrade to TLS is unsupported")
	}
	if !strings.Contains(got["adminurl"], "192.168.1.9") {
		t.Errorf("adminurl = %q, want LAN IP", got["adminurl"])
	}
	if !strings.Contains(got["pdl"], "application/pdf") {
		t.Errorf("pdl = %q, want application/pdf", got["pdl"])
	}
	if got["URF"] != core.URFCaps {
		t.Error("incorrect URF capabilities")
	}
	if want := PrinterUUID("hp", "mypc"); got["UUID"] != want {
		t.Errorf("UUID = %q, want stable uuid5 %q", got["UUID"], want)
	}
}

func TestPrinterUUIDStable(t *testing.T) {
	first, again := PrinterUUID("p1", "pc-a"), PrinterUUID("p1", "pc-a")
	if first != again {
		t.Error("UUID not stable for same printer@host")
	}
	if PrinterUUID("p1", "pc-a") == PrinterUUID("p1", "pc-b") {
		t.Error("UUID must differ per host")
	}
	if PrinterUUID("p1", "pc-a") == PrinterUUID("p2", "pc-a") {
		t.Error("UUID must differ per printer")
	}
}

func TestAdvertSignatureTracksAdvertisedFieldsOnly(t *testing.T) {
	a := testPrinter()
	base := advertSignature([]core.PrinterInfo{a})
	if advertSignature([]core.PrinterInfo{a}) != base {
		t.Fatal("signature must be stable")
	}
	for name, mutate := range map[string]func(*core.PrinterInfo){
		"name":     func(p *core.PrinterInfo) { p.AirPrintName = "Renamed" },
		"location": func(p *core.PrinterInfo) { p.Location = "Floor 2" },
		"duplex":   func(p *core.PrinterInfo) { duplex := true; p.DuplexCapable = &duplex },
	} {
		b := a
		mutate(&b)
		if advertSignature([]core.PrinterInfo{b}) == base {
			t.Fatalf("%s change must alter the signature", name)
		}
	}
	if advertSignature(nil) == base {
		t.Fatal("removing a printer must alter the signature")
	}
}

func TestAdvertiserResyncTracksNetworkChanges(t *testing.T) {
	ip, network := "192.0.2.1", "interface-a"
	adv := &Advertiser{
		ip: ip, network: network, sig: advertSignature(nil),
		registered: make(map[string]bool),
		snapshot:   func() (string, string) { return ip, network },
	}
	if changed, err := adv.Resync(nil); err != nil || changed {
		t.Fatalf("unchanged network: changed=%v err=%v", changed, err)
	}
	ip, network = "192.0.2.2", "interface-b"
	if changed, err := adv.Resync(nil); err != nil || !changed {
		t.Fatalf("changed network: changed=%v err=%v", changed, err)
	}
	if adv.ip != ip || adv.network != network {
		t.Fatalf("network not updated: ip=%q network=%q", adv.ip, adv.network)
	}
	if changed, err := adv.Resync(nil); err != nil || changed {
		t.Fatalf("stable new network: changed=%v err=%v", changed, err)
	}
}

func TestAdvertiserRenamesConflictingInstances(t *testing.T) {
	adv := &Advertiser{
		host: "pc", sig: "current", generation: 4,
		registered: map[string]bool{"hp": true},
		names:      make(map[string]int),
	}
	if adv.markConflict(instanceConflict{id: "hp", generation: 3}) {
		t.Fatal("a withdrawn registration must not trigger another rename")
	}
	if !adv.markConflict(instanceConflict{id: "hp", generation: 4}) || adv.names["hp"] != 1 || adv.sig != "" {
		t.Fatal("current conflict must invalidate and rename the registration")
	}
	long := strings.Repeat("printer", 12)
	renamed := instanceName(long, "hp", "pc", 1)
	wire := make([]byte, 1024)
	if _, err := dns.PackDomainName(renamed+"._ipp._tcp.local.", wire, 0, nil, false); err != nil ||
		wire[0] > 63 || !strings.Contains(renamed, PrinterUUID("hp", "pc")[:8]) {
		t.Fatalf("invalid conflict name: %q", renamed)
	}
	if instanceName(long, "hp", "pc", 2) == renamed {
		t.Fatal("successive conflicts must use different names")
	}
}

func TestInstanceNamesAreValidDNSLabels(t *testing.T) {
	for _, display := range []string{
		"HP Air (PC)", `Printer 2.1.`, `Printer\Office`,
		"Printer \xe2\x98\x83", strings.Repeat("printer", 12),
		strings.Repeat("\xe2\x98\x83", 30),
	} {
		for _, attempt := range []int{0, 1} {
			label := instanceName(display, "hp", "pc", attempt)
			name := label + "._ipp._tcp.local."
			wire := make([]byte, 1024)
			end, err := dns.PackDomainName(name, wire, 0, nil, false)
			if err != nil {
				t.Fatalf("%q attempt %d: %v", display, attempt, err)
			}
			length := int(wire[0])
			if length > 63 || !utf8.Valid(wire[1:1+length]) {
				t.Fatalf("%q attempt %d: invalid wire label %q", display, attempt, wire[1:1+length])
			}
			decoded, _, err := dns.UnpackDomainName(wire[:end], 0)
			if err != nil || decoded != name || len(dns.SplitDomainName(decoded)) != 4 {
				t.Fatalf("%q attempt %d: decoded=%q want=%q err=%v", display, attempt, decoded, name, err)
			}
		}
	}
}

func TestHostConflictChangesURIWithoutChangingPrinterIdentity(t *testing.T) {
	adv := &Advertiser{
		host: "pc", identityHost: "pc", ip: "192.0.2.1", sig: "current", generation: 4,
		registered: map[string]bool{"hp": true}, names: make(map[string]int),
	}
	shared := testShared()
	shared.setIdentityHost("pc")
	mux := newMux(shared, adv.Host, 631)
	getAttrs := func() goipp.Attributes {
		req := goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1)
		req.Operation.Add(goipp.MakeAttr("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
		req.Operation.Add(goipp.MakeAttr("attributes-natural-language", goipp.TagLanguage, goipp.String("en")))
		req.Operation.Add(goipp.MakeAttr("printer-uri", goipp.TagURI, goipp.String("ipp://pc.local:631/ipp/print/hp")))
		body, err := req.EncodeBytes()
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(http.MethodPost, "/ipp/print/hp", bytes.NewReader(body))
		r.Header.Set("Content-Type", goipp.ContentType)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		var rsp goipp.Message
		if err := rsp.DecodeBytes(w.Body.Bytes()); err != nil {
			t.Fatal(err)
		}
		return rsp.Printer
	}
	before := getAttrs()
	if !adv.markConflict(instanceConflict{id: "hp", generation: 4, host: true}) {
		t.Fatal("host conflict not handled")
	}
	after := getAttrs()
	if adv.Host() == "pc" {
		t.Fatalf("host=%q was not renamed", adv.Host())
	}
	if got := attrValues(t, after, "printer-uri-supported"); len(got) != 1 || got[0] != printerURI(testPrinter(), adv.Host(), 631) {
		t.Fatalf("printer URI did not follow renamed host: %v", got)
	}
	if !slices.Equal(attrValues(t, before, "printer-uuid"), attrValues(t, after, "printer-uuid")) {
		t.Fatal("host rename changed printer identity")
	}
}
