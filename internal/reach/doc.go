// Package reach decides whether other devices can reach the bridge's IPP
// port: it parses what Windows says about the active networks and the
// firewall, and classifies it into a verdict the UI can explain. Only the
// collection of the raw facts is Windows-specific; everything else is pure.
//
// The facts are read unelevated through COM interfaces (the Network List
// Manager and HNetCfg.FwPolicy2), because the NetSecurity PowerShell cmdlets
// require administrator rights even for reads. The cost of staying
// unelevated: firewall rules pushed by domain group policy are not visible
// to the COM API, so a corporate block rule can go unnoticed.
package reach
