package winspool

import (
	"testing"
)

func TestIsVirtualPrinterNameOnly(t *testing.T) {
	for _, name := range []string{
		"Microsoft Print to PDF",
		"Microsoft XPS Document Writer",
		"OneNote",
		"Send To OneNote 2016",
		"  Fax ",
		"FAX",
		"microsoft print to pdf (copy 1)",
		"OneNote for Windows 10",
		"OneNote (Desktop) - Protected",
		"RustDesk Printer",
	} {
		if !IsVirtualPrinter(name) {
			t.Fatalf("IsVirtualPrinter(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"Office LaserJet Pro", "HP LaserJet", "", "   "} {
		if IsVirtualPrinter(name) {
			t.Fatalf("IsVirtualPrinter(%q) = true, want false", name)
		}
	}
}

func TestDescriptorDerivesVirtualFlag(t *testing.T) {
	pdf := NewPrinterDescriptor("Microsoft Print to PDF", false)
	if !pdf.IsVirtual || pdf.IsDefault {
		t.Fatalf("bad pdf descriptor: %+v", pdf)
	}
	real := NewPrinterDescriptor("Office LaserJet", true)
	if real.IsVirtual || !real.IsDefault || real.Name != "Office LaserJet" {
		t.Fatalf("bad laser descriptor: %+v", real)
	}
}

// The old code let the driver verdict overwrite the name verdict, so a
// brand name with an unrecognised driver was reported as physical. Now
// every signal counts, and only physical-port evidence outranks the names.
func TestDescriptorCombinesNameDriverAndPort(t *testing.T) {
	for _, tc := range []struct {
		e       PrinterEvidence
		virtual bool
	}{
		{PrinterEvidence{Name: "Renamed PDF", DriverName: "Microsoft Print To PDF"}, true},
		{PrinterEvidence{Name: "OneNote for Windows 10", DriverName: "Unknown Driver"}, true},
		{PrinterEvidence{Name: "Microsoft Print to PDF", DriverName: "Office LaserJet"}, true},
		{PrinterEvidence{Name: "Microsoft Print to PDF", DriverName: "Office LaserJet", PortName: "192.168.0.5"}, false},
		{PrinterEvidence{Name: "Renamed Fax", DriverName: "unknown", IsFax: true}, true},
		{PrinterEvidence{Name: "Office printer", DriverName: "Office LaserJet"}, false},
	} {
		if d := NewPrinterDescriptorFromEvidence(tc.e, false); d.IsVirtual != tc.virtual {
			t.Errorf("%+v: got %+v", tc.e, d)
		}
	}
}

func TestNewPrinterDescriptorFromEvidenceMapsFields(t *testing.T) {
	d := NewPrinterDescriptorFromEvidence(PrinterEvidence{
		Name: "Brother HL", DriverName: "Brother HL-L2350DW series", PortName: "WSD-8f12b3c4", IsNetwork: true,
	}, true)
	want := PrinterDescriptor{
		Name: "Brother HL", DriverName: "Brother HL-L2350DW series", PortName: "WSD-8f12b3c4",
		IsDefault: true, IsRemote: true, VirtualReason: "network printer connection",
	}
	if d != want {
		t.Fatalf("got %+v, want %+v", d, want)
	}
	v := NewPrinterDescriptorFromEvidence(PrinterEvidence{Name: "Fax", PortName: "SHRFAX:"}, false)
	if !v.IsVirtual || v.IsDefault || v.IsRemote || v.PortName != "SHRFAX:" || v.VirtualReason != "port SHRFAX:" {
		t.Fatalf("bad virtual descriptor: %+v", v)
	}
}

func TestSortPrefersDefaultThenPhysical(t *testing.T) {
	// Default-ness outranks physicality, matching config-override intent.
	printers := []PrinterDescriptor{
		NewPrinterDescriptor("Office LaserJet", false),
		NewPrinterDescriptor("Microsoft Print to PDF", true),
	}
	SortPrinters(printers)
	if printers[0].Name != "Microsoft Print to PDF" {
		t.Fatalf("default should sort first: %+v", printers)
	}
}

func TestSortPhysicalBeforeVirtualAlphabetical(t *testing.T) {
	printers := []PrinterDescriptor{
		NewPrinterDescriptor("Microsoft Print to PDF", false),
		NewPrinterDescriptor("Zebra Label", false),
		NewPrinterDescriptor("Fax", false),
		NewPrinterDescriptor("Alpha Inkjet", false),
	}
	SortPrinters(printers)
	want := []string{"Alpha Inkjet", "Zebra Label", "Fax", "Microsoft Print to PDF"}
	for i, name := range want {
		if printers[i].Name != name {
			t.Fatalf("position %d = %q, want %q (full: %+v)", i, printers[i].Name, name, printers)
		}
	}
}

func TestToCoreConversion(t *testing.T) {
	d := NewPrinterDescriptor("Office LaserJet", true)
	info := d.ToCore("office", "Office AirPrint", "Lab", true)
	if info.ID != "office" || info.WindowsName != "Office LaserJet" ||
		info.AirPrintName != "Office AirPrint" || info.Location != "Lab" || !info.Enabled {
		t.Fatalf("bad core conversion: %+v", info)
	}
	if got := info.DisplayName(); got != "Office AirPrint" {
		t.Fatalf("DisplayName = %q", got)
	}
}
