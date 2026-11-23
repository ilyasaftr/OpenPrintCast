package net

import (
	"encoding/hex"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const mimeOctetStream = core.MIMEOctetStream

// supportedDocumentFormats is advertised as document-format-supported: every
// registered format plus the auto-sense type (RFC 8011 section 5.1.10.1),
// which the sniffing below honors.
func supportedDocumentFormats() []string {
	return append(core.AdvertisedMIMEs(), mimeOctetStream)
}

// documentRule names the rule that decided a document, so a refusal can be
// traced in the log to the exact check that made it.
type documentRule string

const (
	// Accepting rules.
	ruleDeclaredMatchesData documentRule = "declared type matches the data"
	ruleAutoSensed          documentRule = "no declared type, data recognized"
	ruleTrustedData         documentRule = "declared type differs from the data, data trusted"
	// Rejecting rules.
	ruleUnsupportedDeclared documentRule = "declared type is not a supported format"
	ruleUnrecognizedData    documentRule = "no declared type and the data is not a supported format"
	ruleDataNotDeclared     documentRule = "data is not in the declared format"
)

// documentDecision is the outcome of judging a document: the MIME type the
// spooler receives, or the IPP status that refuses it.
type documentDecision struct {
	mime   string
	status goipp.Status
	rule   documentRule
}

func (d documentDecision) accepted() bool { return d.status == goipp.StatusOk }

// autoSense reports whether the declared type leaves the format to the
// printer: absent, or application/octet-stream (RFC 8011 section 5.1.10.1).
func autoSense(declared string) bool {
	name := core.NormalizeMIME(declared)
	return name == "" || name == mimeOctetStream
}

// decideDeclared judges a declared document-format alone, as Validate-Job and
// Create-Job (which carry no data) must: a type outside the registry is
// unsupported (RFC 8011 section 4.2.1.1); the rest may proceed.
func decideDeclared(declared string) documentDecision {
	if _, known := core.LookupDocumentFormat(declared); !known && !autoSense(declared) {
		return documentDecision{status: goipp.StatusErrorDocumentFormatNotSupported, rule: ruleUnsupportedDeclared}
	}
	return documentDecision{status: goipp.StatusOk, rule: ruleDeclaredMatchesData}
}

// decideDocument judges a document against its declared format. The data is
// sniffed first and decides what is printed: RFC 8011 asks printers to trust
// a declared type over auto-sensing, but that rule exists for ambiguous text
// formats; the registered formats all have unambiguous signatures, so when a
// client mislabels one registered format as another the document is still
// printed as what it is. A declared type outside the registry is refused,
// and so is data that no registered format recognizes.
func decideDocument(doc []byte, declared string) documentDecision {
	if d := decideDeclared(declared); !d.accepted() {
		return d
	}
	sniffed, recognized := core.SniffDocumentFormat(doc)
	if !recognized {
		if autoSense(declared) {
			return documentDecision{status: goipp.StatusErrorDocumentFormatNotSupported, rule: ruleUnrecognizedData}
		}
		return documentDecision{status: goipp.StatusErrorDocumentFormatError, rule: ruleDataNotDeclared}
	}
	switch named, _ := core.LookupDocumentFormat(declared); {
	case autoSense(declared):
		return documentDecision{mime: sniffed.MIME, rule: ruleAutoSensed}
	case named.MIME == sniffed.MIME:
		return documentDecision{mime: sniffed.MIME, rule: ruleDeclaredMatchesData}
	default:
		return documentDecision{mime: sniffed.MIME, rule: ruleTrustedData}
	}
}

// rejectedMagicBytes is how much of a rejected document's start is logged: a
// file signature, never the content.
const rejectedMagicBytes = 12

// logDocumentDecision records a refusal, and a document printed as something
// other than its declared type: what the client declared, what the data
// looks like and which rule decided, so a client that sends a format the
// bridge does not know can be told apart from one whose declared format does
// not match its data.
func logDocumentDecision(op, printerID, declared string, doc []byte, d documentDecision) {
	if d.accepted() && d.rule != ruleTrustedData {
		return
	}
	magic := doc
	if len(magic) > rejectedMagicBytes {
		magic = magic[:rejectedMagicBytes]
	}
	message, log := "document format rejected", componentLogger(componentIPP).Warn
	if d.accepted() {
		message, log = "document format differs from its declaration", componentLogger(componentIPP).Info
	}
	detected := mimeOctetStream
	if sniffed, ok := core.SniffDocumentFormat(doc); ok {
		detected = sniffed.MIME
	}
	log(message, "op", op, "printer", printerID, "declared", declared, "detected", detected,
		"rule", string(d.rule), "status", d.status.String(), "bytes", len(doc), "magic", hex.EncodeToString(magic))
}
