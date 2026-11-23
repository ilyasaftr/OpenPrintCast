package net

import (
	"fmt"
	stdnet "net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

var (
	ipp11 = goipp.MakeVersion(1, 1)
	ipp20 = goipp.MakeVersion(2, 0)
)

// maxDocumentBytes caps a single Print-Job upload.
const maxDocumentBytes = 256 << 20

// maxRequestID is the largest request-id RFC 8011 allows.
const maxRequestID = 0x7fffffff

const (
	unknownUser     = "unknown"
	charsetUTF8     = "utf-8"
	languageEnglish = "en"
	compressionNone = "none"
	valueAuto       = "auto"
)

// IPP job-state enum values (RFC 8011 section 5.3.7).
const (
	jobStatePending    = 3
	jobStateProcessing = 5
	jobStateCanceled   = 7
	jobStateAborted    = 8
	jobStateCompleted  = 9
)

// IPP printer-state enum values (RFC 8011 section 5.4.11).
const (
	printerStateIdle       = 3
	printerStateProcessing = 4
	printerStateStopped    = 5
)

// nextJobID numbers IPP job-id responses; bridge JobEntry IDs come from
// core.NewJobEntry instead.
var nextJobID atomic.Uint32

var startedAt = time.Now()

func printerUptime() int {
	return int(time.Since(startedAt).Seconds()) + 1
}

// opAttrString reads the first value of a named operation attribute.
func opAttrString(attrs goipp.Attributes, name string) string {
	for _, a := range attrs {
		if a.Name == name && len(a.Values) > 0 {
			return a.Values[0].V.String()
		}
	}
	return ""
}

// okOperation is the shared charset/language operation group.
func okOperation(rsp *goipp.Message) {
	rsp.Operation.Add(stringsAttr("attributes-charset", goipp.TagCharset, charsetUTF8))
	rsp.Operation.Add(stringsAttr("attributes-natural-language", goipp.TagLanguage, languageEnglish))
}

// statusResponse builds a response that carries only a status code.
func statusResponse(version goipp.Version, status goipp.Status, requestID uint32) *goipp.Message {
	rsp := goipp.NewResponse(version, status, requestID)
	okOperation(rsp)
	return rsp
}

// requestUser is the requesting-user-name of req, or unknownUser.
func requestUser(req *goipp.Message) string {
	if user := opAttrString(req.Operation, "requesting-user-name"); user != "" {
		return user
	}
	return unknownUser
}

// printerURI is the ipp:// URI clients address for printer.
func printerURI(printer core.PrinterInfo, host string, port int) string {
	if stdnet.ParseIP(host) == nil && !strings.Contains(host, ".") {
		host += ".local"
	}
	return fmt.Sprintf("ipp://%s/%s", stdnet.JoinHostPort(host, strconv.Itoa(port)), core.ResourcePath(printer.ID))
}

func folderAvailable(printer core.PrinterInfo) bool {
	if printer.FolderPath == "" {
		return true
	}
	info, err := os.Stat(printer.FolderPath)
	return err == nil && info.IsDir()
}

// attributeGroupNames are requested-attributes values that select a whole
// group, which the bridge answers with every attribute it has.
var attributeGroupNames = map[string]bool{
	"all":                 true,
	"job-template":        true,
	"printer-description": true,
	"printer-status":      true,
	"job-description":     true,
}

func requestedAttributes(req *goipp.Message, attrs, defaults goipp.Attributes) goipp.Attributes {
	for _, attr := range req.Operation {
		if attr.Name == "requested-attributes" {
			return filterRequested(attr, attrs)
		}
	}
	if defaults != nil {
		return defaults
	}
	return attrs
}

func filterRequested(requested goipp.Attribute, attrs goipp.Attributes) goipp.Attributes {
	names := make(map[string]bool, len(requested.Values))
	for _, value := range requested.Values {
		name := value.V.String()
		if attributeGroupNames[name] {
			return attrs
		}
		names[name] = true
	}
	var filtered goipp.Attributes
	for _, a := range attrs {
		if names[a.Name] || names["printer-defaults"] && strings.HasSuffix(a.Name, "-default") {
			filtered.Add(a)
		}
	}
	return filtered
}
