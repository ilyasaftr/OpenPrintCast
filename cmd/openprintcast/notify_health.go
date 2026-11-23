package main

import (
	"sync"
)

// notificationStatus is whether the system lets the app show notifications.
type notificationStatus int

const (
	notificationsAvailable notificationStatus = iota
	// notificationsBlocked: the user (or policy) turned notifications off
	// for the app in Windows.
	notificationsBlocked
	// notificationsUnsupported: this system cannot show notifications.
	notificationsUnsupported
)

// notificationStatusReader is what the Settings page needs of the
// notifier's health: the current status. *notificationHealth implements it.
type notificationStatusReader interface {
	Status() notificationStatus
}

// notificationHealth remembers whether notifications are blocked or
// unsupported, as the notifier's attempts and a startup probe find out. It
// is written from the server goroutine (a failed job notifies there) and
// read by the UI thread, so it is safe for concurrent use. The zero value
// is ready to use and says notifications are available.
type notificationHealth struct {
	mu     sync.Mutex
	status notificationStatus
}

// Status is the current status; a nil h says available.
func (h *notificationHealth) Status() notificationStatus {
	if h == nil {
		return notificationsAvailable
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.status
}

// ProbeSupport records an unsupported system up front, so Settings can say
// so before any notice was attempted. A nil h or probe does nothing.
func (h *notificationHealth) ProbeSupport(supported func() bool) {
	if h != nil && supported != nil && !supported() {
		h.set(notificationsUnsupported)
	}
}

// NoteUnsupported records that a notice was dropped because the system
// cannot show notifications.
func (h *notificationHealth) NoteUnsupported() {
	if h != nil {
		h.set(notificationsUnsupported)
	}
}

// NoteShowResult records the outcome of showing a notice: a denial blocks,
// a success clears an earlier block (the user allowed notifications again),
// and any other error says nothing about permission.
func (h *notificationHealth) NoteShowResult(err error) {
	if h == nil {
		return
	}
	switch {
	case err == nil:
		h.set(notificationsAvailable)
	case notificationsDenied(err):
		h.set(notificationsBlocked)
	}
}

// set stores status and logs the change into a refused state once, so a
// run of failed notices fills the log with one line, not one per notice.
func (h *notificationHealth) set(status notificationStatus) {
	h.mu.Lock()
	changed := h.status != status
	h.status = status
	h.mu.Unlock()
	if !changed {
		return
	}
	switch status {
	case notificationsBlocked:
		componentLogger(componentNotify).Warn("notifications are blocked by the system; failure notices will not show")
	case notificationsUnsupported:
		componentLogger(componentNotify).Warn("notifications are unsupported on this system; failure notices will not show")
	}
}
