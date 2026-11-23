package winspool

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klippa-app/go-pdfium/requests"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// pageBox is the MediaBox size of one page in points.
type pageBox struct{ width, height float64 }

// savedPDFPages reopens a PDF with PDFium and returns the MediaBox of every
// page; it fails the test when PDFium cannot open the file.
func savedPDFPages(t *testing.T, data []byte) []pageBox {
	t.Helper()
	instance, err := acquirePDFInstance()
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		t.Fatalf("PDFium cannot open the saved PDF: %v", err)
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		t.Fatal(err)
	}
	boxes := make([]pageBox, count.PageCount)
	for i := range boxes {
		box, err := instance.FPDFPage_GetMediaBox(&requests.FPDFPage_GetMediaBox{
			Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}},
		})
		if err != nil {
			t.Fatal(err)
		}
		boxes[i] = pageBox{float64(box.Right - box.Left), float64(box.Top - box.Bottom)}
	}
	return boxes
}

func sameBox(a, b pageBox) bool {
	const tolerance = 0.05
	return abs(a.width-b.width) <= tolerance && abs(a.height-b.height) <= tolerance
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func requireBoxes(t *testing.T, data []byte, want ...pageBox) {
	t.Helper()
	got := savedPDFPages(t, data)
	if len(got) != len(want) {
		t.Fatalf("pages = %v, want %v", got, want)
	}
	for i := range got {
		if !sameBox(got[i], want[i]) {
			t.Fatalf("pages = %v, want %v", got, want)
		}
	}
}

// savedFolderPDF saves a document with SpoolFolderJobContext into a new
// folder and returns the folder and the contents of the one saved file.
func savedFolderPDF(t *testing.T, data []byte, mime string, options core.PrintOptions) (folder string, saved []byte) {
	t.Helper()
	folder = t.TempDir()
	if err := NewSystemBackend().SpoolFolderJobContext(context.Background(), data, mime, options, folder, "original.pdf"); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(folder, "original-*.pdf"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("output files = %v, error = %v", paths, err)
	}
	if saved, err = os.ReadFile(paths[0]); err != nil {
		t.Fatal(err)
	}
	requireFolderHoldsOnly(t, folder, 1)
	return folder, saved
}

// requireFolderHoldsOnly checks that folder holds count regular files and no
// staging directory.
func requireFolderHoldsOnly(t *testing.T, folder string, count int) {
	t.Helper()
	entries, err := os.ReadDir(folder)
	if err != nil || len(entries) != count {
		t.Fatalf("folder entries = %v, error = %v", entries, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatalf("staging directory left behind: %s", entry.Name())
		}
	}
}

// pdfWithPages builds a PDF whose page i has the given size in points.
func pdfWithPages(sizes []pageBox, catalogExtra string) []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	kids := ""
	objects := []string{"", ""}
	for i, size := range sizes {
		pageID, contentID := 3+2*i, 4+2*i
		kids += fmt.Sprintf("%d 0 R ", pageID)
		content := fmt.Sprintf("%d 0 0 rg 0 0 %v %v re f", i%2, size.width, size.height)
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %v %v] /Contents %d 0 R >>", size.width, size.height, contentID),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content))
	}
	objects[0] = "<< /Type /Catalog /Pages 2 0 R " + catalogExtra + ">>"
	objects[1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(sizes), kids)
	writeObjects(&out, objects, "/Root 1 0 R")
	return out.Bytes()
}

func writeObjects(out *bytes.Buffer, objects []string, trailerExtra string) {
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = out.Len()
		fmt.Fprintf(out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(out, "trailer\n<< /Size %d %s >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, trailerExtra, xref)
}

// encryptedPDF has a standard security handler whose empty password does not
// match, so PDFium needs a password to open it.
func encryptedPDF() []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	zeros := "<" + strings.Repeat("00", 32) + ">"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] >>",
		"<< /Filter /Standard /V 1 /R 2 /P -4 /O " + zeros + " /U " + zeros + " >>",
	}
	writeObjects(&out, objects, "/Root 1 0 R /Encrypt 4 0 R /ID [<00112233445566778899aabbccddeeff><00112233445566778899aabbccddeeff>]")
	return out.Bytes()
}

func encodeJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withSegment inserts a marker segment right after the SOI marker.
func withSegment(jpegData []byte, marker byte, payload []byte) []byte {
	segment := []byte{0xFF, marker, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], uint16(len(payload)+2))
	segment = append(segment, payload...)
	return append(append(append([]byte{}, jpegData[:2]...), segment...), jpegData[2:]...)
}

func jfifSegment(units byte, x, y uint16) []byte {
	payload := append([]byte("JFIF\x00\x01\x01"), units, 0, 0, 0, 0, 0, 0)
	binary.BigEndian.PutUint16(payload[8:], x)
	binary.BigEndian.PutUint16(payload[10:], y)
	return payload
}

// exifSegment is a little-endian EXIF block whose first directory holds a
// resolution of x by y dots per unit (2 = inch, 3 = centimetre).
func exifSegment(x, y uint32, unit uint16) []byte {
	tiff := []byte("II*\x00\x08\x00\x00\x00\x03\x00")
	entry := func(tag, kind uint16, count, value uint32) []byte {
		e := make([]byte, 12)
		binary.LittleEndian.PutUint16(e, tag)
		binary.LittleEndian.PutUint16(e[2:], kind)
		binary.LittleEndian.PutUint32(e[4:], count)
		binary.LittleEndian.PutUint32(e[8:], value)
		return e
	}
	const rationalsAt = 8 + 2 + 3*12 + 4
	tiff = append(tiff, entry(tiffTagXResolution, tiffTypeRational, 1, rationalsAt)...)
	tiff = append(tiff, entry(tiffTagYResolution, tiffTypeRational, 1, rationalsAt+8)...)
	tiff = append(tiff, entry(tiffTagResolutionUnit, tiffTypeShort, 1, uint32(unit))...)
	tiff = append(tiff, 0, 0, 0, 0)
	for _, v := range []uint32{x, 1, y, 1} {
		tiff = binary.LittleEndian.AppendUint32(tiff, v)
	}
	return append([]byte("Exif\x00\x00"), tiff...)
}
