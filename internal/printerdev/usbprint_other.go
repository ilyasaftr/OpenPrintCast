//go:build !windows

package printerdev

// openPrinterDevice on non-Windows systems finds nothing: the transport
// is the Windows USB printing driver.
func openPrinterDevice(printer string) (deviceHandle, error) {
	return deviceHandle{}, errNoProbeSource
}
