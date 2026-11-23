package core

import (
	"bytes"
	"slices"
	"strings"
)

// DocumentFormat is one document type the bridge accepts, advertises and
// prints. The registry below is the single source of truth: IPP acceptance,
// document-format-supported, the TXT `pdl` key and the spoolers' dispatch
// all derive from it, so the bridge cannot accept a format it does not
// advertise or advertise one it cannot print. Adding a format is adding an
// entry here and a print handler in internal/winspool (a test enforces both).
type DocumentFormat struct {
	// MIME is the canonical media type, advertised and handed to spoolers.
	MIME string
	// Aliases are other spellings clients send for the same data. They are
	// accepted but never advertised.
	Aliases []string
	// Sniff reports whether data starts like this format.
	Sniff func(data []byte) bool
}

var (
	magicPDF  = []byte("%PDF")
	magicJPEG = []byte{0xFF, 0xD8, 0xFF}
	magicPNG  = []byte{0x89, 'P', 'N', 'G'}
	magicURF  = []byte("UNIRAST")
	magicPWG  = []byte("RaS2")
)

// Canonical media types of the registered formats.
const (
	MIMEPDF  = "application/pdf"
	MIMEURF  = "image/urf"
	MIMEPWG  = "image/pwg-raster"
	MIMEJPEG = "image/jpeg"
	MIMEPNG  = "image/png"
)

// MIMEOctetStream is the RFC 8011 auto-sense media type: the client leaves
// the format to the printer. It is not a format of its own.
const MIMEOctetStream = "application/octet-stream"

// documentFormats is in advertising order. The aliases are the spellings
// seen in the wild: CUPS names its internal PDF types application/vnd.cups-pdf
// and application/vnd.cups-pdf-banner.
var documentFormats = []DocumentFormat{
	{MIMEPDF, []string{"application/x-pdf", "application/vnd.cups-pdf", "application/vnd.cups-pdf-banner"}, sniffPDF},
	{MIMEURF, nil, hasPrefix(magicURF)},
	{MIMEPWG, nil, hasPrefix(magicPWG)},
	{MIMEJPEG, []string{"image/jpg"}, hasPrefix(magicJPEG)},
	{MIMEPNG, []string{"image/x-png"}, hasPrefix(magicPNG)},
}

func hasPrefix(magic []byte) func([]byte) bool {
	return func(data []byte) bool { return bytes.HasPrefix(data, magic) }
}

// sniffPDF follows the CUPS mime.types rule for PDF, `^[\n\r]*%PDF`: line
// breaks may precede the header.
func sniffPDF(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimLeft(data, "\r\n"), magicPDF)
}

// NormalizeMIME is a media type without its parameters, trimmed and in lower
// case (media types are case-insensitive, RFC 2045).
func NormalizeMIME(mime string) string {
	return strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
}

// DocumentFormats returns the registered formats in advertising order.
func DocumentFormats() []DocumentFormat {
	return append([]DocumentFormat(nil), documentFormats...)
}

// AdvertisedMIMEs lists the canonical media types of the registered formats.
func AdvertisedMIMEs() []string {
	mimes := make([]string, len(documentFormats))
	for i, f := range documentFormats {
		mimes[i] = f.MIME
	}
	return mimes
}

// LookupDocumentFormat finds the format a declared media type names, by its
// canonical type or an alias; parameters and case are ignored.
func LookupDocumentFormat(declared string) (DocumentFormat, bool) {
	name := NormalizeMIME(declared)
	for _, f := range documentFormats {
		if f.MIME == name || slices.Contains(f.Aliases, name) {
			return f, true
		}
	}
	return DocumentFormat{}, false
}

// SniffDocumentFormat identifies a document by its leading bytes.
func SniffDocumentFormat(data []byte) (DocumentFormat, bool) {
	for _, f := range documentFormats {
		if f.Sniff(data) {
			return f, true
		}
	}
	return DocumentFormat{}, false
}
