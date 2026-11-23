package net

import "github.com/OpenPrinting/goipp"

const identifyActionSound = "sound"

func handleGetPrinterAttributes(c *ippRequest) *goipp.Message {
	if decision := c.judgeDeclaredFormat(); !decision.accepted() {
		return c.respond(decision.status)
	}
	return printerAttrsResponse(c.req, c.printer, c.shared, c.host, c.port)
}

func handleIdentifyPrinter(c *ippRequest) *goipp.Message {
	identifier, supported := capabilitySource(c.shared, c.printer).(identifyProvider)
	status := identifyRequestStatus(c.req, supported)
	if status == goipp.StatusOk {
		status = c.identify(identifier)
	}
	return c.respond(status)
}

// identifyRequestStatus validates the identify-actions of the request.
func identifyRequestStatus(req *goipp.Message, supported bool) goipp.Status {
	status := goipp.StatusOk
	if !supported {
		status = goipp.StatusErrorOperationNotSupported
	}
	for _, attr := range req.Operation {
		if attr.Name != "identify-actions" {
			continue
		}
		for _, value := range attr.Values {
			if value.T != goipp.TagKeyword || value.V.String() != identifyActionSound {
				status = goipp.StatusErrorAttributesOrValues
			}
		}
	}
	return status
}

func (c *ippRequest) identify(identifier identifyProvider) goipp.Status {
	if !c.shared.allowIdentify() {
		return goipp.StatusErrorBusy
	}
	if err := identifier.IdentifyPrinter(c.printer.WindowsName); err != nil {
		componentLogger(componentIPP).Warn("printer identification failed", "printer", c.printer.ID, "err", err)
		return goipp.StatusErrorInternal
	}
	return goipp.StatusOk
}
