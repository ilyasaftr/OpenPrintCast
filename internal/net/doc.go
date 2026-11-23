// Package net handles network advertisement (mDNS/DNS-SD) and the
// IPP HTTP transport for OpenPrintCast.
//
// Printers selected for sharing are advertised as _ipp._tcp services
// with the _universal subtype iOS browses for, and served over HTTP on
// port 631 at /ipp/print/<id> (see core.ResourcePath). Responses carry
// the AirPrint markers iOS requires and echo the client's IPP version,
// mirroring the Python bridge.
package net
