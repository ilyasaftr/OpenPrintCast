package main

import (
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// appWithFailureBadge is a test app on the Printers tab whose job log feeds
// a failure tally; it returns the app and the log.
func appWithFailureBadge(t *testing.T) (*bridgeApp, *core.JobLog) {
	t.Helper()
	app := newTestApp(t)
	jobs := &core.JobLog{}
	tally := &failureTally{}
	jobs.Observe(tally.OnJob)
	app.shared.Jobs = jobs
	app.failures = tally
	app.tab = tabPrinters
	return app, jobs
}

func failJob(jobs *core.JobLog, id string) {
	jobs.Push(storedJob(id, id+".pdf", core.JobFailed))
}

func TestFailureTallyCountsOnlyFailuresAndResets(t *testing.T) {
	var tally failureTally
	changes := 0
	tally.changed = func() { changes++ }
	for _, status := range []core.JobStatus{core.JobCompleted, core.JobCancelled, core.JobFailed, core.JobFailed} {
		tally.OnJob(core.JobEntry{Status: status})
	}
	if tally.Unseen() != 2 || changes != 2 {
		t.Fatalf("unseen %d after %d change calls, want 2 and 2", tally.Unseen(), changes)
	}
	tally.MarkSeen()
	if tally.Unseen() != 0 {
		t.Fatal("MarkSeen did not clear the count")
	}
}

func TestFailureTallyIsThreadSafe(t *testing.T) {
	var tally failureTally
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				tally.OnJob(core.JobEntry{Status: core.JobFailed})
				_ = tally.Unseen()
			}
		}()
	}
	wg.Wait()
	if tally.Unseen() != 400 {
		t.Fatalf("unseen = %d, want 400", tally.Unseen())
	}
}

func TestFailureTallyIgnoresRestoredHistory(t *testing.T) {
	jobs := &core.JobLog{}
	var tally failureTally
	jobs.Observe(tally.OnJob)
	jobs.Restore([]core.JobEntry{storedJob("old", "old.pdf", core.JobFailed)})
	if tally.Unseen() != 0 {
		t.Fatal("a failure restored from an earlier run was counted")
	}
}

func TestJobsTabBadgeShowsUnseenFailuresWithAccessibleName(t *testing.T) {
	app, jobs := appWithFailureBadge(t)
	tt := ui.NewTester(app.view, 860, 620)
	if _, ok := tt.AccessByRole(ui.RoleTab, "Jobs"); !ok {
		t.Fatal("without failures the tab is just Jobs")
	}
	failJob(jobs, "a")
	failJob(jobs, "b")
	tt.Frame()
	tab, ok := tt.AccessByRole(ui.RoleTab, "Jobs, 2 new failures")
	if !ok {
		t.Fatalf("no tab named \"Jobs, 2 new failures\"; texts: %q", tt.Texts())
	}
	if !tt.HasText("2") {
		t.Fatalf("the number is not shown: %q", tt.Texts())
	}
	snapshot(t, tt, "tabs-badge")
	tt.SetDark(true)
	snapshot(t, tt, "tabs-badge-dark")
	tt.SetDark(false)
	_ = tab

	failJob(jobs, "c")
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleTab, "Jobs, 3 new failures"); !ok {
		t.Fatal("the count did not follow the new failure")
	}
}

func TestJobsTabBadgeSaysOneFailureInTheSingular(t *testing.T) {
	app, jobs := appWithFailureBadge(t)
	tt := ui.NewTester(app.view, 640, 460)
	failJob(jobs, "a")
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleTab, "Jobs, 1 new failure"); !ok {
		t.Fatalf("no tab named \"Jobs, 1 new failure\"; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "tabs-badge-640")
	tt.SetDark(true)
	snapshot(t, tt, "tabs-badge-dark-640")
}

func TestBadgeClearsWhenTheJobsTabIsSelected(t *testing.T) {
	app, jobs := appWithFailureBadge(t)
	tt := ui.NewTester(app.view, 860, 620)
	failJob(jobs, "a")
	tt.Frame()
	if err := tt.Click("Jobs, 1 new failure"); err != nil {
		t.Fatal(err)
	}
	if app.tab != tabJobs || app.failures.Unseen() != 0 {
		t.Fatalf("tab %d, unseen %d; want Jobs and nothing unseen", app.tab, app.failures.Unseen())
	}
	if _, ok := tt.AccessByRole(ui.RoleTab, "Jobs"); !ok {
		t.Fatalf("the badge stayed on the chosen tab; texts: %q", tt.Texts())
	}
	// A failure while the Jobs page is shown is seen at once.
	failJob(jobs, "b")
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleTab, "Jobs"); !ok || app.failures.Unseen() != 0 {
		t.Fatal("a failure arriving on the Jobs page raised a badge")
	}
	// Leaving the page starts counting again, from zero.
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	failJob(jobs, "c")
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleTab, "Jobs, 1 new failure"); !ok {
		t.Fatalf("the count did not restart; texts: %q", tt.Texts())
	}
}

func TestBadgeCapsLargeCounts(t *testing.T) {
	if badgeText(7) != "7" || badgeText(99) != "99" || badgeText(100) != "99+" {
		t.Fatal("badge text wrong")
	}
}

func TestTabBarKeepsKeyboardBehaviour(t *testing.T) {
	app, _ := appWithFailureBadge(t)
	tt := ui.NewTester(app.view, 860, 620)
	for _, name := range []string{"Printers", "Jobs", "Settings"} {
		if _, ok := tt.AccessByRole(ui.RoleTab, name); !ok {
			t.Fatalf("tab %q missing", name)
		}
	}
	if err := tt.Click("Printers"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	if app.tab != tabJobs {
		t.Fatalf("Right arrow chose tab %d, want Jobs", app.tab)
	}
	tt.Key(0, ui.KeyEnd)
	if app.tab != tabSettings {
		t.Fatalf("End chose tab %d, want Settings", app.tab)
	}
}

func TestBadgeColorsCarryTheNumber(t *testing.T) {
	for _, base := range []*ui.Theme{ui.LightTheme(), ui.DarkTheme()} {
		theme := accessibleTheme(base)
		fill, text := badgeColors(theme)
		if got := contrastRatio(text, fill); got < minTextContrast {
			t.Errorf("dark=%t: badge number = %.2f:1, want at least %.1f", theme.Dark, got, minTextContrast)
		}
	}
	custom := ui.DarkTheme()
	custom.Danger = ui.Hex("#990000") // as a contrast setting would
	if fill, _ := badgeColors(custom); fill != custom.Danger {
		t.Fatal("a danger color the desktop changed was overridden")
	}
}
