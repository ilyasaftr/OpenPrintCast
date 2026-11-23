package winspool

import (
	"bufio"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"strconv"
)

// pdfWriter writes a PDF made of image pages and nothing else: objects, a
// cross-reference table and a trailer. Every page is written in full as it
// is added, so only one page is ever in memory.
type pdfWriter struct {
	out     *bufio.Writer
	written int64
	offsets []int64 // byte offset of each object, indexed by object number
	pages   []int   // object numbers of the page objects
}

// Fixed object numbers; page objects follow them.
const (
	pdfCatalogObject = 1
	pdfPagesObject   = 2
	pdfFirstObject   = 3
)

const (
	pdfOutputBuffer = 64 * 1024
	// pdfBinaryMarker is the comment that tells transfer programs the file
	// holds binary data.
	pdfBinaryMarker = "%\xe2\xe3\xcf\xd3\n"
)

// Colour spaces and stream filters the writer emits.
const (
	pdfDeviceGray = "DeviceGray"
	pdfDeviceRGB  = "DeviceRGB"
	pdfDeviceCMYK = "DeviceCMYK"
	pdfFlate      = "FlateDecode"
	pdfDCT        = "DCTDecode"
)

// pdfImage describes one page-sized image XObject. body writes the encoded
// stream data, already encoded with filter.
type pdfImage struct {
	width, height int
	colorSpace    string
	filter        string
	// invertedDecode adds a /Decode array that flips every component, for
	// the inverted CMYK that Adobe applications store in JPEG files.
	invertedDecode bool
	body           func(io.Writer) error
}

func newPDFWriter(w io.Writer) (*pdfWriter, error) {
	p := &pdfWriter{out: bufio.NewWriterSize(w, pdfOutputBuffer), offsets: make([]int64, pdfFirstObject)}
	if err := p.raw("%PDF-1.4\n" + pdfBinaryMarker); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *pdfWriter) raw(s string) error {
	n, err := p.out.WriteString(s)
	p.written += int64(n)
	return err
}

func (p *pdfWriter) printf(format string, args ...any) error {
	return p.raw(fmt.Sprintf(format, args...))
}

// beginObject records the offset of object id and opens it.
func (p *pdfWriter) beginObject(id int) error {
	for len(p.offsets) <= id {
		p.offsets = append(p.offsets, 0)
	}
	p.offsets[id] = p.written
	return p.printf("%d 0 obj\n", id)
}

func (p *pdfWriter) object(id int, body string) error {
	if err := p.beginObject(id); err != nil {
		return err
	}
	return p.raw(body + "\nendobj\n")
}

// countedWriter counts the bytes that pass through it, so a stream's length
// is known without holding the stream in memory.
type countedWriter struct {
	p *pdfWriter
	n int64
}

func (c *countedWriter) Write(b []byte) (int, error) {
	n, err := c.p.out.Write(b)
	c.p.written += int64(n)
	c.n += int64(n)
	return n, err
}

// addPage appends a page of widthPt x heightPt points filled by img.
func (p *pdfWriter) addPage(img pdfImage, widthPt, heightPt float64) error {
	imageID := p.nextObject()
	lengthID, contentID, pageID := imageID+1, imageID+2, imageID+3
	if err := p.writeImage(imageID, lengthID, img); err != nil {
		return err
	}
	content := fmt.Sprintf("q %s 0 0 %s 0 0 cm /Im0 Do Q", pdfNumber(widthPt), pdfNumber(heightPt))
	if err := p.object(contentID, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)); err != nil {
		return err
	}
	page := fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
		pdfPagesObject, pdfNumber(widthPt), pdfNumber(heightPt), imageID, contentID)
	if err := p.object(pageID, page); err != nil {
		return err
	}
	p.pages = append(p.pages, pageID)
	return nil
}

// nextObject is the number the next object will take.
func (p *pdfWriter) nextObject() int { return max(len(p.offsets), pdfFirstObject) }

// writeImage writes the image stream with an indirect /Length that follows
// the stream, which lets the data go straight to the output.
func (p *pdfWriter) writeImage(id, lengthID int, img pdfImage) error {
	if err := p.beginObject(id); err != nil {
		return err
	}
	dict := fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /%s /BitsPerComponent 8 /Filter /%s",
		img.width, img.height, img.colorSpace, img.filter)
	if img.invertedDecode {
		dict += " /Decode [1 0 1 0 1 0 1 0]"
	}
	if err := p.printf("%s /Length %d 0 R >>\nstream\n", dict, lengthID); err != nil {
		return err
	}
	counter := &countedWriter{p: p}
	if err := img.body(counter); err != nil {
		return err
	}
	if err := p.raw("\nendstream\nendobj\n"); err != nil {
		return err
	}
	return p.object(lengthID, strconv.FormatInt(counter.n, 10))
}

// finish writes the page tree, the catalog, the cross-reference table and
// the trailer, and flushes the output.
func (p *pdfWriter) finish() error {
	if len(p.pages) == 0 {
		return fmt.Errorf("PDF has no pages")
	}
	kids := ""
	for _, id := range p.pages {
		kids += fmt.Sprintf("%d 0 R ", id)
	}
	if err := p.object(pdfPagesObject, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids, len(p.pages))); err != nil {
		return err
	}
	if err := p.object(pdfCatalogObject, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pdfPagesObject)); err != nil {
		return err
	}
	if err := p.writeTrailer(); err != nil {
		return err
	}
	return p.out.Flush()
}

func (p *pdfWriter) writeTrailer() error {
	xref := p.written
	if err := p.printf("xref\n0 %d\n0000000000 65535 f \n", len(p.offsets)); err != nil {
		return err
	}
	for _, offset := range p.offsets[1:] {
		if err := p.printf("%010d 00000 n \n", offset); err != nil {
			return err
		}
	}
	return p.printf("trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(p.offsets), pdfCatalogObject, xref)
}

// pdfNumber formats a coordinate with the precision a page size needs.
func pdfNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

// contextWriter stops writing as soon as ctx ends.
type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (c contextWriter) Write(b []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.w.Write(b)
}

// flateImageBody returns a stream body that compresses height rows of
// rowBytes bytes each, as produced by fill, into the zlib data FlateDecode
// expects, checking ctx for every row.
func flateImageBody(ctx context.Context, rowBytes, height int, fill func(y int, row []byte)) func(io.Writer) error {
	return func(w io.Writer) error {
		compressor := zlib.NewWriter(contextWriter{ctx, w})
		row := make([]byte, rowBytes)
		for y := 0; y < height; y++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			fill(y, row)
			if _, err := compressor.Write(row); err != nil {
				return err
			}
		}
		return compressor.Close()
	}
}
