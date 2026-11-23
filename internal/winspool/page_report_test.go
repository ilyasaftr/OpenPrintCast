package winspool

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// reportRecorder logs, in order, every page count reported through the
// context and every page drawn, so tests can assert the count arrives once
// and before the first page is drawn.
type reportRecorder struct{ events []string }

func (r *reportRecorder) context() context.Context {
	return core.WithPageCounter(context.Background(), func(n int) {
		r.events = append(r.events, fmt.Sprintf("report %d", n))
	})
}

func (r *reportRecorder) draw(_ image.Image, page, _ int) error {
	r.events = append(r.events, fmt.Sprintf("draw %d", page))
	return nil
}

// render runs the production dispatcher over data.
func (r *reportRecorder) render(data []byte, mime string, settings RenderSettings) error {
	return renderDocument(r.context(), data, mime, settings, 200, 200, r.draw)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func encodedImage(t *testing.T, encode func(*bytes.Buffer, image.Image) error) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := encode(&buf, image.NewGray(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pageReportCases are valid documents with the page count they contain.
func pageReportCases(t *testing.T) []struct {
	name, mime string
	data       []byte
	pages      int
} {
	return []struct {
		name, mime string
		data       []byte
		pages      int
	}{
		{"pdf", "application/pdf", testPDF(), 2},
		{"urf", "image/urf", fixture(t, "cups-color.urf"), 2},
		{"pwg", "image/pwg-raster", fixture(t, "cups-gray.pwg"), 2},
		{"png", "image/png", encodedImage(t, func(b *bytes.Buffer, i image.Image) error { return png.Encode(b, i) }), 1},
		{"jpeg", "image/jpeg", encodedImage(t, func(b *bytes.Buffer, i image.Image) error { return jpeg.Encode(b, i, nil) }), 1},
	}
}

func expectedEvents(pages int, drawn ...int) []string {
	events := []string{fmt.Sprintf("report %d", pages)}
	for _, page := range drawn {
		events = append(events, fmt.Sprintf("draw %d", page))
	}
	return events
}

func TestPageCountIsReportedOnceBeforeTheFirstPageIsDrawn(t *testing.T) {
	for _, tc := range pageReportCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			var rec reportRecorder
			if err := rec.render(tc.data, tc.mime, RenderSettings{}); err != nil {
				t.Fatal(err)
			}
			var drawn []int
			for page := 1; page <= tc.pages; page++ {
				drawn = append(drawn, page)
			}
			if want := expectedEvents(tc.pages, drawn...); !reflect.DeepEqual(rec.events, want) {
				t.Fatalf("events = %v, want %v", rec.events, want)
			}
		})
	}
}

func TestPageCountIsTheDocumentTotalNotTheSelection(t *testing.T) {
	options := core.PrintOptions{PageRanges: []core.PageRange{{First: 2, Last: 2}}}
	for _, tc := range pageReportCases(t)[:3] {
		t.Run(tc.name, func(t *testing.T) {
			var rec reportRecorder
			settings := RenderSettings{ReversePages: true, options: options}
			if err := rec.render(tc.data, tc.mime, settings); err != nil {
				t.Fatal(err)
			}
			if want := expectedEvents(tc.pages, 2); !reflect.DeepEqual(rec.events, want) {
				t.Fatalf("events = %v, want %v", rec.events, want)
			}
		})
	}
}

func TestPageCountIsReportedWhenRangesSelectNothing(t *testing.T) {
	settings := RenderSettings{options: core.PrintOptions{PageRanges: []core.PageRange{{First: 9, Last: 9}}}}
	var rec reportRecorder
	if err := rec.render(testPDF(), "application/pdf", settings); err == nil {
		t.Fatal("empty page selection accepted")
	}
	if want := expectedEvents(2); !reflect.DeepEqual(rec.events, want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}
}

func TestCorruptDocumentsReportNoPageCount(t *testing.T) {
	urf := fixture(t, "cups-color.urf")
	for name, tc := range map[string]struct {
		mime string
		data []byte
	}{
		"pdf":            {"application/pdf", []byte("not a PDF")},
		"truncated urf":  {"image/urf", urf[:len(urf)-1]},
		"urf bad header": {"image/urf", []byte("UNIRAST?....")},
		"pwg":            {"image/pwg-raster", []byte("RaS2 truncated")},
		"png":            {"image/png", []byte("not an image")},
	} {
		t.Run(name, func(t *testing.T) {
			var rec reportRecorder
			if err := rec.render(tc.data, tc.mime, RenderSettings{}); err == nil {
				t.Fatal("corrupt document accepted")
			}
			if len(rec.events) != 0 {
				t.Fatalf("events = %v, want none", rec.events)
			}
		})
	}
}

func TestURFHeaderCountMismatchReportsNoPageCount(t *testing.T) {
	data := bytes.Clone(fixture(t, "cups-color.urf"))
	data[11]++ // the header claims more pages than the stream holds
	var rec reportRecorder
	if err := rec.render(data, "image/urf", RenderSettings{}); err == nil {
		t.Fatal("mismatching URF accepted")
	}
	if len(rec.events) != 0 {
		t.Fatalf("events = %v, want none", rec.events)
	}
}

func TestRenderingWithoutACounterStillWorks(t *testing.T) {
	var rec reportRecorder
	err := renderDocument(context.Background(), testPDF(), "application/pdf", RenderSettings{}, 200, 200, rec.draw)
	if err != nil || len(rec.events) != 2 {
		t.Fatalf("events = %v, err = %v", rec.events, err)
	}
}
