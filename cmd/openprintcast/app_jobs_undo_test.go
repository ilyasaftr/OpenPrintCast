package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/jobhistory"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// jobIDs lists the ids of the entries.
func jobIDs(entries []core.JobEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

// appWithRealHistory is a test app on the Jobs tab whose log is recorded
// in a real history file holding finished jobs f1 and f2 around a live
// one, p1. It returns the app and the history file's path.
func appWithRealHistory(t *testing.T) (*bridgeApp, string) {
	t.Helper()
	path := writeHistory(t)
	jobs := &core.JobLog{}
	app := newTestApp(t)
	app.shared = bridgenet.NewSharedState(app.printers, jobs)
	app.jobHistory = attachJobHistory(jobs, path, true)
	app.tab = tabJobs
	jobs.Push(storedJob("f1", "one.pdf", core.JobCompleted))
	jobs.Push(core.JobEntry{ID: "p1", PrinterID: "printer-epson", Filename: "live.pdf", Status: core.JobPrinting})
	jobs.Push(storedJob("f2", "two.pdf", core.JobFailed))
	return app, path
}

// storedIDs reads the ids in the history file.
func storedIDs(t *testing.T, path string) []string {
	t.Helper()
	entries, _, err := jobhistory.Open(path, 0).Load()
	if err != nil {
		t.Fatal(err)
	}
	return jobIDs(entries)
}

func TestUndoClearFinishedRestoresTheListAndTheFile(t *testing.T) {
	app, path := appWithRealHistory(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Clear Finished"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Cleared 2 finished jobs") || !tt.HasText(clearUndoLabel) {
		t.Fatalf("no undo offer; texts: %q", tt.Texts())
	}
	if got := jobIDs(app.shared.Jobs.List()); !reflect.DeepEqual(got, []string{"p1"}) || storedIDs(t, path) != nil {
		t.Fatalf("after clearing: list %v, file %v", got, storedIDs(t, path))
	}

	if err := tt.Click(clearUndoLabel); err != nil {
		t.Fatal(err)
	}
	if got := jobIDs(app.shared.Jobs.List()); !reflect.DeepEqual(got, []string{"f1", "p1", "f2"}) {
		t.Fatalf("restored list = %v, want the original order f1 p1 f2", got)
	}
	if got := storedIDs(t, path); !reflect.DeepEqual(got, []string{"f1", "f2"}) {
		t.Fatalf("restored file = %v, want [f1 f2]", got)
	}
	if !tt.HasText("Restored 2 finished jobs") || !tt.HasText("one.pdf") || tt.HasText(clearUndoLabel) {
		t.Fatalf("after Undo: %q", tt.Texts())
	}
	snapshot(t, tt, "jobs-undo-restored")
}

func TestUndoIsNotPossibleAfterTheToastExpires(t *testing.T) {
	app, path := appWithRealHistory(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Clear Finished"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(clearUndoWindow + time.Second)
	if tt.HasText(clearUndoLabel) || tt.HasText("Cleared 2 finished jobs") {
		t.Fatalf("the toast outlived its window: %q", tt.Texts())
	}
	if len(app.shared.Jobs.List()) != 1 || storedIDs(t, path) != nil {
		t.Fatal("clearing was undone without anyone asking")
	}
}

func TestClearUndoIsIdempotentAndEndsWithItsWindow(t *testing.T) {
	app, path := appWithRealHistory(t)
	cleared, err := app.clearFinishedJobs()
	if err != nil || cleared.Len() != 2 {
		t.Fatalf("cleared %d, err %v", cleared.Len(), err)
	}
	undo := &clearUndo{app: app, cleared: cleared}
	undo.Undo()
	undo.Undo()
	if got := jobIDs(app.shared.Jobs.List()); !reflect.DeepEqual(got, []string{"f1", "p1", "f2"}) {
		t.Fatalf("list after two Undos = %v", got)
	}
	if got := storedIDs(t, path); !reflect.DeepEqual(got, []string{"f1", "f2"}) {
		t.Fatalf("file after two Undos = %v", got)
	}
	if got := app.notices.Drain(); len(got) != 1 {
		t.Fatalf("%d notices after two Undos, want one", len(got))
	}

	again, _ := app.clearFinishedJobs()
	expired := &clearUndo{app: app, cleared: again}
	expired.Expire()
	expired.Undo()
	if got := jobIDs(app.shared.Jobs.List()); !reflect.DeepEqual(got, []string{"p1"}) {
		t.Fatalf("an expired Undo restored %v", got)
	}
}

func TestUndoKeepsJobsFinishedMeanwhileInOrderInTheFile(t *testing.T) {
	app, path := appWithRealHistory(t)
	cleared, _ := app.clearFinishedJobs()
	app.shared.Jobs.Push(storedJob("f3", "three.pdf", core.JobCompleted)) // recorded after the clear
	(&clearUndo{app: app, cleared: cleared}).Undo()
	if got := storedIDs(t, path); !reflect.DeepEqual(got, []string{"f1", "f2", "f3"}) {
		t.Fatalf("file = %v, want the restored jobs before the newer one", got)
	}
}

func TestUndoSaysWhenTheHistoryFileCannotBeRewritten(t *testing.T) {
	store := &fakeHistoryStore{restoreErr: errors.New("access denied")}
	app := appWithHistory(t, newHistoryRecorder(store, true))
	finishJob(app.shared.Jobs, "a")
	app.tab = tabJobs
	logged := captureLogs(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Clear Finished"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(clearUndoLabel); err != nil {
		t.Fatal(err)
	}
	if got := jobIDs(app.shared.Jobs.List()); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("the job is not back on the page: %v", got)
	}
	if !tt.HasText("Restored 1 finished job, but couldn't save them to the history file") {
		t.Fatalf("the failure was not reported; texts: %q", tt.Texts())
	}
	if want := `msg="could not restore job history"`; !strings.Contains(logged.String(), want) {
		t.Fatalf("log missing %q; got %q", want, logged.String())
	}
}

func TestUndoLeavesTheFileAloneWhenHistoryIsOff(t *testing.T) {
	store := &fakeHistoryStore{}
	app := appWithHistory(t, newHistoryRecorder(store, false))
	finishJob(app.shared.Jobs, "a")
	cleared, _ := app.clearFinishedJobs()
	(&clearUndo{app: app, cleared: cleared}).Undo()
	if len(app.shared.Jobs.List()) != 1 || len(store.restores()) != 0 {
		t.Fatalf("list %v, restores %v; want the job back on the page and no file write", app.shared.Jobs.List(), store.restores())
	}
	if got := app.notices.Drain(); len(got) != 1 || got[0].Type == "error" {
		t.Fatalf("notices = %+v, want one plain confirmation", got)
	}
}

func TestRestoredMessage(t *testing.T) {
	if got := restoredMessage(5, nil); got != "Restored 5 finished jobs" {
		t.Fatalf("got %q", got)
	}
	if got := restoredMessage(1, errors.New("x")); got != "Restored 1 finished job, but couldn't save them to the history file" {
		t.Fatalf("got %q", got)
	}
}
