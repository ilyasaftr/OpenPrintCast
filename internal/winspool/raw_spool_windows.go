//go:build windows

package winspool

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// docInfo1W mirrors the DOC_INFO_1W struct: three LPWSTR pointers.
type docInfo1W struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
}

// platformSpool renders supported documents and sends non-raster data
// through the legacy RAW WritePrinter path.
func platformSpool(printer string, data []byte, mime string, settings RenderSettings) error {
	base := canonicalMIME(mime)
	if needsRasterPath(base) {
		return spoolRendered(printer, data, base, settings)
	}
	return spoolRaw(printer, data, mime)
}

// spoolRaw sends data to the printer unchanged as a RAW job.
func spoolRaw(printer string, data []byte, mime string) error {
	handle, err := openPrinter(printer)
	if err != nil {
		return err
	}
	defer closePrinter(handle)

	if err := startRawDocument(handle, printer, mime); err != nil {
		return err
	}
	result := writeRawChunks(handle, printer, data)
	// EndDocPrinter must run whenever a job was started, even if a chunk
	// write failed.
	if r1, _, errno := procEndDocPrinter.Call(handle); r1 == 0 && result == nil {
		result = PlatformError("EndDocPrinter",
			fmt.Sprintf("printer '%s' failed to finish the job: %s", printer, errno.Error()))
	}
	return result
}

// startRawDocument begins a RAW print job on handle.
func startRawDocument(handle uintptr, printer, mime string) error {
	titlePtr, err := windows.UTF16PtrFromString(fmt.Sprintf("AirPrint job (%s)", mime))
	if err != nil {
		return SpoolError(printer, "cannot encode job title")
	}
	rawPtr, err := windows.UTF16PtrFromString("RAW")
	if err != nil {
		return SpoolError(printer, "cannot encode RAW datatype")
	}
	docInfo := docInfo1W{pDocName: titlePtr, pOutputFile: nil, pDatatype: rawPtr}
	// StartDocPrinterW returns the job id, 0 on failure.
	jobID, _, errno := procStartDocPrinterW.Call(
		handle,
		1,
		uintptr(unsafe.Pointer(&docInfo)),
	)
	if jobID == 0 {
		return PlatformError("StartDocPrinterW",
			fmt.Sprintf("printer '%s' rejected the job: %s", printer, errno.Error()))
	}
	return nil
}

// writeRawChunks writes data in RawChunkSize pieces, stopping at the
// first short or failed write.
func writeRawChunks(handle uintptr, printer string, data []byte) error {
	for offset := 0; offset < len(data); {
		end := offset + RawChunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]
		var written uint32
		r1, _, errno := procWritePrinter.Call(
			handle,
			uintptr(unsafe.Pointer(&chunk[0])),
			uintptr(len(chunk)),
			uintptr(unsafe.Pointer(&written)),
		)
		if r1 == 0 || int(written) != len(chunk) {
			return SpoolError(printer,
				fmt.Sprintf("WritePrinter short/failed write: %s", errno.Error()))
		}
		offset = end
	}
	return nil
}
