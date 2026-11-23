package net

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func validateIPPTarget(req *goipp.Message, printer core.PrinterInfo) goipp.Status {
	target := opAttrString(req.Operation, "printer-uri")
	jobTarget := target == ""
	if jobTarget {
		target = opAttrString(req.Operation, "job-uri")
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "ipp" || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return goipp.StatusErrorBadRequest
	}
	path := "/" + core.ResourcePath(printer.ID)
	if jobTarget {
		path += "/job/" + strconv.FormatUint(uint64(requestedJobID(req)), 10)
	}
	if u.Path != path {
		return goipp.StatusErrorNotFound
	}
	return goipp.StatusOk
}

func validateIPPRequest(req *goipp.Message) goipp.Status {
	if req.RequestID == 0 || req.RequestID > maxRequestID {
		return goipp.StatusErrorBadRequest
	}
	if req.Version.Major() != 1 && req.Version.Major() != 2 {
		return goipp.StatusErrorVersionNotSupported
	}
	if status := validateOperationPreamble(req); status != goipp.StatusOk {
		return status
	}
	return validateTargetAttributes(req)
}

// validateOperationPreamble checks the mandatory leading charset and
// natural-language operation attributes.
func validateOperationPreamble(req *goipp.Message) goipp.Status {
	if len(req.Operation) < 3 {
		return goipp.StatusErrorBadRequest
	}
	charset, lang := req.Operation[0], req.Operation[1]
	if charset.Name != "attributes-charset" || !hasSingleValue(charset, goipp.TagCharset) ||
		lang.Name != "attributes-natural-language" || !hasSingleValue(lang, goipp.TagLanguage) {
		return goipp.StatusErrorBadRequest
	}
	if !strings.EqualFold(charset.Values[0].V.String(), charsetUTF8) {
		return goipp.StatusErrorCharset
	}
	return goipp.StatusOk
}

// validateTargetAttributes checks that the printer-uri / job-uri / job-id
// combination is the one the operation requires.
func validateTargetAttributes(req *goipp.Message) goipp.Status {
	printer := hasTargetAttr(req, "printer-uri", goipp.TagURI)
	jobURI := hasTargetAttr(req, "job-uri", goipp.TagURI)
	jobID := hasTargetAttr(req, "job-id", goipp.TagInteger) && requestedJobID(req) > 0 && requestedJobID(req) <= maxRequestID
	switch goipp.Op(req.Code) {
	case goipp.OpSendDocument, goipp.OpCancelJob, goipp.OpGetJobAttributes:
		if !(jobURI && !printer || printer && jobID && !jobURI) {
			return goipp.StatusErrorBadRequest
		}
	case goipp.OpCloseJob:
		if !printer || !jobID || jobURI {
			return goipp.StatusErrorBadRequest
		}
	default:
		if !printer {
			return goipp.StatusErrorBadRequest
		}
	}
	if printer && jobURI {
		return goipp.StatusErrorBadRequest
	}
	return goipp.StatusOk
}

// hasTargetAttr reports whether the operation group holds exactly one
// non-empty value of the given tag under name.
func hasTargetAttr(req *goipp.Message, name string, tag goipp.Tag) bool {
	found := false
	for _, attr := range req.Operation {
		if attr.Name != name {
			continue
		}
		if found || !hasSingleValue(attr, tag) || attr.Values[0].V.String() == "" {
			return false
		}
		found = true
	}
	return found
}
