package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// failingStore is a configStore whose Save always fails.
type failingStore struct{ saves int }

func (f *failingStore) Save(core.AppConfig) error { f.saves++; return errors.New("disk full") }
func (f *failingStore) Location() string          { return "nowhere" }

func TestFailedSaveReachesTheUser(t *testing.T) {
	app := newTestApp(t)
	store := &failingStore{}
	app.store = store
	logged := captureLogs(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Share EPSON L3210 Series"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if store.saves == 0 || !tt.HasText(saveFailedNotice) {
		t.Fatalf("a failed save was not shown; texts: %q", tt.Texts())
	}
	if !strings.Contains(logged.String(), "save failed") {
		t.Fatalf("the failure was not logged: %q", logged.String())
	}
}

func TestSavedChangesShowNoErrorToast(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Share EPSON L3210 Series"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText(saveFailedNotice) {
		t.Fatal("an error toast appeared although the save worked")
	}
}

func TestNoticeQueueDrainsOnce(t *testing.T) {
	var q noticeQueue
	q.PostError("first")
	q.Post(ui.Toast{Title: "second"})
	got := q.Drain()
	if len(got) != 2 || got[0].Type != "error" || got[0].Timeout != errorNoticeTimeout || got[1].Title != "second" {
		t.Fatalf("drained %+v", got)
	}
	if again := q.Drain(); len(again) != 0 {
		t.Fatalf("drained twice: %+v", again)
	}
}

func TestClearFinishedSaysWhenTheSavedHistoryRemains(t *testing.T) {
	store := &fakeHistoryStore{clearErr: errors.New("access denied")}
	app := appWithHistory(t, newHistoryRecorder(store, true))
	finishJob(app.shared.Jobs, "a")
	app.tab = tabJobs
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Clear Finished"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Cleared 1 finished job, but couldn't erase the saved history") {
		t.Fatalf("the failed erase was not reported; texts: %q", tt.Texts())
	}
}

func TestClearedMessage(t *testing.T) {
	if got := clearedMessage(2, nil); got != "Cleared 2 finished jobs" {
		t.Fatalf("got %q", got)
	}
	if got := clearedMessage(1, errors.New("x")); got != "Cleared 1 finished job, but couldn't erase the saved history" {
		t.Fatalf("got %q", got)
	}
}

func TestTurningHistoryOffTellsWhenTheFileSurvives(t *testing.T) {
	store := &fakeHistoryStore{clearErr: errors.New("access denied")}
	app := appWithHistory(t, newHistoryRecorder(store, true))
	app.cfg.KeepJobHistory = false
	app.keepJobHistoryChanged()
	tt := ui.NewTester(app.view, 860, 620)
	if !tt.HasText(historyEraseFailedText) {
		t.Fatalf("an erase failure was silent; texts: %q", tt.Texts())
	}
}

func TestFolderDialogFailureReachesTheUser(t *testing.T) {
	app := newTestApp(t)
	app.folderDialogFailed()
	tt := ui.NewTester(app.view, 860, 620)
	if !tt.HasText(folderDialogNotice) {
		t.Fatalf("a failed folder dialog was silent; texts: %q", tt.Texts())
	}
}

func TestErrorWordingSaysWhatToDo(t *testing.T) {
	for _, text := range []string{saveFailedNotice, folderDialogNotice, scanFailedText} {
		if strings.Contains(strings.ToLower(text), "you did") || strings.Contains(strings.ToLower(text), "invalid") {
			t.Errorf("%q blames the user", text)
		}
		if !strings.HasPrefix(text, "Couldn't") {
			t.Errorf("%q does not say what could not be done", text)
		}
	}
}

func TestTruncatedJobCellsCarryTheirFullText(t *testing.T) {
	app, _ := newJobsApp(t, &fakeCanceler{})
	long := "Quarterly report for the whole of the third floor.pdf"
	app.shared.Jobs.Push(core.NewJobEntry("printer-epson", "Agung", long, 3))
	tt := ui.NewTester(app.view, 640, 460)
	if a, ok := tt.Access(long); !ok || a.Description != long {
		t.Fatalf("the document cell has no full-text tooltip: %+v, %t", a, ok)
	}
	if a, ok := tt.Access("EPSON L3210 Series (PC)"); !ok || a.Description != "EPSON L3210 Series (PC)" {
		t.Fatalf("the printer cell has no full-text tooltip: %+v, %t", a, ok)
	}
}

func TestClearFinishedExplainsItself(t *testing.T) {
	app, _ := newJobsApp(t, &fakeCanceler{})
	tt := ui.NewTester(app.view, 860, 620)
	if a, ok := tt.AccessByRole(ui.RoleButton, "Clear Finished"); !ok || a.Description != clearFinishedTip {
		t.Fatalf("Clear Finished tooltip = %+v, %t", a, ok)
	}
}

func TestInvalidNameLeavesDoneReachableWithItsReason(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	openSheet(t, tt, "Microsoft Print to PDF")
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type(strings.Repeat("x", maxDNSLabelBytes+1))
	done, ok := tt.AccessByRole(ui.RoleButton, "Done")
	if !ok || !done.Disabled || !done.Focusable || done.Description != validateAirPrintName(strings.Repeat("x", maxDNSLabelBytes+1)) {
		t.Fatalf("Done = %+v, %t; it must be unavailable, reachable and say why", done, ok)
	}
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if !app.printerDialogOpen {
		t.Fatal("clicking the unavailable Done committed an invalid name")
	}
}

func TestPauseAndResumeKeepOneWidth(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	pause, _ := tt.AccessByRole(ui.RoleButton, "Pause")
	status, _ := tt.Find("Status: Sharing 1 of 2")
	app.setPaused(true)
	tt.Frame()
	resume, _ := tt.AccessByRole(ui.RoleButton, "Resume")
	if pause.Bounds.W != resume.Bounds.W || pause.Bounds.X != resume.Bounds.X {
		t.Fatalf("the header button changed box: %v -> %v", pause.Bounds, resume.Bounds)
	}
	if after, _ := tt.Find("Status: Paused"); after.X+after.W != status.X+status.W {
		t.Fatalf("the status pill moved: %v -> %v", status, after)
	}
}

func TestPrintersCaptionCountsOnlyListedPrinters(t *testing.T) {
	app := newTestApp(t)
	app.cfg.FolderPath = t.TempDir()
	app.cfg.FolderEnabled = true
	app.printers[1].IsRemote = false
	tt := ui.NewTester(app.view, 860, 620)
	if _, ok := app.folderPrinter(); !ok {
		t.Fatal("setup: the folder destination is not active")
	}
	if !tt.HasText("1 of 2 shared") || !tt.HasText("Sharing 2 of 3") {
		t.Fatalf("the Printers caption must count its own rows and the header every destination; texts: %q", tt.Texts())
	}
}

func TestCloseHintShowsOncePerRunAndOnlyWithATray(t *testing.T) {
	notifier := &fakeNotifier{}
	hint := trayHint{notifier: notifier}
	hint.OnWindowHidden(false)
	if len(notifier.got()) != 0 {
		t.Fatal("hinted although no tray icon exists to reopen the app")
	}
	hint.OnWindowHidden(true)
	hint.OnWindowHidden(true)
	got := notifier.got()
	if len(got) != 1 || got[0].Title != trayHintTitle || got[0].Body != trayHintBody || got[0].ID != trayHintID {
		t.Fatalf("notices = %+v, want the hint once", got)
	}
	var silent trayHint
	silent.OnWindowHidden(true)
}

// HIG control sizes on the desktop: 28 pt is the default, 20 pt the least.
const (
	hitDefault = 28
	hitMinimum = 20
)

func TestControlsMeetTheMinimumHitSize(t *testing.T) {
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		for _, tab := range []int{tabPrinters, tabJobs, tabSettings} {
			app, _ := newJobsApp(t, &fakeCanceler{})
			app.tab = tab
			tt := ui.NewTester(app.view, size[0], size[1])
			for _, a := range tt.AccessAll() {
				switch a.Role {
				case ui.RoleButton, ui.RoleSwitch, ui.RoleCheckBox, ui.RoleTab:
					if a.Bounds.W < hitMinimum || a.Bounds.H < hitMinimum {
						t.Errorf("%dx%d tab %d: %q is %.0fx%.0f, below %d", size[0], size[1], tab, a.Name, a.Bounds.W, a.Bounds.H, hitMinimum)
					}
					if a.Role != ui.RoleSwitch && a.Bounds.H < hitDefault && a.Name != "Decrease" && a.Name != "Increase" && !strings.HasPrefix(a.Name, "Cancel job") {
						t.Errorf("%dx%d tab %d: %q is %.0f tall, below the default %d", size[0], size[1], tab, a.Name, a.Bounds.H, hitDefault)
					}
				}
			}
		}
	}
}
