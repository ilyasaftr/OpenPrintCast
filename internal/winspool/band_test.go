package winspool

import (
	"testing"
)

func TestBandsCoverEveryRowWithoutOverlap(t *testing.T) {
	if got := bandRanges(0); len(got) != 0 {
		t.Fatalf("bandRanges(0) = %+v", got)
	}
	if got := bandRanges(100); len(got) != 1 || got[0] != (BandRange{Y0: 0, Rows: 100}) {
		t.Fatalf("bandRanges(100) = %+v", got)
	}
	if got := bandRanges(256); len(got) != 1 || got[0] != (BandRange{Y0: 0, Rows: 256}) {
		t.Fatalf("bandRanges(256) = %+v", got)
	}
	bands := bandRanges(600)
	want := []BandRange{{Y0: 0, Rows: 256}, {Y0: 256, Rows: 256}, {Y0: 512, Rows: 88}}
	if len(bands) != len(want) {
		t.Fatalf("bandRanges(600) = %+v", bands)
	}
	var covered uint32
	prev := uint32(0)
	for i, b := range bands {
		if b != want[i] {
			t.Fatalf("bandRanges(600) = %+v, want %+v", bands, want)
		}
		if b.Y0 != prev {
			t.Fatalf("gap/overlap at band %d: %+v", i, b)
		}
		if b.Rows > SpoolBandRows {
			t.Fatalf("band %d exceeds SpoolBandRows: %+v", i, b)
		}
		covered += b.Rows
		prev = b.Y0 + b.Rows
	}
	if covered != 600 {
		t.Fatalf("covered %d rows, want 600", covered)
	}
	if got := BandRanges(600); len(got) != 3 {
		t.Fatalf("exported BandRanges(600) has %d bands", len(got))
	}
}

func TestBandDestinationMatchesPythonMapping(t *testing.T) {
	// 600-row source onto a 600-row page: identity bands.
	y0, y1, ok := bandDestination(0, 256, 600, 600, 0)
	if !ok || y0 != 0 || y1 != 256 {
		t.Fatalf("got (%d,%d,%v)", y0, y1, ok)
	}
	y0, y1, ok = bandDestination(512, 88, 600, 600, 0)
	if !ok || y0 != 512 || y1 != 600 {
		t.Fatalf("got (%d,%d,%v)", y0, y1, ok)
	}
	// Degenerate empty bands are skipped like "if dst_y1 <= dst_y0".
	if _, _, ok := bandDestination(0, 0, 600, 600, 0); ok {
		t.Fatal("empty band should be skipped")
	}
	if _, _, ok := bandDestination(0, 10, 0, 600, 0); ok {
		t.Fatal("empty source should be skipped")
	}
	// Centering offset applies to both edges.
	y0, y1, ok = bandDestination(0, 600, 600, 800, 100)
	if !ok || y0 != 100 || y1 != 900 {
		t.Fatalf("got (%d,%d,%v)", y0, y1, ok)
	}
	// Exported wrapper agrees.
	if a0, a1, aok := BandDestination(0, 256, 600, 600, 0); !aok || a0 != 0 || a1 != 256 {
		t.Fatalf("BandDestination got (%d,%d,%v)", a0, a1, aok)
	}
}

func TestWidthAlignmentRemovesDIBPadding(t *testing.T) {
	for in, want := range map[uint32]uint32{
		0: 0, 1: 4, 4: 4, 5: 8, 1920: 1920, 1921: 1924,
	} {
		if got := alignWidthToDword(in); got != want {
			t.Fatalf("alignWidthToDword(%d) = %d, want %d", in, got, want)
		}
		if got := AlignWidthToDword(in); got != want {
			t.Fatalf("AlignWidthToDword(%d) = %d, want %d", in, got, want)
		}
	}
}
