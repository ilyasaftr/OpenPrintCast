// Package printerdev reads printer facts straight from the device, without
// the network: ink and waste-ink levels, the serial number, the firmware
// version and error states, over the USB cable the printer is plugged in
// with.
//
// The package is a chain of vendor probes over one transport. The
// transport is the Windows USB printing device interface (usbprint.sys),
// which every USB printer already publishes — no driver is replaced and no
// rights beyond the user's are needed. Each vendor probe recognizes its
// devices by the IEEE 1284 device ID and speaks the vendor's protocol;
// Epson is the first, over IEEE 1284.4 (D4) and the EPSON-CTRL service.
//
// The protocol code is portable and tested against recorded frames; only
// the device access is Windows-only, behind build tags, so the package
// compiles everywhere.
package printerdev
