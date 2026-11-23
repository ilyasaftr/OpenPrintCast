package net

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// registeredFixtures returns a real document for each registered format: the
// CUPS sample PDF and JPEG, the winspool raster samples and a generated PNG.
func registeredFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join("..", "winspool", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewGray(image.Rect(0, 0, 16, 12))); err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{
		core.MIMEPDF:  cupsDocument(t, requestDocument),
		core.MIMEJPEG: cupsDocument(t, "color.jpg"),
		core.MIMEPNG:  pngBytes.Bytes(),
		core.MIMEURF:  read("cups-color.urf"),
		core.MIMEPWG:  read("cups-gray.pwg"),
	}
}

// Every registered format, sent the way iOS sends it (IPP/2.0 Print-Job) to
// the Print to Folder printer, ends as a PDF that PDFium reopens: plain, with
// no declared type (auto-sense), gzip-compressed, and mislabeled.
func TestEveryRegisteredFormatPrintsToTheFolderAtIPP20(t *testing.T) {
	fixtures := registeredFixtures(t)
	for _, format := range core.DocumentFormats() {
		doc, ok := fixtures[format.MIME]
		if !ok {
			t.Fatalf("no fixture for the registered format %s", format.MIME)
		}
		variants := []struct {
			name, declared, compression string
			body                        []byte
		}{
			{"declared", format.MIME, "", doc},
			{"auto-sensed", mimeOctetStream, "", doc},
			{"gzip", format.MIME, compressionGzip, gzipped(t, doc)},
			{"deflate", format.MIME, compressionDeflate, rawDeflated(t, doc)},
			{"mislabeled", "application/vnd.cups-pdf", "", doc},
		}
		for _, v := range variants {
			t.Run(format.MIME+"/"+v.name, func(t *testing.T) {
				printer, shared := folderShared(t)
				req := goipp.NewRequest(ipp20, goipp.OpPrintJob, 1)
				req.Operation.Add(goipp.MakeAttr("document-format", goipp.TagMimeType, goipp.String(v.declared)))
				if v.compression != "" {
					req.Operation.Add(goipp.MakeAttr("compression", goipp.TagKeyword, goipp.String(v.compression)))
				}
				if rsp := serveIPP(req, v.body, printer, shared, "pc", 631); goipp.Status(rsp.Code) != goipp.StatusOk {
					t.Fatalf("Print-Job: %s", goipp.Status(rsp.Code))
				}
				if pages := savedPageCount(t, waitSavedPDF(t, shared, printer)); pages < 1 {
					t.Fatalf("saved PDF has %d pages", pages)
				}
			})
		}
	}
}
