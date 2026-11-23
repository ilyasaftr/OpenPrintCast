package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

func TestEmptyJobsOffersNextStep(t *testing.T) {
	app := newTestApp(t)
	app.shared = bridgenet.NewSharedState(app.printers, nil)
	app.tab = 1
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Show Printers"); err != nil {
		t.Fatal(err)
	}
	if app.tab != 0 || !tt.HasText("Windows printers") && !tt.HasText("Destinations") {
		t.Fatalf("Show Printers did not go to the printers page; tab=%d", app.tab)
	}
}

func TestClearFinishedEmptiesJobsAndExplains(t *testing.T) {
	app := newTestApp(t)
	app.tab = 1
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Clear Finished"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("No Print Jobs") || len(app.shared.Jobs.List()) != 0 {
		t.Fatalf("finished jobs were not cleared: %q", tt.Texts())
	}
}

func TestJobStatusesUseDistinctSymbols(t *testing.T) {
	seen := map[*ui.SVG]core.JobStatus{}
	for _, s := range []core.JobStatus{core.JobPending, core.JobPrinting, core.JobCompleted, core.JobFailed, core.JobCancelled} {
		icon := jobStatusIcon(s)
		if prev, dup := seen[icon]; dup {
			t.Errorf("%q and %q share a status symbol", prev, s)
		}
		seen[icon] = s
	}
}

func TestJobsStatusStaysVisibleInCompactWindow(t *testing.T) {
	app := newTestApp(t)
	app.tab = 1
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		tt := ui.NewTester(app.view, size[0], size[1])
		for _, label := range []string{"Status", "Completed", "Failed", "invoice.pdf"} {
			r, ok := tt.Find(label)
			if !ok || r.X < 0 || r.X+r.W > float32(size[0]) {
				t.Errorf("%dx%d: %q is missing or outside the window: %v", size[0], size[1], label, r)
			}
		}
	}
}

func TestShowsReasonInlineOnlyForFailedJobsWithAReasonInCompactLayout(t *testing.T) {
	failed := core.JobEntry{Status: core.JobFailed, Reason: "Printer is not accepting jobs"}
	for _, tc := range []struct {
		name    string
		compact bool
		job     core.JobEntry
		want    bool
	}{
		{"compact failed with reason", true, failed, true},
		{"wide layout has a Details column", false, failed, false},
		{"failed without a reason", true, core.JobEntry{Status: core.JobFailed, Reason: "  "}, false},
		{"cancelled says so in its label", true, core.JobEntry{Status: core.JobCancelled, Reason: "Cancelled"}, false},
		{"completed", true, core.JobEntry{Status: core.JobCompleted}, false},
	} {
		if got := showsReasonInline(tc.compact, tc.job); got != tc.want {
			t.Errorf("%s: got %t, want %t", tc.name, got, tc.want)
		}
	}
}

func TestCompactFailedRowShowsItsReasonWithoutHovering(t *testing.T) {
	app, _ := newJobsApp(t, &fakeCanceler{})
	long := failureReason + " because the page size is not supported by this printer model"
	jobs := app.shared.Jobs.List()
	for _, j := range jobs {
		if j.Status == core.JobFailed {
			app.shared.Jobs.Push(core.JobEntry{ID: "long", PrinterID: j.PrinterID, Filename: "long.pdf", Status: core.JobFailed, Reason: long})
		}
	}
	app.shared.Jobs.Restore([]core.JobEntry{storedJob("older", "older.pdf", core.JobCompleted)})
	tt := ui.NewTester(app.view, 640, 460)
	reason, ok := tt.Find(failureReason)
	status, okStatus := tt.Find("Failed")
	if !ok || !okStatus {
		t.Fatalf("reason or status missing; texts: %q", tt.Texts())
	}
	if reason.Y <= status.Y || reason.X > status.X+status.W {
		t.Errorf("reason %v is not the second line under the status %v", reason, status)
	}
	if reason.X+reason.W > 640 {
		t.Errorf("reason %v runs out of the window", reason)
	}
	// A reason too long for the cell is ellipsized, with the whole text as its tooltip.
	carries := 0
	for _, a := range tt.AccessAll() {
		if a.Description == long {
			carries++
		}
	}
	if carries != 1 {
		t.Errorf("%d elements carry the long reason as a tooltip, want exactly 1", carries)
	}
	if tt.HasText("Cancel job broken.pdf") {
		t.Error("a failed job offers Cancel")
	}
	// Failed rows are taller, the others keep their height, and Status and Cancel stay in view.
	done, _ := tt.Find("invoice.pdf")
	older, _ := tt.Find("older.pdf")
	failedDoc, _ := tt.Find("broken.pdf")
	cancel, okCancel := tt.Find("Cancel job report.pdf")
	if !okCancel || cancel.X+cancel.W > 640 {
		t.Errorf("Cancel is missing or outside the window: %v", cancel)
	}
	if failedRow, plainRow := done.Y-failedDoc.Y, older.Y-done.Y; failedRow <= plainRow {
		t.Errorf("a failed row (%v high) is not taller than a completed one (%v)", failedRow, plainRow)
	}
	snapshot(t, tt, "jobs-compact-failed")
	tt.SetDark(true)
	snapshot(t, tt, "jobs-compact-failed-dark")
}
