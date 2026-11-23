package net

import "github.com/OpenPrinting/goipp"

// featureIPPEverywhere is the ipp-features-supported keyword of PWG 5100.14
// (IPP Everywhere): the printer states that it meets that profile.
const featureIPPEverywhere = "ipp-everywhere"

// addIPPFeatures declares IPP Everywhere conformance only for a printer that
// also publishes the supply attributes PWG 5100.14 section 5.1 requires
// (printer-supply and its description and info URI). The bridge learns supply
// levels only for some printers (see addSupplies), so a printer without them
// states nothing: the claim must match what the printer really reports.
func addIPPFeatures(c *printerAttrsContext, attrs *goipp.Attributes) {
	if len(c.supplies) == 0 {
		return
	}
	attrs.Add(keywordAttr("ipp-features-supported", featureIPPEverywhere))
}
