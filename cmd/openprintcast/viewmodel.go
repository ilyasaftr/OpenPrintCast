package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// level is the severity a status carries. The UI always pairs its color
// with a text label (WCAG 1.4.1: color is never the only signal).
type level int

const (
	levelOK level = iota
	levelWarn
	levelError
	levelIdle
)

// maxDNSLabelBytes is the DNS-SD limit on a service instance name.
const maxDNSLabelBytes = 63

// plural returns "1 printer" / "2 printers".
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// summarize is the one-line system status shown in the header pill and
// the tray tooltip: what the bridge is doing right now.
func summarize(paused bool, shared, total int, online bool) (string, level) {
	switch {
	case paused:
		return "Paused", levelWarn
	case !online:
		return "No network", levelError
	case total == 0:
		return "No printers found", levelWarn
	case shared == 0:
		return "Nothing shared", levelWarn
	default:
		return fmt.Sprintf("Sharing %d of %d", shared, total), levelOK
	}
}

// bannerMessage reserves banners for conditions needing action outside the
// printer list. "Nothing shared" is already shown in the header.
func bannerMessage(paused, online bool) (string, level) {
	switch {
	case paused:
		return "Sharing is paused. Print jobs from iPhone, iPad, Mac and Android are rejected until you resume.", levelWarn
	case !online:
		return "No network connection found. Connect this PC to Wi-Fi or Ethernet so your devices can find it.", levelError
	}
	return "", levelOK
}

// connectionLine tells the user how devices see this PC.
func connectionLine(host, ip string, port int) string {
	if ip == "" {
		return "Not connected to a network."
	}
	return fmt.Sprintf("Visible on your network as %s (%s:%d). On iPhone, iPad, Mac or Android, choose Print, then pick the printer.", host, ip, port)
}

func matchesPrinter(p core.PrinterInfo, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return query == "" || strings.Contains(strings.ToLower(p.WindowsName), query) ||
		strings.Contains(strings.ToLower(p.DisplayName()), query) ||
		strings.Contains(strings.ToLower(p.Location), query)
}

// jobStatus maps a job state to its label and severity.
func jobStatus(s core.JobStatus) (string, level) {
	switch s {
	case core.JobCompleted:
		return "Completed", levelOK
	case core.JobFailed:
		return "Failed", levelError
	case core.JobCancelled:
		return "Cancelled", levelIdle
	case core.JobPrinting:
		return "Printing", levelWarn
	default:
		return "Pending", levelWarn
	}
}

// jobInFlight reports whether the job is still waiting or printing, which
// is when it can be cancelled.
func jobInFlight(s core.JobStatus) bool {
	return s == core.JobPending || s == core.JobPrinting
}

// jobDetails shows a job without a failure or cancel reason as a dash.
func jobDetails(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "—"
	}
	return reason
}

// newestFirst returns a reversed copy: logs read best newest on top.
func newestFirst(jobs []core.JobEntry) []core.JobEntry {
	out := make([]core.JobEntry, len(jobs))
	for i, j := range jobs {
		out[len(jobs)-1-i] = j
	}
	return out
}

// hasFinishedJobs reports whether "Clear finished" would remove anything.
func hasFinishedJobs(jobs []core.JobEntry) bool {
	for _, j := range jobs {
		if j.Status.Finished() {
			return true
		}
	}
	return false
}

// jobPages renders an unknown page count as a dash, not a misleading 0.
func jobPages(n int) string {
	if n <= 0 {
		return "—"
	}
	return strconv.Itoa(n)
}

// jobDocument falls back to a dash for jobs submitted without a name.
func jobDocument(name string) string {
	if strings.TrimSpace(name) == "" {
		return "—"
	}
	return name
}

// validateAirPrintName flags names too long to advertise over DNS-SD. The
// limit counts UTF-8 bytes (the DNS label's octets), so the message says
// bytes and why a short name written in accents or another script can hit it.
func validateAirPrintName(name string) string {
	if len(name) > maxDNSLabelBytes {
		return fmt.Sprintf("Name is too long. It can be up to %d bytes; accented and non-Latin characters use more than one.", maxDNSLabelBytes)
	}
	return ""
}
