package winspool

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func pagesRange(first, last int) core.PrintOptions {
	return core.PrintOptions{PageRanges: []core.PageRange{{First: first, Last: last}}}
}

var fivePages = []pageBox{{100, 200}, {110, 210}, {120, 220}, {130, 230}, {140, 240}}

func TestFolderPDFCopiesPageRanges(t *testing.T) {
	source := pdfWithPages(fivePages, "")
	options := core.PrintOptions{PageRanges: []core.PageRange{{First: 2, Last: 2}, {First: 4, Last: 9}}}
	_, saved := savedFolderPDF(t, source, "application/pdf", options)
	requireBoxes(t, saved, fivePages[1], fivePages[3], fivePages[4])
}

func TestFolderPDFWithoutRangesKeepsEveryPage(t *testing.T) {
	_, saved := savedFolderPDF(t, pdfWithPages(fivePages, ""), "application/pdf", core.PrintOptions{})
	requireBoxes(t, saved, fivePages...)
}

func TestFolderPDFIgnoresEverythingButPageRanges(t *testing.T) {
	options := core.PrintOptions{
		Monochrome: true, Orientation: 4, Quality: 3, Sides: core.TwoSidedLongEdge, PaperID: 9,
		Overrides: []core.PageOverride{{Pages: []core.PageRange{{First: 1, Last: 1}}, ColorMode: "monochrome"}},
	}
	if !options.Valid() {
		t.Fatal("test options are invalid")
	}
	_, saved := savedFolderPDF(t, pdfWithPages(fivePages[:2], ""), "application/pdf", options)
	requireBoxes(t, saved, fivePages[0], fivePages[1])
}

func TestFolderPDFDropsDocumentLevelScripts(t *testing.T) {
	extra := "/OpenAction << /S /JavaScript /JS (app.alert\\(1\\)) >> " +
		"/Names << /JavaScript << /Names [(x) << /S /JavaScript /JS (1) >>] >> >> "
	source := pdfWithPages(fivePages[:1], extra)
	if !bytes.Contains(source, []byte("/JavaScript")) {
		t.Fatal("fixture has no script")
	}
	_, saved := savedFolderPDF(t, source, "application/pdf", core.PrintOptions{})
	requireBoxes(t, saved, fivePages[0])
	for _, marker := range []string{"JavaScript", "OpenAction", "app.alert"} {
		if bytes.Contains(saved, []byte(marker)) {
			t.Errorf("saved PDF still contains %q", marker)
		}
	}
}

func TestFolderPDFFailsForUnreadableDocumentsAndWritesNothing(t *testing.T) {
	for name, data := range map[string][]byte{
		"encrypted": encryptedPDF(),
		"corrupt":   []byte("%PDF-1.4\nthis is not a pdf at all"),
		"truncated": pdfWithPages(fivePages, "")[:120],
	} {
		t.Run(name, func(t *testing.T) {
			folder := t.TempDir()
			err := NewSystemBackend().SpoolFolderJobContext(context.Background(), data, "application/pdf", core.PrintOptions{}, folder, "a.pdf")
			if !errors.Is(err, errPDFUnreadable) {
				t.Fatalf("error = %v", err)
			}
			if !strings.Contains(err.Error(), "password-protected or damaged") {
				t.Fatalf("reason does not explain the failure: %v", err)
			}
			requireFolderHoldsOnly(t, folder, 0)
		})
	}
}

func TestFolderJobWithEmptySelectionFailsAndWritesNothing(t *testing.T) {
	for name, tc := range map[string]struct {
		data []byte
		mime string
	}{
		"pdf":  {pdfWithPages(fivePages, ""), "application/pdf"},
		"urf":  {fixture(t, "cups-color.urf"), "image/urf"},
		"pwg":  {fixture(t, "cups-gray.pwg"), "image/pwg-raster"},
		"jpeg": {encodeJPEG(t, image.NewGray(image.Rect(0, 0, 4, 4))), "image/jpeg"},
		"png":  {encodePNG(t, image.NewGray(image.Rect(0, 0, 4, 4))), "image/png"},
	} {
		t.Run(name, func(t *testing.T) {
			folder := t.TempDir()
			err := NewSystemBackend().SpoolFolderJobContext(context.Background(), tc.data, tc.mime, pagesRange(7, 9), folder, "a.pdf")
			if !errors.Is(err, errNoPagesSelected) {
				t.Fatalf("error = %v", err)
			}
			requireFolderHoldsOnly(t, folder, 0)
		})
	}
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pngChunk builds a PNG chunk with its length and CRC.
func pngChunk(kind string, body []byte) []byte {
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(body)))
	chunk = append(chunk, kind...)
	chunk = append(chunk, body...)
	return binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
}

const pngHeaderEnd = pngSignatureSize + pngChunkOverhead + 13 // signature and IHDR

func pngWithChunkAfterHeader(data, chunk []byte) []byte {
	return append(append(append([]byte{}, data[:pngHeaderEnd]...), chunk...), data[pngHeaderEnd:]...)
}

// pngWithSize rewrites the dimensions in the IHDR chunk.
func pngWithSize(data []byte, width, height uint32) []byte {
	header := append([]byte{}, data[pngSignatureSize+8:pngHeaderEnd-4]...)
	binary.BigEndian.PutUint32(header, width)
	binary.BigEndian.PutUint32(header[4:], height)
	return append(append(append([]byte{}, data[:pngSignatureSize]...), pngChunk("IHDR", header)...), data[pngHeaderEnd:]...)
}

func TestFolderJPEGIsEmbeddedWithoutReencoding(t *testing.T) {
	gray := image.NewGray(image.Rect(0, 0, 30, 20))
	rgb := image.NewRGBA(image.Rect(0, 0, 30, 20))
	for _, tc := range []struct {
		name, space string
		img         image.Image
	}{{"gray", "/DeviceGray", gray}, {"rgb", "/DeviceRGB", rgb}} {
		t.Run(tc.name, func(t *testing.T) {
			source := encodeJPEG(t, tc.img)
			_, saved := savedFolderPDF(t, source, "image/jpeg", core.PrintOptions{})
			requireBoxes(t, saved, pageBox{30.0 / defaultImageDPI * 72, 20.0 / defaultImageDPI * 72})
			if !bytes.Contains(saved, source) || !bytes.Contains(saved, []byte("/ColorSpace "+tc.space)) ||
				!bytes.Contains(saved, []byte("/Filter /DCTDecode")) {
				t.Fatal("JPEG data was not embedded as a DCTDecode image")
			}
		})
	}
}

func TestFolderJPEGPageSizeFollowsDensity(t *testing.T) {
	base := encodeJPEG(t, image.NewRGBA(image.Rect(0, 0, 144, 72)))
	for name, tc := range map[string]struct {
		data []byte
		want pageBox
	}{
		"jfif dpi":        {withSegment(base, jpegAPP0, jfifSegment(1, 144, 144)), pageBox{72, 36}},
		"jfif dots/cm":    {withSegment(base, jpegAPP0, jfifSegment(2, 100, 100)), pageBox{144.0 / 254 * 100 * 72 / 100, 72.0 / 254 * 100 * 72 / 100}},
		"jfif ratio":      {withSegment(base, jpegAPP0, jfifSegment(0, 1, 1)), pageBox{144.0 / 300 * 72, 72.0 / 300 * 72}},
		"jfif zero":       {withSegment(base, jpegAPP0, jfifSegment(1, 0, 0)), pageBox{144.0 / 300 * 72, 72.0 / 300 * 72}},
		"jfif absurd":     {withSegment(base, jpegAPP0, jfifSegment(1, 65535, 65535)), pageBox{144.0 / 300 * 72, 72.0 / 300 * 72}},
		"exif dpi":        {withSegment(base, jpegAPP1, exifSegment(72, 72, 2)), pageBox{144, 72}},
		"exif dots/cm":    {withSegment(base, jpegAPP1, exifSegment(100, 100, 3)), pageBox{144.0 / 254 * 100 * 72 / 100, 72.0 / 254 * 100 * 72 / 100}},
		"jfif beats exif": {withSegment(withSegment(base, jpegAPP1, exifSegment(72, 72, 2)), jpegAPP0, jfifSegment(1, 144, 144)), pageBox{72, 36}},
		"no density":      {base, pageBox{144.0 / 300 * 72, 72.0 / 300 * 72}},
	} {
		t.Run(name, func(t *testing.T) {
			_, saved := savedFolderPDF(t, tc.data, "image/jpeg", core.PrintOptions{})
			requireBoxes(t, saved, tc.want)
		})
	}
}

func TestParseJPEGComponentsAndVariants(t *testing.T) {
	frame := func(marker byte, precision byte, components byte) []byte {
		data := []byte{0xFF, jpegSOI}
		payload := []byte{precision, 0, 8, 0, 16, components}
		for i := byte(0); i < components; i++ {
			payload = append(payload, i+1, 0x11, 0)
		}
		data = withSegment(data, marker, payload)
		return append(data, 0xFF, jpegEOI)
	}
	cmyk := withSegment(frame(jpegSOFBaseline, 8, 4), jpegAPP14, []byte("Adobe\x00\x64\x00\x00\x00\x00\x02"))
	info, err := parseJPEG(cmyk)
	if err != nil || info.components != 4 || !info.adobe || info.width != 16 || info.height != 8 {
		t.Fatalf("CMYK info = %+v, error = %v", info, err)
	}
	if got := jpegPageImage(cmyk, info); got.colorSpace != pdfDeviceCMYK || !got.invertedDecode {
		t.Fatalf("Adobe CMYK image = %+v", got)
	}
	plainCMYK, _ := parseJPEG(frame(jpegSOFBaseline, 8, 4))
	if got := jpegPageImage(nil, plainCMYK); got.colorSpace != pdfDeviceCMYK || got.invertedDecode {
		t.Fatalf("plain CMYK image = %+v", got)
	}
	if _, err := parseJPEG(frame(jpegSOFProgress, 8, 3)); err != nil {
		t.Fatalf("progressive JPEG is embeddable: %v", err)
	}
	for name, tc := range map[string]struct {
		data []byte
		want error
	}{
		"arithmetic":   {frame(0xC9, 8, 3), errJPEGUnsupported},
		"lossless":     {frame(0xC3, 8, 3), errJPEGUnsupported},
		"hierarchical": {frame(0xC5, 8, 3), errJPEGUnsupported},
		"12 bit":       {frame(jpegSOFExtended, 12, 3), errJPEGPrecision},
		"two comps":    {frame(jpegSOFBaseline, 8, 2), errJPEGComponents},
		"no end":       {frame(jpegSOFBaseline, 8, 3)[:len(frame(jpegSOFBaseline, 8, 3))-2], errJPEGTruncated},
		"not jpeg":     {[]byte("hello, this is text"), errJPEGNotImage},
		"cut header":   {[]byte{0xFF, jpegSOI, 0xFF, 0xE0, 0x00}, errJPEGTruncated},
	} {
		if _, err := parseJPEG(tc.data); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", name, err, tc.want)
		}
	}
}

func TestFolderCMYKJPEGKeepsItsColorSpaceInThePDF(t *testing.T) {
	data := withSegment(withSegment(append([]byte{0xFF, jpegSOI}, 0xFF, jpegEOI), jpegSOFBaseline,
		[]byte{8, 0, 8, 0, 16, 4, 1, 0x11, 0, 2, 0x11, 0, 3, 0x11, 0, 4, 0x11, 0}), jpegAPP14, []byte("Adobe\x00\x64\x00\x00\x00\x00\x02"))
	_, saved := savedFolderPDF(t, data, "image/jpeg", core.PrintOptions{})
	for _, want := range []string{"/DeviceCMYK", "/Decode [1 0 1 0 1 0 1 0]", "/DCTDecode"} {
		if !bytes.Contains(saved, []byte(want)) {
			t.Errorf("saved PDF lacks %s", want)
		}
	}
	requireBoxes(t, saved, pageBox{16.0 / 300 * 72, 8.0 / 300 * 72})
}

func TestFolderRasterPagesFollowTheirResolution(t *testing.T) {
	tiny := pageBox{4.0 / 300 * 72, 3.0 / 300 * 72}
	large := pageBox{2480.0 / 300 * 72, 3508.0 / 300 * 72}
	for _, tc := range []struct {
		file, mime string
		want       []pageBox
	}{
		{"cups-color.urf", "image/urf", []pageBox{tiny, tiny}},
		{"cups-gray.urf", "image/urf", []pageBox{tiny, tiny}},
		{"cups-color.pwg", "image/pwg-raster", []pageBox{tiny, tiny}},
		{"cups-gray.pwg", "image/pwg-raster", []pageBox{tiny, tiny}},
		{"cups-large.urf", "image/urf", []pageBox{large, large}},
		{"cups-large.pwg", "image/pwg-raster", []pageBox{large, large}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			_, saved := savedFolderPDF(t, fixture(t, tc.file), tc.mime, core.PrintOptions{})
			requireBoxes(t, saved, tc.want...)
		})
	}
}

func TestFolderRasterPageRangeAndColorSpaces(t *testing.T) {
	_, saved := savedFolderPDF(t, fixture(t, "cups-gray.urf"), "image/urf", pagesRange(2, 2))
	if got := savedPDFPages(t, saved); len(got) != 1 || !bytes.Contains(saved, []byte("/DeviceGray")) || bytes.Contains(saved, []byte("/DeviceRGB")) {
		t.Fatalf("gray raster: pages %v", got)
	}
	_, saved = savedFolderPDF(t, fixture(t, "cups-color.pwg"), "image/pwg-raster", core.PrintOptions{})
	if !bytes.Contains(saved, []byte("/DeviceRGB")) || bytes.Contains(saved, []byte("/DeviceGray")) {
		t.Fatal("color raster is not DeviceRGB")
	}
}

// centerColor renders the first page of a PDF and returns its middle pixel.
func centerColor(t *testing.T, data []byte) (r, g, b uint32) {
	t.Helper()
	err := renderPDF(data, 20, 20, false, core.PrintOptions{}, func(img image.Image) error {
		r, g, b, _ = img.At(10, 10).RGBA()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return r >> 8, g >> 8, b >> 8
}

func TestFolderPNGFlattensTransparencyOnWhite(t *testing.T) {
	for name, tc := range map[string]struct {
		pixel   color.NRGBA
		r, g, b uint32
	}{
		"opaque":      {color.NRGBA{200, 10, 30, 255}, 200, 10, 30},
		"transparent": {color.NRGBA{200, 10, 30, 0}, 255, 255, 255},
		"half":        {color.NRGBA{0, 0, 0, 128}, 127, 127, 127},
	} {
		t.Run(name, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, 20, 20))
			for i := 0; i < len(img.Pix); i += 4 {
				copy(img.Pix[i:], []byte{tc.pixel.R, tc.pixel.G, tc.pixel.B, tc.pixel.A})
			}
			_, saved := savedFolderPDF(t, encodePNG(t, img), "image/png", core.PrintOptions{})
			requireBoxes(t, saved, pageBox{20.0 / defaultImageDPI * 72, 20.0 / defaultImageDPI * 72})
			r, g, b := centerColor(t, saved)
			if abs(float64(r)-float64(tc.r)) > 2 || abs(float64(g)-float64(tc.g)) > 2 || abs(float64(b)-float64(tc.b)) > 2 {
				t.Fatalf("center = %d,%d,%d, want %d,%d,%d", r, g, b, tc.r, tc.g, tc.b)
			}
		})
	}
}

func TestFolderPNGSixteenBitAndGray(t *testing.T) {
	wide := image.NewNRGBA64(image.Rect(0, 0, 20, 20))
	gray := image.NewGray16(image.Rect(0, 0, 20, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			wide.SetNRGBA64(x, y, color.NRGBA64{R: 0x8000, G: 0x4000, B: 0xFFFF, A: 0xFFFF})
			gray.SetGray16(x, y, color.Gray16{Y: 0x4000})
		}
	}
	_, saved := savedFolderPDF(t, encodePNG(t, wide), "image/png", core.PrintOptions{})
	if r, g, b := centerColor(t, saved); abs(float64(r)-128) > 2 || abs(float64(g)-64) > 2 || b != 255 {
		t.Fatalf("16-bit center = %d,%d,%d", r, g, b)
	}
	_, saved = savedFolderPDF(t, encodePNG(t, gray), "image/png", core.PrintOptions{})
	if r, g, b := centerColor(t, saved); abs(float64(r)-64) > 2 || r != g || g != b || !bytes.Contains(saved, []byte("/DeviceGray")) {
		t.Fatalf("16-bit gray center = %d,%d,%d", r, g, b)
	}
}

func TestFolderPNGPageSizeFollowsPHYs(t *testing.T) {
	data := encodePNG(t, image.NewGray(image.Rect(0, 0, 144, 72)))
	// 5669 pixels per metre is 144 dpi.
	data = pngWithChunkAfterHeader(data, pngChunk("pHYs", []byte{0, 0, 0x16, 0x25, 0, 0, 0x16, 0x25, 1}))
	dpiX, dpiY := pngDensity(data)
	if abs(dpiX-144) > 0.1 || abs(dpiY-144) > 0.1 {
		t.Fatalf("density = %v x %v", dpiX, dpiY)
	}
	_, saved := savedFolderPDF(t, data, "image/png", core.PrintOptions{})
	requireBoxes(t, saved, pageBox{72, 36})
}

func TestPageSizePointsGuards(t *testing.T) {
	for name, tc := range map[string]struct {
		w, h       int
		dpiX, dpiY float64
		want       pageBox
	}{
		"300 dpi":        {300, 600, 300, 300, pageBox{72, 144}},
		"non square":     {300, 300, 300, 150, pageBox{72, 144}},
		"zero dpi":       {300, 300, 0, 0, pageBox{72, 72}},
		"negative":       {300, 300, -5, 300, pageBox{72, 72}},
		"absurd high":    {300, 300, 1e9, 1e9, pageBox{72, 72}},
		"absurd low":     {300, 300, 0.5, 0.5, pageBox{72, 72}},
		"oversized page": {40000, 20000, 72, 72, pageBox{maxPageDimensionPt, maxPageDimensionPt / 2}},
	} {
		if w, h := pageSizePoints(tc.w, tc.h, tc.dpiX, tc.dpiY); !sameBox(pageBox{w, h}, tc.want) {
			t.Errorf("%s: %v x %v, want %v", name, w, h, tc.want)
		}
	}
}

func TestFolderJobRejectsHugeImagesBeforeAllocating(t *testing.T) {
	hugeJPEG := withSegment([]byte{0xFF, jpegSOI, 0xFF, jpegEOI}, jpegSOFBaseline, []byte{8, 0xFF, 0xFF, 0xFF, 0xFF, 1, 1, 0x11, 0})
	hugePNG := pngWithSize(encodePNG(t, image.NewGray(image.Rect(0, 0, 4, 4))), 65535, 65535)
	hugeRaster := append([]byte("UNIRAST\x00\x00\x00\x00\x01"), make([]byte, 32)...)
	hugeRaster[12] = 8
	copy(hugeRaster[24:], []byte{0, 0, 0xFF, 0xFF, 0, 0, 0xFF, 0xFF})
	copy(hugeRaster[32:], []byte{0, 0, 1, 0x2C})
	for name, tc := range map[string]struct {
		data []byte
		mime string
	}{"jpeg": {hugeJPEG, "image/jpeg"}, "png": {hugePNG, "image/png"}, "urf": {hugeRaster, "image/urf"}} {
		t.Run(name, func(t *testing.T) {
			folder := t.TempDir()
			err := NewSystemBackend().SpoolFolderJobContext(context.Background(), tc.data, tc.mime, core.PrintOptions{}, folder, "a.pdf")
			if err == nil || !strings.Contains(err.Error(), "large") && !strings.Contains(err.Error(), "oversized") {
				t.Fatalf("error = %v", err)
			}
			requireFolderHoldsOnly(t, folder, 0)
		})
	}
}

func TestFolderJobReportsThePageCountOnceBeforeAnyOutput(t *testing.T) {
	for _, tc := range []struct {
		name, mime string
		data       []byte
		options    core.PrintOptions
		total      int
	}{
		{"pdf", "application/pdf", pdfWithPages(fivePages, ""), pagesRange(2, 3), 5},
		{"urf", "image/urf", fixture(t, "cups-color.urf"), pagesRange(1, 1), 2},
		{"pwg", "image/pwg-raster", fixture(t, "cups-gray.pwg"), core.PrintOptions{}, 2},
		{"jpeg", "image/jpeg", encodeJPEG(t, image.NewGray(image.Rect(0, 0, 4, 4))), core.PrintOptions{}, 1},
		{"png", "image/png", encodePNG(t, image.NewGray(image.Rect(0, 0, 4, 4))), core.PrintOptions{}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			folder := t.TempDir()
			var reports []int
			ctx := core.WithPageCounter(context.Background(), func(n int) {
				if entries, _ := os.ReadDir(folder); len(entries) != 0 {
					t.Errorf("output exists before the page count: %v", entries)
				}
				reports = append(reports, n)
			})
			if err := NewSystemBackend().SpoolFolderJobContext(ctx, tc.data, tc.mime, tc.options, folder, "a.pdf"); err != nil {
				t.Fatal(err)
			}
			if len(reports) != 1 || reports[0] != tc.total {
				t.Fatalf("page counts = %v, want [%d]", reports, tc.total)
			}
		})
	}
}

func TestFolderJobCancelledMidJobLeavesNothing(t *testing.T) {
	for _, tc := range []struct {
		name, mime string
		data       []byte
	}{
		{"pdf", "application/pdf", pdfWithPages(fivePages, "")},
		{"raster", "image/urf", fixture(t, "cups-large.urf")},
		{"png", "image/png", encodePNG(t, image.NewGray(image.Rect(0, 0, 40, 40)))},
		{"jpeg", "image/jpeg", encodeJPEG(t, image.NewGray(image.Rect(0, 0, 40, 40)))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			folder := t.TempDir()
			ctx, cancel := context.WithCancel(context.Background())
			ctx = core.WithPageCounter(ctx, func(int) { cancel() })
			err := NewSystemBackend().SpoolFolderJobContext(ctx, tc.data, tc.mime, core.PrintOptions{}, folder, "a.pdf")
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v", err)
			}
			requireFolderHoldsOnly(t, folder, 0)
		})
	}
}

func TestFlateImageBodyStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	rows := 0
	body := flateImageBody(ctx, 1000, 100, func(y int, row []byte) {
		rows++
		if y == 3 {
			cancel()
		}
	})
	if err := body(&bytes.Buffer{}); !errors.Is(err, context.Canceled) || rows != 4 {
		t.Fatalf("error = %v after %d rows", err, rows)
	}
}

func TestFolderJobCancelledDuringCompressionLeavesNothing(t *testing.T) {
	folder := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	staged := &stagedPDF{folder: folder}
	defer staged.discard()
	out, err := newPDFWriter(staged)
	if err != nil {
		t.Fatal(err)
	}
	img := pdfImage{width: 10, height: 100, colorSpace: pdfDeviceGray, filter: pdfFlate,
		body: flateImageBody(ctx, 10, 100, func(y int, row []byte) {
			if y == 50 {
				cancel()
			}
		})}
	if err := out.addPage(img, 10, 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	staged.discard()
	requireFolderHoldsOnly(t, folder, 0)
}

func TestFolderJobsWithTheSameNameGetDistinctFiles(t *testing.T) {
	folder := t.TempDir()
	backend := NewSystemBackend()
	data := pdfWithPages(fivePages[:1], "")
	const jobs = 8
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := backend.SpoolFolderJobContext(context.Background(), data, "application/pdf", core.PrintOptions{}, folder, "Report.pdf"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	requireFolderHoldsOnly(t, folder, jobs)
	entries, _ := os.ReadDir(folder)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "Report-") || filepath.Ext(entry.Name()) != ".pdf" {
			t.Errorf("unexpected name %q", entry.Name())
		}
	}
}

func TestFolderJobNeverShowsAPartialPDF(t *testing.T) {
	folder := t.TempDir()
	var stop, incomplete atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		for !stop.Load() {
			entries, _ := os.ReadDir(folder)
			for _, entry := range entries {
				if path := filepath.Join(folder, entry.Name()); !entry.IsDir() && !completePDFOutput(path) {
					if _, err := os.Stat(path); err == nil {
						incomplete.Store(true)
					}
				}
			}
		}
	}()
	data := fixture(t, "cups-large.urf")
	for i := 0; i < 3; i++ {
		if err := NewSystemBackend().SpoolFolderJobContext(context.Background(), data, "image/urf", core.PrintOptions{}, folder, "big.pdf"); err != nil {
			t.Fatal(err)
		}
	}
	stop.Store(true)
	<-done
	if incomplete.Load() {
		t.Fatal("a PDF that was not complete was visible in the folder")
	}
	requireFolderHoldsOnly(t, folder, 3)
}

func TestFolderJobRejectsUnsafeInputs(t *testing.T) {
	backend := NewSystemBackend()
	ctx := context.Background()
	if err := backend.SpoolFolderJobContext(ctx, []byte("x"), "application/pdf", core.PrintOptions{}, "relative", "a.pdf"); err == nil {
		t.Fatal("relative folder accepted")
	}
	if err := backend.SpoolFolderJobContext(ctx, nil, "application/pdf", core.PrintOptions{}, t.TempDir(), "a.pdf"); err == nil {
		t.Fatal("empty document accepted")
	}
	var target *BackendError
	if err := backend.SpoolFolderJobContext(ctx, []byte("x"), "text/plain", core.PrintOptions{}, t.TempDir(), "a.pdf"); !errors.As(err, &target) || target.Kind != KindUnsupported {
		t.Fatalf("unsupported format error = %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if err := backend.SpoolFolderJobContext(ctx, pdfWithPages(fivePages[:1], ""), "application/pdf", core.PrintOptions{}, missing, "a.pdf"); err == nil {
		t.Fatal("missing folder accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := backend.SpoolFolderJobContext(cancelled, pdfWithPages(fivePages[:1], ""), "application/pdf", core.PrintOptions{}, t.TempDir(), "a.pdf"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
}

func TestPDFPageRange(t *testing.T) {
	for name, tc := range map[string]struct {
		count   int
		options core.PrintOptions
		want    string
		ok      bool
	}{
		"all":      {5, core.PrintOptions{}, "", true},
		"one":      {5, pagesRange(3, 3), "3", true},
		"span":     {5, pagesRange(2, 4), "2-4", true},
		"clamped":  {5, pagesRange(4, 99), "4-5", true},
		"multiple": {9, core.PrintOptions{PageRanges: []core.PageRange{{First: 1, Last: 2}, {First: 5, Last: 5}, {First: 7, Last: 9}}}, "1-2,5,7-9", true},
		"none":     {5, pagesRange(6, 9), "", false},
	} {
		got, ok := pdfPageRange(tc.count, tc.options)
		switch {
		case ok != tc.ok, ok && tc.want == "" && got != nil, ok && tc.want != "" && (got == nil || *got != tc.want):
			t.Errorf("%s: range = %v, ok = %t", name, got, ok)
		}
	}
}

func TestOrdinaryPDFOutputStaysInTheBridgeLocation(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	path, err := allocatePDFJobOutput("")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "OpenPrintCast", "PDFs", "job-"); !strings.HasPrefix(path, want) || filepath.Base(path) != "AirPrint.pdf" {
		t.Fatalf("ordinary job output = %q, want it under %s*", path, want)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("job directory missing: %v", err)
	}
}
