package main

import "testing"

func TestChooseCloseAction(t *testing.T) {
	cases := []struct {
		name                     string
		trayAvailable, minimized bool
		want                     closeAction
	}{
		{"tray: hides", true, false, closeToTray},
		{"tray: hides even if minimized", true, true, closeToTray},
		{"no tray: minimizes, keeping a taskbar entry", false, false, closeMinimize},
		{"no tray, already minimized: quits, the only way out", false, true, closeQuit},
	}
	for _, tc := range cases {
		if got := chooseCloseAction(tc.trayAvailable, tc.minimized); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// fakeClosableWindow records what the close handler did to the window.
type fakeClosableWindow struct {
	minimized bool
	calls     []string
}

func (f *fakeClosableWindow) Hide()             { f.calls = append(f.calls, "hide") }
func (f *fakeClosableWindow) Minimize()         { f.calls = append(f.calls, "minimize") }
func (f *fakeClosableWindow) IsMinimized() bool { return f.minimized }

func TestCloseButtonHidesToTrayAndHintsOnce(t *testing.T) {
	app := newTestApp(t)
	notifier := &fakeNotifier{}
	app.closeHint = trayHint{notifier: notifier}
	win := &fakeClosableWindow{}

	for range 2 {
		if !app.onWindowClose(win, true) {
			t.Fatal("close was not cancelled")
		}
	}
	if len(win.calls) != 2 || win.calls[0] != "hide" || len(notifier.got()) != 1 {
		t.Fatalf("calls = %v, notices = %d; want hides and one hint", win.calls, len(notifier.got()))
	}
}

func TestCloseButtonMinimizesWithoutTray(t *testing.T) {
	app := newTestApp(t)
	notifier := &fakeNotifier{}
	app.closeHint = trayHint{notifier: notifier}
	win := &fakeClosableWindow{}

	if !app.onWindowClose(win, false) {
		t.Fatal("close was not cancelled")
	}
	if len(win.calls) != 1 || win.calls[0] != "minimize" || len(notifier.got()) != 0 {
		t.Fatalf("calls = %v, notices = %d; want one minimize and no tray hint", win.calls, len(notifier.got()))
	}
}

func TestCloseWhileQuittingIsLeftAlone(t *testing.T) {
	app := newTestApp(t)
	app.quitting = true
	win := &fakeClosableWindow{}
	if app.onWindowClose(win, true) || len(win.calls) != 0 {
		t.Fatalf("a quitting app must let the window close; calls = %v", win.calls)
	}
}
