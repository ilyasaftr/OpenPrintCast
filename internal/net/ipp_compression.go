package net

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"

	"github.com/OpenPrinting/goipp"
)

// Compression keywords of the RFC 8011 "compression" operation attribute.
// The 'compress' keyword (LZW) is deliberately unsupported: no client sends
// it and the standard library cannot decode it.
const (
	compressionDeflate = "deflate"
	compressionGzip    = "gzip"
)

// decompressors maps each supported compression keyword to its decoder. RFC
// 8011 section 5.4.32 defines 'deflate' as RFC 1951 (raw DEFLATE, which is
// what the CUPS ipp backend and ipptool send) and 'gzip' as RFC 1952.
var decompressors = map[string]func(doc []byte) (io.ReadCloser, error){
	compressionGzip:    func(doc []byte) (io.ReadCloser, error) { return gzip.NewReader(bytes.NewReader(doc)) },
	compressionDeflate: newDeflateReader,
}

// zlibHeaderMultiple is the RFC 1950 check: CMF*256+FLG is a multiple of 31.
const (
	zlibHeaderMultiple = 31
	zlibMethodDeflate  = 8
)

// newDeflateReader decodes raw DEFLATE, and also a zlib stream (RFC 1950),
// the HTTP "deflate" coding some clients confuse it with. A zlib header is
// told from raw data by its method nibble and checksum bits.
func newDeflateReader(doc []byte) (io.ReadCloser, error) {
	if len(doc) >= 2 && doc[0]&0x0F == zlibMethodDeflate && (int(doc[0])<<8|int(doc[1]))%zlibHeaderMultiple == 0 {
		return zlib.NewReader(bytes.NewReader(doc))
	}
	return flate.NewReader(bytes.NewReader(doc)), nil
}

// supportedCompressions is advertised as compression-supported; clients such
// as the CUPS ipp backend pick the first of gzip and deflate they find and
// then compress raster documents.
func supportedCompressions() []string {
	return []string{compressionNone, compressionDeflate, compressionGzip}
}

// compressionSupported reports whether the keyword may be sent.
func compressionSupported(keyword string) bool {
	if keyword == compressionNone {
		return true
	}
	_, ok := decompressors[keyword]
	return ok
}

// inflateDocument expands doc compressed with keyword, producing at most
// limit bytes. It stops reading as soon as the limit is exceeded, so a
// decompression bomb never allocates more than limit+1 bytes. The status is
// client-error-compression-error for a damaged or truncated stream (the
// trailing checksum is verified) and client-error-request-entity-too-large
// when the document would exceed limit.
func inflateDocument(doc []byte, keyword string, limit int) ([]byte, goipp.Status) {
	newReader, ok := decompressors[keyword]
	if !ok || len(doc) == 0 {
		return doc, goipp.StatusOk // no compression, or no document at all (Validate-Job, a bare last Send-Document)
	}
	reader, err := newReader(doc)
	if err != nil {
		return nil, goipp.StatusErrorCompressionError
	}
	defer reader.Close()
	out, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	switch {
	case err != nil:
		return nil, goipp.StatusErrorCompressionError
	case len(out) > limit:
		return nil, goipp.StatusErrorRequestEntity
	}
	return out, goipp.StatusOk
}
