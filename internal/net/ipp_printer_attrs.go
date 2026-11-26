package net

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// printerAttrsContext carries the printer facts every attribute group draws
// on, so each group builder stays a small pure function of it.
type printerAttrsContext struct {
	printer        core.PrinterInfo
	shared         *SharedState // nil in tests that only describe a printer
	host           string
	port           int
	paused         bool
	available      bool
	state          int
	reasons        []string
	advertisedName string
	media          goipp.Attributes
	orientation    bool
	duplex         bool
	supplies       []core.Supply // publishable supplies; none without data
	supplyHost     string
	serial         string
	firmware       string
	totalPages     int
	pagesKnown     bool
	deviceReasons  []string
	qualities      []int
	defaultQuality int
}

// printerAttrGroup appends one cohesive group of Get-Printer-Attributes
// attributes. Groups run in order; a new group is added by listing it here.
type printerAttrGroup func(c *printerAttrsContext, attrs *goipp.Attributes)

var printerAttrGroups = []printerAttrGroup{
	addPrinterDescription,
	addPrinterStatus,
	addSupplies,
	addIPPFeatures,
	addPrinterIdentity,
	addDeviceFacts,
	addDeviceID,
	addPrintSpeed,
	addProtocolSupport,
	addWebLinks,
	addChangeTracking,
	addGetAttributesSupported,
	addMedia,
	addDocumentFormats,
	addPrinterKind,
	addPrintScaling,
	addPrintRenderingIntent,
	addRasterFormats,
	addJobTemplateDefaults,
	addSides,
	addPrintQuality,
	addOrientation,
	addColorMode,
	addJobCreationSupport,
	addServiceCapabilities,
	addRasterSheetBack,
	addOperations,
}

// printerAttrsResponse answers Get-Printer-Attributes with the bridge
// record plus the AirPrint markers, echoing the client version.
func printerAttrsResponse(req *goipp.Message, printer core.PrinterInfo, shared *SharedState, host string, port int) *goipp.Message {
	_, echo := NegotiateVersion(req.Version)
	rsp := statusResponse(echo, goipp.StatusOk, req.RequestID)
	c := newPrinterAttrsContext(printer, shared, host, port)
	for _, group := range printerAttrGroups {
		group(c, &rsp.Printer)
	}
	InjectMarkers(rsp, printer)
	rsp.Printer = requestedAttributes(req, rsp.Printer, nil)
	return rsp
}

func newPrinterAttrsContext(printer core.PrinterInfo, shared *SharedState, host string, port int) *printerAttrsContext {
	c := &printerAttrsContext{
		printer:   printer,
		shared:    shared,
		host:      host,
		port:      port,
		paused:    shared != nil && shared.IsPaused(),
		available: folderAvailable(printer),
		media:     mediaAttrs(shared, printer),
	}
	c.supplies, c.supplyHost = publishedSupplies(shared, printer)
	c.serial, c.firmware, c.deviceReasons, c.totalPages, c.pagesKnown = deviceFacts(shared, printer)
	c.state, c.reasons = c.stateAndReasons()
	c.advertisedName = c.dnsSDName()
	c.qualities, c.defaultQuality, _ = printQualities(shared, printer)
	c.duplex = duplexSupported(shared, printer)
	if shared != nil {
		provider, ok := capabilitySource(shared, printer).(orientationProvider)
		c.orientation = ok && provider.OrientationSupported(printer.WindowsName)
	}
	return c
}

func (c *printerAttrsContext) stateAndReasons() (int, []string) {
	state := c.jobDerivedState()
	reasons := []string{"none"}
	if reported, reportedReasons, known := c.driverStatus(); known {
		if reported == printerStateStopped || reported == printerStateProcessing && state == printerStateIdle {
			state = reported
		}
		if len(reportedReasons) > 0 {
			reasons = reportedReasons
		}
	}
	if !c.available {
		state = printerStateStopped
		reasons = []string{"other"}
	}
	reasons = mergeSupplyReasons(reasons, core.SupplyStateReasons(c.supplies))
	reasons = mergeSupplyReasons(reasons, c.deviceReasons)
	if c.paused {
		reasons = withPausedReason(reasons)
	}
	return state, reasons
}

// jobDerivedState is stopped while paused, processing while a job runs.
func (c *printerAttrsContext) jobDerivedState() int {
	if c.paused {
		return printerStateStopped
	}
	if c.shared != nil {
		for _, job := range c.shared.ippJobsFor(c.printer.ID) {
			if job.state == jobStateProcessing {
				return printerStateProcessing
			}
		}
	}
	return printerStateIdle
}

func (c *printerAttrsContext) driverStatus() (state int, reasons []string, known bool) {
	if c.shared == nil {
		return 0, nil, false
	}
	provider, ok := capabilitySource(c.shared, c.printer).(statusProvider)
	if !ok {
		return 0, nil, false
	}
	return provider.IPPStatus(c.printer.WindowsName)
}

func withPausedReason(reasons []string) []string {
	if len(reasons) == 1 && reasons[0] == "none" {
		reasons = nil
	}
	if !slices.Contains(reasons, "paused") {
		reasons = append(reasons, "paused")
	}
	return reasons
}

func (c *printerAttrsContext) dnsSDName() string {
	if c.shared == nil {
		return ""
	}
	return c.shared.advertisedInstanceName(c.printer)
}

func (c *printerAttrsContext) queuedJobCount() int {
	queued := 0
	if c.shared == nil {
		return queued
	}
	for _, job := range c.shared.ippJobsFor(c.printer.ID) {
		if !job.isFinished() {
			queued++
		}
	}
	return queued
}

func (c *printerAttrsContext) pagesPerMinute() (int, bool) {
	if c.shared == nil {
		return 0, false
	}
	provider, ok := capabilitySource(c.shared, c.printer).(printSpeedProvider)
	if !ok {
		return 0, false
	}
	return provider.PagesPerMinute(c.printer.WindowsName)
}

func (c *printerAttrsContext) colorPagesPerMinute() (int, bool) {
	if c.shared == nil {
		return 0, false
	}
	provider, ok := capabilitySource(c.shared, c.printer).(colorPrintSpeedProvider)
	if !ok {
		return 0, false
	}
	return provider.ColorPagesPerMinute(c.printer.WindowsName)
}

// configSignature identifies the printer's capabilities; media-ready is
// excluded because it flips without the configuration changing.
func (c *printerAttrsContext) configSignature() string {
	var configMedia goipp.Attributes
	for _, attr := range c.media {
		if attr.Name != "media-ready" && attr.Name != "media-col-ready" {
			configMedia.Add(attr)
		}
	}
	return fmt.Sprintf("%s|%s|%s|%s|%t|%t|%v|%d|%v", c.printer.WindowsName, c.printer.DisplayName(), c.advertisedName,
		c.printer.Location, c.printer.SupportsColor(), c.orientation, c.qualities, c.defaultQuality, configMedia)
}

func addPrinterDescription(c *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(stringsAttr("printer-name", goipp.TagName, c.printer.DisplayName()))
	if c.advertisedName != "" {
		attrs.Add(stringsAttr("printer-dns-sd-name", goipp.TagName, c.advertisedName))
	}
}

func addPrinterStatus(c *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(enumAttr("printer-state", c.state))
	attrs.Add(keywordAttr("printer-state-reasons", c.reasons...))
	attrs.Add(boolAttr("printer-is-accepting-jobs", !c.paused && c.available))
	attrs.Add(intAttr("printer-up-time", printerUptime()))
	attrs.Add(keywordAttr("pdl-override-supported", "not-attempted"))
	attrs.Add(intAttr("queued-job-count", c.queuedJobCount()))
}

func addPrinterIdentity(c *printerAttrsContext, attrs *goipp.Attributes) {
	identityHost := c.host
	if c.shared != nil {
		identityHost = c.shared.printerIdentityHost(c.host)
	}
	attrs.Add(stringsAttr("printer-uuid", goipp.TagURI, "urn:uuid:"+PrinterUUID(c.printer.ID, identityHost)))
	// The bridge has no configured organizational metadata; empty text
	// represents an unspecified organization rather than inventing one.
	attrs.Add(stringsAttr("printer-organization", goipp.TagText, ""))
	attrs.Add(stringsAttr("printer-organizational-unit", goipp.TagText, ""))
	attrs.Add(stringsAttr("printer-uri-supported", goipp.TagURI, printerURI(c.printer, c.host, c.port)))
}

func addPrintSpeed(c *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(boolAttr("color-supported", c.printer.SupportsColor()))
	speed, known := c.pagesPerMinute()
	attrs.Add(pagesPerMinuteAttr("pages-per-minute", speed, known))
	if c.printer.SupportsColor() {
		colorSpeed, colorKnown := c.colorPagesPerMinute()
		attrs.Add(pagesPerMinuteAttr("pages-per-minute-color", colorSpeed, colorKnown))
	}
}

// pagesPerMinuteAttr reports an unknown speed as an unknown value.
func pagesPerMinuteAttr(name string, speed int, known bool) goipp.Attribute {
	if known && speed > 0 && speed <= math.MaxInt32 {
		return intAttr(name, speed)
	}
	return voidAttr(name, goipp.TagUnknown)
}

func addProtocolSupport(c *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(stringsAttr("charset-configured", goipp.TagCharset, charsetUTF8))
	attrs.Add(stringsAttr("charset-supported", goipp.TagCharset, charsetUTF8))
	attrs.Add(stringsAttr("natural-language-configured", goipp.TagLanguage, languageEnglish))
	attrs.Add(stringsAttr("generated-natural-language-supported", goipp.TagLanguage, languageEnglish))
	attrs.Add(keywordAttr("compression-supported", supportedCompressions()...))
	versions := []string{"1.1"}
	if supportsIPP20(c.shared, c.printer) {
		versions = append(versions, "2.0")
	}
	attrs.Add(keywordAttr("ipp-versions-supported", versions...))
	attrs.Add(keywordAttr("uri-authentication-supported", "none"))
	attrs.Add(keywordAttr("uri-security-supported", "none"))
}

// webURI is the bridge's own web page of the printer.
func (c *printerAttrsContext) webURI() string {
	return strings.Replace(printerURI(c.printer, c.host, c.port), "ipp://", "http://", 1)
}

func addWebLinks(c *printerAttrsContext, attrs *goipp.Attributes) {
	webURI := c.webURI()
	attrs.Add(stringsAttr("printer-more-info", goipp.TagURI, webURI))
	attrs.Add(stringsAttr("printer-icons", goipp.TagURI, webURI+"/icon.png"))
}

func addChangeTracking(c *printerAttrsContext, attrs *goipp.Attributes) {
	if c.shared == nil {
		return
	}
	observed := c.shared.observePrinter(c.printer.ID, c.configSignature(), c.state, strings.Join(c.reasons, ","))
	attrs.Add(goipp.MakeAttr("printer-config-change-date-time", goipp.TagDateTime, goipp.Time{Time: observed.configChange}))
	attrs.Add(jobTime("printer-config-change-time", observed.configChange))
	attrs.Add(goipp.MakeAttr("printer-state-change-date-time", goipp.TagDateTime, goipp.Time{Time: observed.stateChange}))
	attrs.Add(jobTime("printer-state-change-time", observed.stateChange))
}

func addGetAttributesSupported(_ *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(keywordAttr("printer-get-attributes-supported", "document-format"))
}
