package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// jobsRefreshInterval matches the Rust shell's 2 s Jobs poll: the Jobs
// page re-reads the shared JobLog on every frame it schedules.
const jobsRefreshInterval = 2 * time.Second

// clearFinishedTip explains what Clear Finished removes.
const clearFinishedTip = "Remove completed, failed and cancelled jobs from this list"

// Jobs table titles double as column keys for the cell renderer.
const (
	colReceived = "Received"
	colDocument = "Document"
	colUser     = "User"
	colPrinter  = "Printer"
	colPages    = "Pages"
	colStatus   = "Status"
	colDetails  = "Details"
)

// Jobs table metrics: from wideJobsWidth the table also shows User and
// Pages and gives Printer more room.
const (
	wideJobsWidth         = 840 // the wide columns leave Document about 100 wide here
	jobsRowHeight         = 36
	jobReceivedWidth      = 172 // fits "Yesterday, 12:38 AM" and its longer German and Indonesian forms
	jobReceivedNarrow     = 152 // fits the widest narrow-layout date, "2025年10月22日", with room for wider CJK fallback fonts
	jobUserWidth          = 80
	jobPrinterWidth       = 190
	jobPrinterWideWidth   = 150
	jobPagesWidth         = 72
	jobStatusWidth        = 148
	jobDetailsWidth       = 88
	jobStatusIconSize     = 16
	clearFinishedIconSize = 16
)

// statusColor is the tint of a job's status symbol; the label next to it
// always carries the meaning, so color is never the only signal.
func statusColor(t *ui.Theme, s core.JobStatus) ui.Color {
	if s == core.JobPrinting {
		return accentForeground(t)
	}
	_, lvl := jobStatus(s)
	return levelColor(t, lvl)
}

// viewJobs shows the shared job history newest first in a virtualized
// table inside a rounded container, re-reading it every frame the After
// timer schedules. Status pairs a symbol with its label.
func (a *bridgeApp) viewJobs(c *ui.Context) {
	c.After(jobsRefreshInterval)
	jobs := newestFirst(a.shared.Jobs.List())
	a.jobsToolbar(c, jobs)
	if len(jobs) == 0 {
		a.jobsEmptyState(c)
		return
	}
	a.jobsTable(c, jobs)
}

// jobsToolbar holds the page caption and the Clear Finished action.
func (a *bridgeApp) jobsToolbar(c *ui.Context, jobs []core.JobEntry) {
	ui.Row(c).Gap(controlGap).Children(func() {
		ui.Text(c, "Latest jobs from your devices, newest first.").TextColor(c.Theme().TextMuted).Grow(1).MinWidth(0).SingleLine()
		if ui.Button(c, "").Label("Clear Finished").Tooltip(clearFinishedTip).Disabled(!hasFinishedJobs(jobs)).Children(func() {
			ui.Icon(c, trashIcon).FontSize(clearFinishedIconSize)
			ui.Text(c, "Clear Finished").SingleLine()
		}).Clicked() {
			a.clearFinishedWithUndo(c)
		}
	})
}

// jobsEmptyState explains the empty history and offers a way back to the
// printers page.
func (a *bridgeApp) jobsEmptyState(c *ui.Context) {
	emptyState(c, printerIcon, "No Print Jobs",
		"When someone prints to a shared printer from an iPhone, iPad, Mac or Android device, the job shows up here.", func() {
			if ui.Button(c, "Show Printers").Clicked() {
				a.tab = tabPrinters
			}
		})
}

// jobsTable renders the job history inside a rounded, clipped container.
func (a *bridgeApp) jobsTable(c *ui.Context, jobs []core.JobEntry) {
	t := c.Theme()
	width, _ := c.Size()
	cols := jobColumns(width)
	compact := width < wideJobsWidth
	times := jobTimes{now: a.currentTime(), dates: newCLDRDates(c.Locale(), c.Region()), layouts: fullTimeLayouts}
	a.jobsList.RowHeight = jobsRowHeight
	if compact {
		a.jobsList.RowHeight = 0 // rows with a reason under their status are taller
		times.layouts = compactTimeLayouts
	}
	ui.Column(c).Grow(1).Radius(groupRadius).Border(hairline, t.Border).Clip().Children(func() {
		ui.Table(c, &a.jobsList, cols, len(jobs), func(row, col int) {
			a.jobCell(c, jobs[row], cols[col].Title, compact, times)
		}).Grow(1)
	})
}

// currentTime is the moment the Jobs table dates its rows against.
func (a *bridgeApp) currentTime() time.Time {
	if a.clock != nil {
		return a.clock()
	}
	return time.Now()
}

// jobColumns is the column set for the given window width: the narrow
// layout drops User and Pages so Status always stays visible.
func jobColumns(width float32) []ui.TableColumn {
	if width < wideJobsWidth {
		return []ui.TableColumn{
			{Title: colReceived, Width: jobReceivedNarrow}, {Title: colDocument},
			{Title: colPrinter, Width: jobPrinterWidth}, {Title: colStatus, Width: jobStatusWidth},
		}
	}
	return []ui.TableColumn{
		{Title: colReceived, Width: jobReceivedWidth}, {Title: colDocument}, {Title: colUser, Width: jobUserWidth},
		{Title: colPrinter, Width: jobPrinterWideWidth}, {Title: colPages, Width: jobPagesWidth, Align: ui.End},
		{Title: colStatus, Width: jobStatusWidth}, {Title: colDetails, Width: jobDetailsWidth},
	}
}

// jobCell renders one cell of the jobs table for the named column; compact
// is the narrow layout, which has no Details column.
func (a *bridgeApp) jobCell(c *ui.Context, job core.JobEntry, column string, compact bool, times jobTimes) {
	switch column {
	case colReceived:
		fullTextOf(ui.Text(c, times.label(job.CreatedAt)), times.detail(job.CreatedAt))
	case colDocument:
		fullText(c, jobDocument(job.Filename))
	case colUser:
		fullText(c, job.User)
	case colPrinter:
		fullText(c, printerLabel(a.printers, job.PrinterID))
	case colPages:
		ui.Text(c, jobPages(job.Pages)).SingleLine()
	case colStatus:
		a.jobStatusCell(c, job, compact)
	case colDetails:
		jobDetailsCell(c, job.Reason)
	}
}

// fullText is a single-line cell whose tooltip shows the whole text, for
// when the column is too narrow to.
func fullText(c *ui.Context, text string) {
	fullTextOf(ui.Text(c, text), text)
}

// fullTextOf makes text single-line with the whole of it as its tooltip.
func fullTextOf(text ui.Element, whole string) {
	text.SingleLine().Tooltip(whole)
}

// showsReasonInline reports whether a job's reason goes under its status:
// in the narrow layout, which has no Details column to hold it, a failed
// job's reason would otherwise show only as a tooltip.
func showsReasonInline(compact bool, job core.JobEntry) bool {
	return compact && job.Status == core.JobFailed && strings.TrimSpace(job.Reason) != ""
}

// jobStatusCell pairs the status symbol with its label, and offers Cancel
// while the job is still in flight. A reason shown inline sits under them.
func (a *bridgeApp) jobStatusCell(c *ui.Context, job core.JobEntry, compact bool) {
	inline := showsReasonInline(compact, job)
	ui.Column(c).Gap(captionTextGap).Children(func() {
		ui.Row(c).Gap(statusCellGap).Children(func() {
			jobStatusLabel(c, job, !inline)
			if jobInFlight(job.Status) && iconButton(c, closeIcon, "Cancel job "+jobDocument(job.Filename), "Cancel this job") {
				c.Toast(a.cancelJob(job))
			}
		})
		if inline {
			fullTextOf(captionText(c, job.Reason, c.Theme().TextMuted), job.Reason)
		}
	})
}

// jobStatusLabel is the status symbol and label; the job's reason, when it
// has one and tip is set, is the tooltip (the label names the element for
// assistive technology, which reads the tooltip as its description). tip is
// off when the reason is already shown as text beside it.
func jobStatusLabel(c *ui.Context, job core.JobEntry, tip bool) {
	label, _ := jobStatus(job.Status)
	row := ui.Row(c).Gap(statusCellGap).Shrink(0).Children(func() {
		ui.Icon(c, jobStatusIcon(job.Status)).FontSize(jobStatusIconSize).TextColor(statusColor(c.Theme(), job.Status))
		ui.Text(c, label).SingleLine()
	})
	if tip && job.Reason != "" {
		row.Label(label).Tooltip(job.Reason)
	}
}

// jobDetailsCell is the muted failure or cancel reason, truncated to the
// column with the full text as its tooltip.
func jobDetailsCell(c *ui.Context, reason string) {
	text := ui.Text(c, jobDetails(reason)).TextColor(c.Theme().TextMuted).SingleLine()
	if reason != "" {
		text.Tooltip(reason)
	}
}

// printerLabel resolves a job's printer id to its display name, falling
// back to the raw id for printers that have since disappeared.
func printerLabel(printers []core.PrinterInfo, id string) string {
	if id == folderPrinterID {
		return "Print to Folder"
	}
	for _, p := range printers {
		if p.ID == id {
			return p.DisplayName()
		}
	}
	return id
}
