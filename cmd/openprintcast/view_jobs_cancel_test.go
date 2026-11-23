package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

const failureReason = "The printer rejected the document"

// fakeCanceler records the ids it is asked to cancel.
type fakeCanceler struct {
	result bool
	ids    []string
}

func (f *fakeCanceler) CancelJob(id string) bool {
	f.ids = append(f.ids, id)
	return f.result
}

// newJobsApp builds a Jobs page holding an in-flight job (printing), a
// finished one and a failed one with a reason.
func newJobsApp(t *testing.T, canceler jobCanceler) (*bridgeApp, core.JobEntry) {
	t.Helper()
	app := newTestApp(t)
	jobs := &core.JobLog{}
	done := core.NewJobEntry("printer-epson", "Agung", "invoice.pdf", 2)
	done.SetStatus(core.JobCompleted)
	failed := core.NewJobEntry("printer-epson", "Agung", "broken.pdf", 1)
	failed.SetStatus(core.JobFailed)
	failed.Reason = failureReason
	live := core.NewJobEntry("printer-epson", "Agung", "report.pdf", 12)
	live.SetStatus(core.JobPrinting)
	for _, j := range []core.JobEntry{done, failed, live} {
		jobs.Push(j)
	}
	app.shared = bridgenet.NewSharedState(app.printers, jobs)
	app.canceler = canceler
	app.tab = tabJobs
	return app, live
}

func TestCancelButtonOnlyForInFlightJobs(t *testing.T) {
	app, _ := newJobsApp(t, &fakeCanceler{})
	tt := ui.NewTester(app.view, 860, 620)
	if _, ok := tt.Access("Cancel job report.pdf"); !ok {
		t.Fatalf("in-flight job has no Cancel button; texts: %q", tt.Texts())
	}
	for _, doc := range []string{"invoice.pdf", "broken.pdf"} {
		if _, ok := tt.Access("Cancel job " + doc); ok {
			t.Errorf("finished job %s offers Cancel", doc)
		}
	}
	pending := core.NewJobEntry("printer-epson", "Agung", "queued.pdf", 1)
	app.shared.Jobs.Push(pending)
	tt = ui.NewTester(app.view, 860, 620)
	if _, ok := tt.Access("Cancel job queued.pdf"); !ok {
		t.Error("pending job has no Cancel button")
	}
}

func TestCancelButtonIsLabelledAndHasTooltip(t *testing.T) {
	app, _ := newJobsApp(t, &fakeCanceler{})
	tt := ui.NewTester(app.view, 860, 620)
	a, ok := tt.AccessByRole(ui.RoleButton, "Cancel job report.pdf")
	if !ok || a.Description != "Cancel this job" {
		t.Fatalf("Cancel button: %+v, found %t", a, ok)
	}
}

func TestCancelClickCallsCancelerAndToasts(t *testing.T) {
	for _, tc := range []struct {
		result bool
		toast  string
	}{
		{true, "Cancelling “report.pdf”…"},
		{false, "This job can no longer be cancelled"},
	} {
		t.Run(fmt.Sprint(tc.result), func(t *testing.T) {
			fake := &fakeCanceler{result: tc.result}
			app, live := newJobsApp(t, fake)
			tt := ui.NewTester(app.view, 860, 620)
			if err := tt.Click("Cancel job report.pdf"); err != nil {
				t.Fatal(err)
			}
			if len(fake.ids) != 1 || fake.ids[0] != live.ID {
				t.Fatalf("CancelJob called with %v, want [%s]", fake.ids, live.ID)
			}
			if !tt.HasText(tc.toast) {
				t.Fatalf("toast %q not shown; texts: %q", tc.toast, tt.Texts())
			}
			snapshot(t, tt, fmt.Sprintf("jobs-cancel-%t", tc.result))
		})
	}
}

func TestCancelJobWithoutCancelerIsRefused(t *testing.T) {
	app, live := newJobsApp(t, nil)
	if got := app.cancelJob(live); got != "This job can no longer be cancelled" {
		t.Fatalf("message = %q", got)
	}
}

func TestDetailsColumnOnlyInWideWindow(t *testing.T) {
	app, _ := newJobsApp(t, &fakeCanceler{})
	wide := ui.NewTester(app.view, 860, 620)
	if countTexts(wide, "Details") != 1 || countTexts(wide, failureReason) != 1 {
		t.Fatalf("wide window should show Details and the reason; texts: %q", wide.Texts())
	}
	if countTexts(wide, "—") < 2 {
		t.Errorf("jobs without a reason should show a dash; texts: %q", wide.Texts())
	}
	compact := ui.NewTester(app.view, 640, 460)
	// The reason no longer waits for a hover: with no Details column it is
	// the muted second line of the failed job's Status cell, once.
	if countTexts(compact, "Details") != 0 || countTexts(compact, failureReason) != 1 {
		t.Fatalf("compact window should not show Details but the reason inline; texts: %q", compact.Texts())
	}
}

func TestFailedJobShowsReasonAsTooltip(t *testing.T) {
	app, _ := newJobsApp(t, &fakeCanceler{})
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		tt := ui.NewTester(app.view, size[0], size[1])
		if countTexts(tt, "Failed") == 0 {
			t.Fatalf("%dx%d: label should stay Failed; texts: %q", size[0], size[1], tt.Texts())
		}
		found := false
		for _, a := range tt.AccessAll() {
			found = found || a.Description == failureReason
		}
		if !found {
			t.Errorf("%dx%d: no element carries the reason as its tooltip", size[0], size[1])
		}
	}
}

func TestInFlightStatusAndCancelStayInsideCompactWindow(t *testing.T) {
	app, _ := newJobsApp(t, &fakeCanceler{})
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		tt := ui.NewTester(app.view, size[0], size[1])
		for _, label := range []string{"Status", "Printing", "Cancel job report.pdf"} {
			r, ok := tt.Find(label)
			if !ok || r.X < 0 || r.W <= 0 || r.X+r.W > float32(size[0]) {
				t.Errorf("%dx%d: %q is missing or outside the window: %v", size[0], size[1], label, r)
			}
		}
		snapshot(t, tt, fmt.Sprintf("jobs-inflight-%d", size[0]))
		tt.SetDark(true)
		snapshot(t, tt, fmt.Sprintf("jobs-inflight-dark-%d", size[0]))
	}
}

func TestJobDetailsDash(t *testing.T) {
	if jobDetails("") != "—" || jobDetails("  ") != "—" || jobDetails("Paper jam") != "Paper jam" {
		t.Fatal("jobDetails should dash blanks and pass reasons through")
	}
}

func TestJobInFlight(t *testing.T) {
	want := map[core.JobStatus]bool{core.JobPending: true, core.JobPrinting: true, core.JobCompleted: false, core.JobFailed: false, core.JobCancelled: false}
	for s, w := range want {
		if jobInFlight(s) != w {
			t.Errorf("jobInFlight(%q) = %t", s, !w)
		}
	}
}
