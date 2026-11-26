package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// deviceBackend answers supply levels for the strip.
type deviceBackend struct {
	stubBackend
}

func (deviceBackend) DeviceFacts(string) (string, string, []string, int, bool) {
	return "X2NZ461391", "RF11I5", nil, 13956, true
}

func (deviceBackend) SupplyLevels(string) ([]core.Supply, string) {
	return []core.Supply{
		{Name: "Black ink", Color: "black", Kind: core.SupplyInk, Level: 72},
		{Name: "Cyan ink", Color: "cyan", Kind: core.SupplyInk, Level: 45},
		{Name: "Magenta ink", Color: "magenta", Kind: core.SupplyInk, Level: 8},
		{Name: "Yellow ink", Color: "yellow", Kind: core.SupplyInk, Level: 91},
		{Name: "Maintenance box", Kind: core.SupplyWasteInk, Level: 34, Receptacle: true},
	}, ""
}

func TestPrinterRowShowsInkStrip(t *testing.T) {
	app := newTestApp(t)
	app.backend = deviceBackend{}
	tt := ui.NewTester(app.view, 860, 620)
	tt.Advance(500 * time.Millisecond)
	for _, want := range []string{"Black 72%", "Cyan 45%", "Magenta 8%", "Yellow 91%"} {
		if !tt.HasText(want) {
			t.Errorf("row missing %q", want)
		}
	}
	if tt.HasText("Maintenance box") {
		t.Error("receptacles stay out of the strip")
	}
}

func TestPrinterRowQuietWithoutReadings(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	tt.Advance(500 * time.Millisecond)
	for _, unwanted := range []string{"Black 72%", "Cyan 45%", "Magenta 8%"} {
		if tt.HasText(unwanted) {
			t.Errorf("a printer without readings must not draw %q", unwanted)
		}
	}
}

func TestSheetSupplyLevelsTab(t *testing.T) {
	app := newTestApp(t)
	app.backend = deviceBackend{}
	tt := ui.NewTester(app.view, 860, 720)
	if err := tt.Click("Name and location for EPSON L3210 Series"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if tt.HasText("34% full") {
		t.Error("supply levels visible before their tab is chosen")
	}
	if err := tt.Click("Supply Levels"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	for _, want := range []string{"34% full", "Serial number: X2NZ461391", "Firmware: RF11I5", "Pages printed: 13956"} {
		if !tt.HasText(want) {
			t.Errorf("supply tab missing %q", want)
		}
	}
	if tt.HasText("Name on devices") {
		t.Error("general content still visible on the supply tab")
	}
}

func TestProbeInkStripVisual(t *testing.T) {
	if os.Getenv("WRITE_PROBE") == "" {
		t.Skip("visual probe")
	}
	app := newTestApp(t)
	app.backend = deviceBackend{}
	tt := ui.NewTester(app.view, 860, 620)
	tt.Advance(500 * time.Millisecond)
	f, _ := os.Create("/tmp/probe-ink-strip.png")
	defer f.Close()
	png.Encode(f, tt.Image())
}
