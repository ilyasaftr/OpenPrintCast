package main

import (
	"path/filepath"
	"sync/atomic"

	"github.com/ilyasaftr/OpenPrintCast/internal/applog"
	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/jobhistory"
)

// jobHistory is what the UI needs of the stored job history: to switch
// recording on or off, to erase what is stored and to put erased jobs back
// (Undo of Clear Finished). *historyRecorder is the production
// implementation.
type jobHistory interface {
	SetEnabled(enabled bool)
	Clear() error
	Restore(entries []core.JobEntry) error
}

// historyStore is the persistent file behind a historyRecorder;
// *jobhistory.Store implements it.
type historyStore interface {
	Observe(entry core.JobEntry)
	Clear() error
	Restore(entries []core.JobEntry) error
}

// historyRecorder is the JobLog observer that records finished jobs in the
// store while recording is enabled. OnJob runs on the server goroutine, so
// the enabled flag is atomic and nothing UI-owned is read.
type historyRecorder struct {
	store   historyStore
	enabled atomic.Bool
}

// newHistoryRecorder builds a recorder that starts with the given state.
func newHistoryRecorder(store historyStore, enabled bool) *historyRecorder {
	r := &historyRecorder{store: store}
	r.enabled.Store(enabled)
	return r
}

// SetEnabled starts or stops recording; stored history is left alone.
func (r *historyRecorder) SetEnabled(enabled bool) { r.enabled.Store(enabled) }

// Clear erases the stored history.
func (r *historyRecorder) Clear() error { return r.store.Clear() }

// Restore writes erased jobs back to the stored history, unless recording
// is off: then nothing is kept on disk, as the user chose, and there is
// nothing to restore.
func (r *historyRecorder) Restore(entries []core.JobEntry) error {
	if !r.enabled.Load() {
		return nil
	}
	return r.store.Restore(entries)
}

// OnJob is the JobLog observer: it stores finished jobs when enabled.
func (r *historyRecorder) OnJob(job core.JobEntry) {
	if r.enabled.Load() {
		r.store.Observe(job)
	}
}

// nopJobHistory is the history of fixtures that set none.
type nopJobHistory struct{}

func (nopJobHistory) SetEnabled(bool)               {}
func (nopJobHistory) Clear() error                  { return nil }
func (nopJobHistory) Restore([]core.JobEntry) error { return nil }

// jobHistoryPath is the history file, in the per-user local data dir beside
// the logs: it is machine-specific and holds document and user names.
func jobHistoryPath() string {
	return filepath.Join(applog.DefaultDir(), jobhistory.FileName)
}

// attachJobHistory restores the stored history into jobs (when kept) and
// then registers the recorder as a finished-job observer. Restoring comes
// first and is silent, so earlier failures never raise notifications. The
// recorder stays registered while the setting is off, so toggling it needs
// no re-registration.
func attachJobHistory(jobs *core.JobLog, path string, keep bool) *historyRecorder {
	store := jobhistory.Open(path, core.MaxLogEntries)
	store.OnError = func(err error) {
		componentLogger(componentHistory).Warn("could not store finished job", "err", err)
	}
	if keep {
		restoreJobHistory(jobs, store, path)
	}
	recorder := newHistoryRecorder(store, keep)
	jobs.Observe(recorder.OnJob)
	return recorder
}

// restoreJobHistory loads the store into jobs and logs what it found.
func restoreJobHistory(jobs *core.JobLog, store *jobhistory.Store, path string) {
	entries, skipped, err := store.Load()
	if err != nil {
		componentLogger(componentHistory).Warn("could not read job history", "path", path, "err", err)
		return
	}
	jobs.Restore(entries)
	log := componentLogger(componentHistory)
	if skipped > 0 {
		log.Warn("skipped unreadable job history lines", "path", path, "skipped", skipped)
	}
	log.Info("job history restored", "jobs", len(entries), "skipped", skipped)
}

// history is the job history, never nil.
func (a *bridgeApp) history() jobHistory {
	if a.jobHistory == nil {
		return nopJobHistory{}
	}
	return a.jobHistory
}

// keepJobHistoryChanged applies the Keep job history switch: turning it
// off stops recording and erases the stored file at once, turning it on
// starts recording (what was erased is not recovered). Then it saves.
// When the stored history could not be erased the user is told, since
// turning the switch off promised that nothing stays on disk.
func (a *bridgeApp) keepJobHistoryChanged() {
	keep := a.cfg.KeepJobHistory
	a.history().SetEnabled(keep)
	if !keep && a.clearStoredHistory("history turned off") != nil {
		a.notices.PostError(historyEraseFailedText)
	}
	a.persistSettings()
}

// clearStoredHistory erases the stored history. A failure is logged under
// reason and returned, so the caller can tell the user.
func (a *bridgeApp) clearStoredHistory(reason string) error {
	err := a.history().Clear()
	if err != nil {
		componentLogger(componentHistory).Warn("could not clear job history", "reason", reason, "err", err)
	}
	return err
}
