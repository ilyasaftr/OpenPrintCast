package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// Undo of Clear Finished: the Undo button lives as long as the toast.
const (
	clearUndoWindow = 8 * time.Second
	clearUndoLabel  = "Undo"
)

// jobCanceler stops an in-flight job by its job-log id; it reports whether
// a running job was actually cancelled. *SharedState implements it.
type jobCanceler interface {
	CancelJob(id string) bool
}

// cancelJob asks the canceler to stop the job and returns the message to
// tell the user what happened.
func (a *bridgeApp) cancelJob(entry core.JobEntry) string {
	if a.canceler != nil && a.canceler.CancelJob(entry.ID) {
		return fmt.Sprintf("Cancelling “%s”…", jobDocument(entry.Filename))
	}
	return "This job can no longer be cancelled"
}

// clearFinishedJobs empties the finished jobs from the page and from the
// stored history, returning what the page dropped (for Undo) and the error
// of erasing the stored history, if any.
func (a *bridgeApp) clearFinishedJobs() (core.ClearedJobs, error) {
	cleared := a.shared.Jobs.TakeFinished()
	return cleared, a.clearStoredHistory("clear finished")
}

// clearedMessage tells the user what Clear Finished did: the page always
// clears, and a history file that could not be erased is named.
func clearedMessage(removed int, err error) string {
	msg := "Cleared " + plural(removed, "finished job")
	if err != nil {
		msg += ", but couldn't erase the saved history"
	}
	return msg
}

// clearFinishedWithUndo is the Clear Finished command: it clears, then
// tells the user what happened in a toast that offers Undo for a short
// while. Nothing to undo (no job was cleared) gets a plain toast.
func (a *bridgeApp) clearFinishedWithUndo(c *ui.Context) {
	cleared, err := a.clearFinishedJobs()
	msg := clearedMessage(cleared.Len(), err)
	if cleared.Len() == 0 {
		c.Toast(msg)
		return
	}
	undo := &clearUndo{app: a, cleared: cleared}
	c.AddToast(ui.Toast{
		ID: msg, Title: msg, Timeout: clearUndoWindow,
		Action: clearUndoLabel, OnAction: undo.Undo, OnClose: undo.Expire,
	})
}

// clearUndo is one Clear Finished that can still be undone. Only the UI
// thread touches it: the toast runs both of its callbacks there.
type clearUndo struct {
	app     *bridgeApp
	cleared core.ClearedJobs
	closed  bool
}

// Expire ends the undo window; it is the toast's OnClose, which also runs
// after Undo itself.
func (u *clearUndo) Expire() { u.closed = true }

// Undo puts the cleared jobs back in the page, in their original order,
// and in the stored history when it is kept. It does nothing once the
// window ended or after the first call. A history file that cannot be
// rewritten still leaves the jobs back on the page, and the user is told.
func (u *clearUndo) Undo() {
	if u.closed {
		return
	}
	u.closed = true
	a := u.app
	restored := a.shared.Jobs.Reinstate(u.cleared)
	if restored == 0 {
		return
	}
	if err := a.history().Restore(u.cleared.Entries()); err != nil {
		componentLogger(componentHistory).Warn("could not restore job history", "err", err)
		a.notices.PostError(restoredMessage(restored, err))
		return
	}
	msg := restoredMessage(restored, nil)
	a.notices.Post(ui.Toast{ID: msg, Title: msg})
}

// restoredMessage tells the user what Undo did: the jobs are always back
// on the page, and a history file that could not be rewritten is named.
func restoredMessage(restored int, err error) string {
	msg := "Restored " + plural(restored, "finished job")
	if err != nil {
		msg += ", but couldn't save them to the history file"
	}
	return msg
}
