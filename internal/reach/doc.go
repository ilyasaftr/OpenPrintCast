// Package reach decides whether other devices can reach the bridge's IPP
// port: it parses what Windows says about the active networks and the
// firewall, and classifies it into a verdict the UI can explain. Only the
// collection of the raw facts is Windows-specific; everything else is pure.
package reach
