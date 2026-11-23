package winspool

import (
	"bytes"
	"context"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// registryFixtures returns one printable document per registered format.
func registryFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	return map[string][]byte{
		core.MIMEPDF:  testPDF(),
		core.MIMEURF:  fixture(t, "cups-color.urf"),
		core.MIMEPWG:  fixture(t, "cups-gray.pwg"),
		core.MIMEJPEG: encodeJPEG(t, img),
		core.MIMEPNG:  encodePNG(t, img),
	}
}

// TestEveryRegisteredFormatHasHandlers is the open/closed guard: the core
// registry is what clients are told the bridge prints, so each of its formats
// needs a fixture here, must be recognized by its own detector, and must go
// through both the spooler dispatch and the folder writer.
func TestEveryRegisteredFormatHasHandlers(t *testing.T) {
	fixtures := registryFixtures(t)
	for _, format := range core.DocumentFormats() {
		t.Run(format.MIME, func(t *testing.T) {
			data, ok := fixtures[format.MIME]
			if !ok {
				t.Fatalf("no fixture for the registered format %s: add one", format.MIME)
			}
			if sniffed, ok := core.SniffDocumentFormat(data); !ok || sniffed.MIME != format.MIME {
				t.Fatalf("the fixture sniffs as %q, %v", sniffed.MIME, ok)
			}
			if _, ok := documentHandlers[format.MIME]; !ok {
				t.Fatalf("%s is registered but has no handler", format.MIME)
			}
			for _, name := range append([]string{format.MIME}, format.Aliases...) {
				requireDispatchRenders(t, data, name)
				requireFolderSaves(t, data, name)
			}
		})
	}
	for mime := range documentHandlers {
		if _, ok := core.LookupDocumentFormat(mime); !ok {
			t.Errorf("handler for %s, which is not a registered format", mime)
		}
	}
}

// requireDispatchRenders runs the spooler's page dispatch with a recording
// renderer (the draw callback) instead of a Windows device context.
func requireDispatchRenders(t *testing.T, data []byte, mime string) {
	t.Helper()
	if !needsRasterPath(mime) {
		t.Fatalf("%s would be sent RAW to the printer instead of rendered", mime)
	}
	var recorder reportRecorder
	if err := recorder.render(data, mime, RenderSettings{}); err != nil {
		t.Fatalf("render as %s: %v", mime, err)
	}
	if len(recorder.events) < 2 {
		t.Fatalf("render as %s drew nothing: %v", mime, recorder.events)
	}
}

// requireFolderSaves pushes the document through the folder writer and
// reopens the saved PDF.
func requireFolderSaves(t *testing.T, data []byte, mime string) {
	t.Helper()
	folder := t.TempDir()
	err := NewSystemBackend().SpoolFolderJobContext(context.Background(), data, mime, core.PrintOptions{}, folder, "doc.pdf")
	if err != nil {
		t.Fatalf("save as %s: %v", mime, err)
	}
	saved, err := filepath.Glob(filepath.Join(folder, "*.pdf"))
	if err != nil || len(saved) != 1 {
		t.Fatalf("saved files = %v, %v", saved, err)
	}
	pdf, err := os.ReadFile(saved[0])
	if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF")) || len(savedPDFPages(t, pdf)) == 0 {
		t.Fatalf("the PDF saved for %s does not reopen: %v", mime, err)
	}
}

func TestUnregisteredFormatsAreNotRendered(t *testing.T) {
	for _, mime := range []string{"application/postscript", "application/octet-stream", "image/tiff", ""} {
		if needsRasterPath(mime) {
			t.Errorf("needsRasterPath(%q) = true", mime)
		}
	}
}
