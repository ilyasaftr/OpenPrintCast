package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/jobhistory"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// storedJob is a finished job with a recognisable file name.
func storedJob(id, filename string, status core.JobStatus) core.JobEntry {
	return core.JobEntry{ID: id, PrinterID: "printer-epson", User: "Agung", Filename: filename, Pages: 1, Status: status, CreatedAt: 1700000000}
}

// writeHistory stores entries in a history file under a temp dir.
func writeHistory(t *testing.T, entries ...core.JobEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data", jobhistory.FileName)
	store := jobhistory.Open(path, 0)
	for _, e := range entries {
		if err := store.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func fileLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// appWithHistory is a test app whose job log is empty and recorded by rec.
func appWithHistory(t *testing.T, rec *historyRecorder) *bridgeApp {
	t.Helper()
	app := newTestApp(t)
	jobs := &core.JobLog{}
	jobs.Observe(rec.OnJob)
	app.shared = bridgenet.NewSharedState(app.printers, jobs)
	app.jobHistory = rec
	return app
}

func finishJob(jobs *core.JobLog, id string) {
	jobs.Push(storedJob(id, id+".pdf", core.JobCompleted))
}

func TestStartupRestoresStoredJobsIntoTheJobsPage(t *testing.T) {
	path := writeHistory(t, storedJob("job-1", "report-from-last-week.pdf", core.JobCompleted),
		storedJob("job-2", "broken-from-last-week.pdf", core.JobFailed))
	logged := captureLogs(t)
	jobs := &core.JobLog{}
	attachJobHistory(jobs, path, true)

	app := newTestApp(t)
	app.shared = bridgenet.NewSharedState(app.printers, jobs)
	app.tab = 1
	tt := ui.NewTester(app.view, 860, 620)
	for _, want := range []string{"report-from-last-week.pdf", "broken-from-last-week.pdf", "Completed", "Failed"} {
		if !tt.HasText(want) {
			t.Errorf("jobs page missing %q; texts: %q", want, tt.Texts())
		}
	}
	if want := `msg="job history restored" component=history jobs=2 skipped=0`; !strings.Contains(logged.String(), want) {
		t.Errorf("log missing %q; got %q", want, logged.String())
	}
}

func TestStartupReportsSkippedLines(t *testing.T) {
	path := writeHistory(t, storedJob("job-1", "a.pdf", core.JobCompleted))
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("{torn\n")
	f.Close()
	logged := captureLogs(t)
	jobs := &core.JobLog{}
	attachJobHistory(jobs, path, true)
	if len(jobs.List()) != 1 {
		t.Fatalf("restored %d jobs, want 1", len(jobs.List()))
	}
	for _, want := range []string{"level=WARN", "skipped=1", "level=INFO", "jobs=1"} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("log missing %q; got %q", want, logged.String())
		}
	}
}

func TestRestoredFailuresDoNotNotifyWhateverTheOrder(t *testing.T) {
	for _, announcerFirst := range []bool{true, false} {
		name := map[bool]string{true: "announcer registered first", false: "announcer registered after"}[announcerFirst]
		t.Run(name, func(t *testing.T) {
			path := writeHistory(t, storedJob("job-1", "old.pdf", core.JobFailed))
			jobs := &core.JobLog{}
			shared := bridgenet.NewSharedState(nil, jobs)
			notifier := &fakeNotifier{}
			announcer := newJobFailureAnnouncer(notifier, shared, shared, true)
			if announcerFirst {
				jobs.Observe(announcer.OnJob)
			}
			attachJobHistory(jobs, path, true)
			if !announcerFirst {
				jobs.Observe(announcer.OnJob)
			}
			if got := notifier.got(); len(got) != 0 {
				t.Fatalf("restored failure raised notices: %+v", got)
			}
			if got := fileLines(t, path); len(got) != 1 {
				t.Fatalf("restoring rewrote the history: %q", got)
			}

			jobs.Push(storedJob("job-2", "new.pdf", core.JobFailed))
			if got := notifier.got(); len(got) != 1 {
				t.Fatalf("live failure notices = %d, want 1", len(got))
			}
			if got := fileLines(t, path); len(got) != 2 {
				t.Fatalf("live failure not stored: %q", got)
			}
		})
	}
}

func TestStartupWithHistoryOffRestoresAndRecordsNothing(t *testing.T) {
	path := writeHistory(t, storedJob("job-1", "old.pdf", core.JobCompleted))
	jobs := &core.JobLog{}
	attachJobHistory(jobs, path, false)
	finishJob(jobs, "job-2")
	if len(jobs.List()) != 1 || jobs.List()[0].ID != "job-2" {
		t.Fatalf("jobs = %+v, want only the live job", jobs.List())
	}
	if got := fileLines(t, path); len(got) != 1 {
		t.Fatalf("history file changed while off: %q", got)
	}
}

func TestStartupSurvivesAnUnreadableHistory(t *testing.T) {
	logged := captureLogs(t)
	jobs := &core.JobLog{}
	attachJobHistory(jobs, t.TempDir(), true) // a directory is not a history file
	if len(jobs.List()) != 0 || !strings.Contains(logged.String(), "could not read job history") {
		t.Fatalf("jobs = %v, log = %q", jobs.List(), logged.String())
	}
}

func TestHistorySwitchOffClearsAndStopsRecordingOnAndResumes(t *testing.T) {
	store := &fakeHistoryStore{}
	app := appWithHistory(t, newHistoryRecorder(store, true))
	app.tab = 2
	tt := ui.NewTester(app.view, 860, 620)

	finishJob(app.shared.Jobs, "before")
	if sw := toggleSwitch(t, tt, keepJobHistoryLabel); sw.Checked {
		t.Fatal("switch should be off after the first click")
	}
	finishJob(app.shared.Jobs, "while-off")
	observed, clears := store.state()
	if !reflect.DeepEqual(observed, []string{"before"}) || clears != 1 {
		t.Fatalf("after OFF: observed %v, clears %d; want [before], 1", observed, clears)
	}

	if sw := toggleSwitch(t, tt, keepJobHistoryLabel); !sw.Checked {
		t.Fatal("switch should be on after the second click")
	}
	finishJob(app.shared.Jobs, "after-on")
	observed, clears = store.state()
	if !reflect.DeepEqual(observed, []string{"before", "after-on"}) || clears != 1 {
		t.Fatalf("after ON: observed %v, clears %d; want [before after-on], 1", observed, clears)
	}
}

func TestHistorySwitchWithARealFile(t *testing.T) {
	path := writeHistory(t, storedJob("job-1", "old.pdf", core.JobCompleted))
	jobs := &core.JobLog{}
	app := newTestApp(t)
	app.shared = bridgenet.NewSharedState(app.printers, jobs)
	app.jobHistory = attachJobHistory(jobs, path, true)
	app.tab = 2
	tt := ui.NewTester(app.view, 860, 620)

	toggleSwitch(t, tt, keepJobHistoryLabel)
	if got := fileLines(t, path); got != nil {
		t.Fatalf("history file survived turning it off: %q", got)
	}
	finishJob(jobs, "while-off")
	if got := fileLines(t, path); got != nil {
		t.Fatalf("a job was recorded while off: %q", got)
	}

	toggleSwitch(t, tt, keepJobHistoryLabel)
	finishJob(jobs, "after-on")
	got := fileLines(t, path)
	if len(got) != 1 || !strings.Contains(got[0], `"after-on"`) {
		t.Fatalf("history after turning on = %q, want only the new job", got)
	}
}

func TestHistorySwitchLabelAndPersistence(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	tt := ui.NewTester(app.view, 860, 620)
	for _, want := range []string{"History", keepJobHistoryDetail, keepJobHistoryFooter} {
		if !tt.HasText(want) {
			t.Errorf("history group missing %q; texts: %q", want, tt.Texts())
		}
	}
	if sw := scrollToControl(t, tt, ui.RoleSwitch, keepJobHistoryLabel, 620); !sw.Checked {
		t.Fatal("job history should be kept by default")
	}
	for _, on := range []bool{false, true} {
		if sw := toggleSwitch(t, tt, keepJobHistoryLabel); sw.Checked != on {
			t.Fatalf("switch checked = %t, want %t", sw.Checked, on)
		}
		saved, err := core.LoadConfig(app.store.Location())
		if err != nil || saved.KeepJobHistory != on {
			t.Fatalf("keep_job_history=%t not persisted: %+v, %v", on, saved, err)
		}
		if on {
			continue
		}
		raw, _ := os.ReadFile(app.store.Location())
		if !strings.Contains(string(raw), `"keep_job_history": false`) {
			t.Fatalf("explicit false missing from %s", raw)
		}
	}
}

func TestHistoryClearFailureIsLoggedNotFatal(t *testing.T) {
	store := &fakeHistoryStore{clearErr: errors.New("file locked")}
	app := appWithHistory(t, newHistoryRecorder(store, true))
	app.tab = 2
	logged := captureLogs(t)
	tt := ui.NewTester(app.view, 860, 620)
	toggleSwitch(t, tt, keepJobHistoryLabel)
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || saved.KeepJobHistory {
		t.Fatalf("setting not saved after a clear failure: %+v, %v", saved, err)
	}
	if want := `msg="could not clear job history" component=history reason="history turned off" err="file locked"`; !strings.Contains(logged.String(), want) {
		t.Fatalf("log missing %q; got %q", want, logged.String())
	}
}

func TestClearFinishedClearsTheStoredHistory(t *testing.T) {
	store := &fakeHistoryStore{}
	app := appWithHistory(t, newHistoryRecorder(store, true))
	finishJob(app.shared.Jobs, "a")
	finishJob(app.shared.Jobs, "b")
	app.tab = 1
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Clear Finished"); err != nil {
		t.Fatal(err)
	}
	if _, clears := store.state(); clears != 1 {
		t.Fatalf("store cleared %d times, want 1", clears)
	}
	if !tt.HasText("No Print Jobs") || !tt.HasText("Cleared 2 finished jobs") {
		t.Fatalf("page after clearing: %q", tt.Texts())
	}
}

func TestClearFinishedToleratesAClearError(t *testing.T) {
	store := &fakeHistoryStore{clearErr: errors.New("access denied")}
	app := appWithHistory(t, newHistoryRecorder(store, true))
	finishJob(app.shared.Jobs, "a")
	app.tab = 1
	logged := captureLogs(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Clear Finished"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Cleared 1 finished job") || len(app.shared.Jobs.List()) != 0 {
		t.Fatalf("the page did not clear despite the store error: %q", tt.Texts())
	}
	if want := `reason="clear finished" err="access denied"`; !strings.Contains(logged.String(), want) {
		t.Fatalf("log missing %q; got %q", want, logged.String())
	}
}

func TestClearFinishedRemovesTheStoredFile(t *testing.T) {
	path := writeHistory(t, storedJob("job-1", "old.pdf", core.JobCompleted))
	jobs := &core.JobLog{}
	app := newTestApp(t)
	app.shared = bridgenet.NewSharedState(app.printers, jobs)
	app.jobHistory = attachJobHistory(jobs, path, true)
	app.tab = 1
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Clear Finished"); err != nil {
		t.Fatal(err)
	}
	if got := fileLines(t, path); got != nil {
		t.Fatalf("history file survived Clear Finished: %q", got)
	}
}

func TestHistoryRecorderOnlyRecordsWhileEnabled(t *testing.T) {
	store := &fakeHistoryStore{}
	rec := newHistoryRecorder(store, false)
	rec.OnJob(storedJob("a", "a.pdf", core.JobCompleted))
	rec.SetEnabled(true)
	rec.OnJob(storedJob("b", "b.pdf", core.JobCompleted))
	rec.SetEnabled(false)
	rec.OnJob(storedJob("c", "c.pdf", core.JobCompleted))
	if observed, _ := store.state(); !reflect.DeepEqual(observed, []string{"b"}) {
		t.Fatalf("observed = %v, want [b]", observed)
	}
}

func TestAppWithoutHistoryIsSafe(t *testing.T) {
	app := newTestApp(t)
	app.jobHistory = nil
	app.keepJobHistoryChanged()
	app.clearStoredHistory("test")
}

func TestJobHistoryPathIsBesideTheLogs(t *testing.T) {
	t.Setenv("LOCALAPPDATA", filepath.Join("C:", "Users", "x", "Local"))
	if got, want := jobHistoryPath(), filepath.Join("C:", "Users", "x", "Local", "OpenPrintCast", "jobs.jsonl"); got != want {
		t.Fatalf("jobHistoryPath = %q, want %q", got, want)
	}
}
