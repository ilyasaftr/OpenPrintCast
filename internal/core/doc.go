// Package core holds the portable domain of OpenPrintCast: printer
// and job models, configuration, AirPrint TXT records, the bounded job
// log, and raster format helpers. It has no OS, network, or UI imports so
// every other package can depend on it, including in tests on any platform.
package core
