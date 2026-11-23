package main

// Wording of the close-to-tray hint.
const (
	trayHintID    = "close-to-tray"
	trayHintTitle = "Still running in the tray"
	trayHintBody  = "Sharing continues in the background. Click the tray icon to open the window, or choose Quit from its menu."
)

// trayHint tells the user, the first time a run hides the window, that the
// app did not quit: the close button only moves it to the tray. Without
// the hint a closed window looks like a stopped app. It is used on the UI
// thread only; its zero value (no notifier) stays silent.
type trayHint struct {
	notifier notifier
	shown    bool
}

// OnWindowHidden is called when the close button hid the window. The hint
// shows once, and only when a tray icon exists to find the app again by.
func (h *trayHint) OnWindowHidden(trayAvailable bool) {
	if h.shown || h.notifier == nil || !trayAvailable {
		return
	}
	h.shown = true
	h.notifier.Notify(userNotice{ID: trayHintID, Title: trayHintTitle, Body: trayHintBody})
}
