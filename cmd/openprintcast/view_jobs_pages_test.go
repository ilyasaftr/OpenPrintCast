package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// pagesColumnTolerance absorbs sub-pixel text layout differences when
// comparing the right edges of right-aligned cells.
const pagesColumnTolerance = 1.5

// newPagesApp builds a Jobs page whose finished jobs show 1, 12 and 123
// pages, plus one whose count is not known.
func newPagesApp(t *testing.T) *bridgeApp {
	t.Helper()
	app := newTestApp(t)
	jobs := &core.JobLog{}
	for _, job := range []struct {
		file  string
		pages int
	}{{"single.pdf", 1}, {"twelve.pdf", 12}, {"long-report.pdf", 123}, {"unknown.pdf", 0}} {
		entry := core.NewJobEntry("printer-epson", "Agung", job.file, job.pages)
		entry.SetStatus(core.JobCompleted)
		jobs.Push(entry)
	}
	app.shared = bridgenet.NewSharedState(app.printers, jobs)
	app.tab = tabJobs
	return app
}

func TestJobsPagesColumnShowsCountsRightAligned(t *testing.T) {
	tt := ui.NewTester(newPagesApp(t).view, 860, 620)
	header, ok := tt.Find(colPages)
	if !ok {
		t.Fatalf("no %q column at 860 wide; texts: %q", colPages, tt.Texts())
	}
	var right []float32
	for _, count := range []string{"1", "12", "123"} {
		rect, ok := tt.Find(count)
		if !ok {
			t.Fatalf("page count %q is not shown; texts: %q", count, tt.Texts())
		}
		if rect.X < header.X-jobPagesWidth || rect.X+rect.W > 860 {
			t.Errorf("%q at %v is outside the Pages column or the window", count, rect)
		}
		if rect.W > jobPagesWidth/2 {
			t.Errorf("%q is %v wide, leaving too little room in the %v wide column", count, rect.W, jobPagesWidth)
		}
		right = append(right, rect.X+rect.W)
	}
	for _, edge := range right {
		if edge < right[0]-pagesColumnTolerance || edge > right[0]+pagesColumnTolerance {
			t.Errorf("page counts are not right-aligned: right edges %v", right)
		}
	}
	snapshot(t, tt, "jobs-pages")
	tt.SetDark(true)
	snapshot(t, tt, "jobs-pages-dark")
}

func TestJobsPagesColumnIsHiddenInCompactWindow(t *testing.T) {
	tt := ui.NewTester(newPagesApp(t).view, 640, 460)
	if tt.HasText(colPages) || tt.HasText("123") {
		t.Fatalf("compact layout shows the Pages column; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "jobs-pages-compact")
}

// countText is how many cells of the table show text.
func countText(tt *ui.Tester, text string) int {
	n := 0
	for _, shown := range tt.Texts() {
		if shown == text {
			n++
		}
	}
	return n
}

func TestPrintingJobShowsADashUntilItsPageCountIsReported(t *testing.T) {
	app := newTestApp(t)
	jobs := &core.JobLog{}
	live := core.NewJobEntry("printer-epson", "Agung", "report.pdf", 0)
	live.SetStatus(core.JobPrinting)
	jobs.Push(live)
	app.shared = bridgenet.NewSharedState(app.printers, jobs)
	app.tab = tabJobs
	tt := ui.NewTester(app.view, 860, 620)
	// Pages and Details are both unknown for a job that has just started.
	if got := countText(tt, "—"); got != 2 {
		t.Fatalf("%d dashes before the count is known, want 2 (Pages and Details); texts: %q", got, tt.Texts())
	}
	jobs.SetPages(live.ID, 7)
	tt.Frame()
	if got := countText(tt, "—"); got != 1 || !tt.HasText("7") {
		t.Fatalf("after the report: %d dashes, want 1, and the count 7 shown; texts: %q", got, tt.Texts())
	}
}
