// Package zeroconf is a pure Golang library that employs Multicast DNS-SD for
// browsing and resolving services in your network and registering own services
// in the local network.
//
// It basically implements aspects of the standards
// RFC 6762 (mDNS) and
// RFC 6763 (DNS-SD).
// Though it does not support all requirements yet, the aim is to provide a
// complient solution in the long-term with the community.
//
// By now, it should be compatible to [Avahi](http://avahi.org/) (tested) and
// Apple's Bonjour (untested). Should work in the most office, home and private
// environments.
//
// This is the bridge's own copy of the library, derived from
// github.com/grandcat/zeroconf through github.com/enbility/zeroconf/v2 at
// commit be1cae74fda6 (MIT licensed).
// The bridge changes are the Hostname server option, the OnConflict and
// OnHostConflict callbacks, and the conflict detection and probe tie-break
// behind them, all in server.go.
package zeroconf
