// Package winspool owns the Windows print-spooler boundary and nothing
// else: no UI, no mDNS, no IPP parsing. Higher layers depend on the
// Backend interface and on the plain data types in this package;
// WinSpool/GDI details never leak past the platform_* functions in the
// platform-specific files (anti-corruption layer).
//
// Conversion to the shared core models happens only through
// PrinterDescriptor.ToCore (dependency inversion): this package depends
// on core's direction, never the reverse.
//
// The one exception to "spooler only" is supply levels: the backend finds a
// printer's own address and SNMP settings in the Windows Standard TCP/IP
// port configuration and reads its Printer MIB there (supply_*.go; only the
// port lookup is platform-specific). It is off until the application
// enables it and never queries an address a client supplied.
//
// Platform behavior:
//
//   - On windows, SystemBackend talks to the real spooler
//     (EnumPrintersW / OpenPrinterW / WritePrinter / GetPrinterW via
//     winspool.drv loaded with LazyDLL, no cgo) and carries the
//     banded-GDI spool skeleton ported from the Python StretchDIBits loop.
//   - On any other OS every operation degrades gracefully (empty list,
//     PrinterStatusUnknown, *BackendError with KindUnsupported) so plain
//     go build / go test pass on Linux CI.
package winspool
