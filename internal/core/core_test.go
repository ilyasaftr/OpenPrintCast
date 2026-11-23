package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxRenderDPI != 300 || cfg.ReversePageOrder || cfg.AutoRotateToFit || !cfg.NotifyOnFailure {
		t.Fatalf("bad defaults: %+v", cfg)
	}
}

func TestClampRenderDPI(t *testing.T) {
	for in, want := range map[int]int{0: 72, 72: 72, 300: 300, 1200: 1200, 9999: 1200} {
		if got := ClampRenderDPI(in); got != want {
			t.Fatalf("ClampRenderDPI(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	want := AppConfig{MaxRenderDPI: 600, ReversePageOrder: true,
		ShowVirtualPrinters: true, FolderEnabled: true, FolderPath: dir}
	if err := want.SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, want)
	}
}

func TestConfigMissingFileGivesDefaults(t *testing.T) {
	got, err := LoadConfig(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || !reflect.DeepEqual(got, DefaultConfig()) {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func loadConfigJSON(t *testing.T, content string) AppConfig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestConfigNotifyOnFailure(t *testing.T) {
	for name, tc := range map[string]struct {
		content string
		want    bool
	}{
		"key absent keeps default": {`{"max_render_dpi": 600}`, true},
		"explicit true":            {`{"notify_on_failure": true}`, true},
		"explicit false":           {`{"notify_on_failure": false}`, false},
		"malformed keeps default":  {`{"notify_on_failure": "no"}`, true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := loadConfigJSON(t, tc.content).NotifyOnFailure; got != tc.want {
				t.Fatalf("NotifyOnFailure = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestConfigKeepJobHistory(t *testing.T) {
	if !DefaultConfig().KeepJobHistory {
		t.Fatal("job history should be kept by default")
	}
	for name, tc := range map[string]struct {
		content string
		want    bool
	}{
		"key absent keeps default": {`{"max_render_dpi": 600}`, true},
		"explicit true":            {`{"keep_job_history": true}`, true},
		"explicit false":           {`{"keep_job_history": false}`, false},
		"malformed keeps default":  {`{"keep_job_history": "no"}`, true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := loadConfigJSON(t, tc.content).KeepJobHistory; got != tc.want {
				t.Fatalf("KeepJobHistory = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestConfigKeepJobHistoryFalseSurvivesSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := DefaultConfig()
	cfg.KeepJobHistory = false
	if err := cfg.SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadConfig(path); err != nil || got.KeepJobHistory {
		t.Fatalf("reloaded = %+v, %v", got, err)
	}
}

func TestSniffDocumentFormat(t *testing.T) {
	cases := map[string]string{
		"%PDF-1.4":                          MIMEPDF,
		"\r\n\n%PDF-1.7":                    MIMEPDF,
		string([]byte{0xFF, 0xD8, 0xFF, 0}): MIMEJPEG,
		"\x89PNG\r\n":                       MIMEPNG,
		"UNIRAST\x00more":                   MIMEURF,
		"RaS2rest":                          MIMEPWG,
		"hello":                             "",
		"%PD":                               "",
		"":                                  "",
	}
	for in, want := range cases {
		got, ok := SniffDocumentFormat([]byte(in))
		if got.MIME != want || ok != (want != "") {
			t.Fatalf("SniffDocumentFormat(%q) = %q, %v, want %q", in, got.MIME, ok, want)
		}
	}
}

func TestJobLogCapAndCancel(t *testing.T) {
	var log JobLog
	for i := 0; i < MaxLogEntries+50; i++ {
		log.Push(NewJobEntry("p1", "u", "f.pdf", 1))
	}
	if n := len(log.List()); n != MaxLogEntries {
		t.Fatalf("cap = %d, want %d", n, MaxLogEntries)
	}
	entry := NewJobEntry("p1", "u", "g.pdf", 2)
	log.Push(entry)
	if err := log.Cancel("missing"); err != ErrJobNotFound {
		t.Fatalf("expected ErrJobNotFound, got %v", err)
	}
	if err := log.Cancel(entry.ID); err != nil {
		t.Fatal(err)
	}
	if got := log.List()[len(log.List())-1].Status; got != JobCancelled {
		t.Fatalf("status = %s", got)
	}
	if csv, err := log.ToCSV(); err != nil || !strings.HasPrefix(csv, "id,printer,user,file,pages,status") {
		t.Fatalf("bad csv: %q %v", csv, err)
	}
}

func TestAirPrintTXTKeys(t *testing.T) {
	info := NewPrinterInfo("hp", "OfficeJet", "OfficeJet (PC)", "Lab", true)
	color := true
	info.ColorCapable = &color
	txt := BuildAirPrintTXT(info, "some-uuid", "PC", "192.168.1.5", 631)
	for _, key := range []string{"rp", "ty", "note", "product", "pdl", "Color", "Duplex", "adminurl", "priority", "qtotal", "txtvers", "UUID"} {
		if _, ok := txt[key]; !ok {
			t.Fatalf("missing TXT key %s", key)
		}
	}
	if txt["rp"] != "ipp/print/hp" {
		t.Fatalf("rp = %s", txt["rp"])
	}
	if txt["UUID"] != "some-uuid" {
		t.Fatalf("UUID = %s", txt["UUID"])
	}
	if txt["pdl"] != AdvertisedPDL || txt["URF"] != URFCaps {
		t.Fatalf("incorrect raster capabilities: %v", txt)
	}
	if !strings.Contains(txt["adminurl"], "192.168.1.5:631") {
		t.Fatalf("adminurl = %s", txt["adminurl"])
	}
	mono := false
	info.ColorCapable = &mono
	txt = BuildAirPrintTXT(info, "some-uuid", "PC", "192.168.1.5", 631)
	if txt["Color"] != "F" || strings.Contains(txt["URF"], "SRGB24") {
		t.Fatalf("monochrome TXT = %v", txt)
	}
	info.ColorCapable = nil
	if txt = BuildAirPrintTXT(info, "some-uuid", "PC", "192.168.1.5", 631); txt["Color"] != "F" {
		t.Fatalf("unknown color capability must be conservative: %v", txt)
	}
}

func TestAirPrintTXTDuplex(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name    string
		capable *bool
		want    string
	}{
		{"driver reports and accepts duplex", &yes, "T"},
		{"driver cannot duplex", &no, "F"},
		{"unknown capability is conservative", nil, "F"},
	} {
		info := NewPrinterInfo("hp", "OfficeJet", "OfficeJet (PC)", "Lab", true)
		info.DuplexCapable = tc.capable
		txt := BuildAirPrintTXT(info, "some-uuid", "PC", "192.168.1.5", 631)
		if txt["Duplex"] != tc.want {
			t.Errorf("%s: Duplex = %q, want %q", tc.name, txt["Duplex"], tc.want)
		}
		// DM1 (back side normal) is in URF either way; Duplex and
		// sides-supported say whether two-sided output is offered.
		if !strings.HasSuffix(txt["URF"], ",DM1") {
			t.Errorf("%s: URF = %q", tc.name, txt["URF"])
		}
	}
}

func TestPrinterConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.json"
	want := DefaultConfig()
	want.Printers = []PrinterEntry{{WindowsName: "Office", AirPrintName: "Office (PC)", Enabled: true}}
	if err := want.SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil || len(got.Printers) != 1 || got.Printers[0] != want.Printers[0] {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestJobIDsUnique(t *testing.T) {
	a := NewJobEntry("p", "u", "a.pdf", 1)
	b := NewJobEntry("p", "u", "b.pdf", 1)
	if a.ID == b.ID {
		t.Fatal("duplicate job ids")
	}
}

func TestPrinterEntryVirtualOverrideRoundTrip(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name     string
		override *bool
		wantJSON string
	}{
		{"automatic", nil, ""},
		{"virtual", &yes, `"virtual_override": true`},
		{"physical", &no, `"virtual_override": false`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := t.TempDir() + "/config.json"
			cfg := DefaultConfig()
			cfg.Printers = []PrinterEntry{{WindowsName: "Office", Enabled: true, VirtualOverride: tc.override}}
			if err := cfg.SaveConfig(path); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if tc.override == nil && strings.Contains(string(data), "virtual_override") {
				t.Fatalf("nil override must be omitted: %s", data)
			}
			if tc.override != nil && !strings.Contains(string(data), tc.wantJSON) {
				t.Fatalf("missing %s in %s", tc.wantJSON, data)
			}
			got, err := LoadConfig(path)
			if err != nil || len(got.Printers) != 1 {
				t.Fatalf("got %+v err %v", got, err)
			}
			if gotOverride := got.Printers[0].VirtualOverride; (gotOverride == nil) != (tc.override == nil) ||
				gotOverride != nil && *gotOverride != *tc.override {
				t.Fatalf("override = %v, want %v", gotOverride, tc.override)
			}
		})
	}
}

func TestPrinterInfoEffectiveVirtual(t *testing.T) {
	yes, no := true, false
	p := NewPrinterInfo("id", "Name", "Name (PC)", "", false)
	p.SetDetectedVirtual(true, "port nul:")
	if !p.IsVirtual || !p.DetectedVirtual || p.VirtualReason != "port nul:" {
		t.Fatalf("detected verdict not applied: %+v", p)
	}
	p.SetVirtualOverride(&no)
	if p.IsVirtual || !p.DetectedVirtual {
		t.Fatalf("override must win and keep the detected verdict: %+v", p)
	}
	p.SetDetectedVirtual(false, "no virtual signs found")
	p.SetVirtualOverride(&yes)
	if !p.IsVirtual || p.DetectedVirtual {
		t.Fatalf("virtual override ignored: %+v", p)
	}
	p.SetVirtualOverride(nil)
	if p.IsVirtual || p.VirtualOverride != nil {
		t.Fatalf("clearing the override must restore the detected verdict: %+v", p)
	}
	// The override is copied, so later edits of the argument do not leak in.
	p.SetVirtualOverride(&yes)
	yes = false
	if !p.IsVirtual || !*p.VirtualOverride {
		t.Fatalf("override aliases the caller's variable: %+v", p)
	}
}
