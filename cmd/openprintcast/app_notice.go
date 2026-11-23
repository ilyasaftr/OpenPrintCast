package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// errorNoticeTimeout is how long an error toast stays: longer than the
// default four seconds, because it tells the user something went wrong.
const errorNoticeTimeout = 12 * time.Second

// Wording of errors found outside the view. Each says what happened and
// what the user can do about it.
const (
	saveFailedNotice       = "Couldn't save your changes. They may be lost when you quit. Check that the settings folder is writable."
	folderDialogNotice     = "Couldn't open the folder picker. Try again."
	historyEraseFailedText = "Couldn't erase the saved job history"
)

// noticeQueue holds toasts raised where there is no ui.Context, such as a
// failed save. Only the UI thread may use it: the view shows what is
// queued on its next frame, and code on other goroutines posts from
// inside mygo.RunOnMain.
type noticeQueue struct {
	pending []ui.Toast
}

// Post queues a toast for the next frame.
func (q *noticeQueue) Post(t ui.Toast) { q.pending = append(q.pending, t) }

// PostError queues an error toast, which shows for errorNoticeTimeout. The
// text doubles as the toast's ID, so a repeated failure shows once.
func (q *noticeQueue) PostError(text string) {
	q.Post(ui.Toast{ID: text, Title: text, Type: "error", Timeout: errorNoticeTimeout})
}

// Drain returns the queued toasts and empties the queue.
func (q *noticeQueue) Drain() []ui.Toast {
	out := q.pending
	q.pending = nil
	return out
}

// showNotices shows the queued toasts; the view calls it once per frame.
func (a *bridgeApp) showNotices(c *ui.Context) {
	for _, t := range a.notices.Drain() {
		c.AddToast(t)
	}
}
