package net

import (
	"slices"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// gapSpooler is a driver whose reported capabilities are set by the test.
type gapSpooler struct {
	*mediaSpooler
	orientation bool
	hasDefault  bool
	dpi         int
}

func (g *gapSpooler) OrientationSupported(string) bool { return g.orientation }

func (g *gapSpooler) DefaultMediaDetails(string) (core.MediaDetails, bool) {
	if !g.hasDefault {
		return core.MediaDetails{}, false
	}
	return core.MediaDetails{
		Width: 21000, Height: 29700, Left: 300, Right: 300, Top: 400, Bottom: 400,
		DPIX: g.dpi, DPIY: g.dpi, MarginsKnown: true,
	}, true
}

func (g *gapSpooler) SupportedMedia(string) []core.MediaSize {
	if !g.hasDefault {
		return nil
	}
	return g.mediaSpooler.SupportedMedia("")
}

func TestIPP20GapsReportsEveryMissingRequirement(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec gapSpooler
		want []string
	}{
		{"all reported", gapSpooler{orientation: true, hasDefault: true, dpi: 600}, nil},
		{"orientation", gapSpooler{orientation: false, hasDefault: true, dpi: 600}, []string{"orientation"}},
		{"resolution", gapSpooler{orientation: true, hasDefault: true, dpi: 0}, []string{"resolution"}},
		{"orientation and resolution", gapSpooler{orientation: false, hasDefault: true, dpi: 0}, []string{"orientation", "resolution"}},
		{"paper sizes and resolution", gapSpooler{orientation: true, hasDefault: false, dpi: 600}, []string{"paper-sizes", "resolution"}},
		{"nothing reported", gapSpooler{orientation: false, hasDefault: false, dpi: 600}, []string{"orientation", "paper-sizes", "resolution"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := tc.spec
			spec.mediaSpooler = &mediaSpooler{&fakeSpooler{}}
			shared := sharedWith(&spec)
			gaps, known := IPP20Gaps(shared, testPrinter())
			if !known || !slices.Equal(gaps, tc.want) {
				t.Fatalf("IPP20Gaps = %v, known %v; want %v, known", gaps, known, tc.want)
			}
			if got := supportsIPP20(shared, testPrinter()); got != (len(tc.want) == 0) {
				t.Fatalf("supportsIPP20 = %v with gaps %v", got, tc.want)
			}
		})
	}
}

func TestIPP20GapsUnknownWithoutCapabilitySource(t *testing.T) {
	printer := testPrinter()
	noProvider := NewSharedState(nil, &core.JobLog{})
	for name, shared := range map[string]*SharedState{"nil shared": nil, "no spooler": noProvider} {
		gaps, known := IPP20Gaps(shared, printer)
		if known || len(gaps) != 0 {
			t.Errorf("%s: gaps %v known %v, want unknown", name, gaps, known)
		}
		if supportsIPP20(shared, printer) {
			t.Errorf("%s: server must not serve IPP/2.0", name)
		}
	}
}

func TestIPP20GapsFolderDestinationIsAlwaysReady(t *testing.T) {
	folder := core.PrinterInfo{ID: "folder", WindowsName: core.FolderPrinterName}
	// The spooler reports nothing; the destination must not depend on it.
	empty := &gapSpooler{mediaSpooler: &mediaSpooler{&fakeSpooler{}}}
	gaps, known := IPP20Gaps(sharedWith(empty), folder)
	if !known || len(gaps) != 0 {
		t.Fatalf("folder: gaps %v known %v, want ready", gaps, known)
	}
	if !supportsIPP20(sharedWith(empty), folder) {
		t.Fatal("folder destination must be served as IPP/2.0")
	}
}
