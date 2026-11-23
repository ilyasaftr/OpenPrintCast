package main

import (
	"errors"

	"github.com/egoist/mygo"
)

// userNotice is a desktop notification's content. Notices that share an ID
// replace each other while still shown.
type userNotice struct {
	ID, Title, Body string
}

// notifier shows a userNotice to the user.
type notifier interface {
	Notify(notice userNotice)
}

// systemNotifier shows notices through the operating system; onClick runs
// on the UI thread when the user clicks one. health, when set, learns
// whether the system refuses notices, for the Settings page to explain.
type systemNotifier struct {
	onClick func()
	health  *notificationHealth
}

// Notify shows notice; an unsupported or denied system only logs, and is
// remembered in health.
func (n systemNotifier) Notify(notice userNotice) {
	if !mygo.NotificationsSupported() {
		n.health.NoteUnsupported()
		componentLogger(componentNotify).Warn("notifications unsupported; dropped notice", "title", notice.Title)
		return
	}
	shown := mygo.NewNotification(mygo.NotificationOptions{ID: notice.ID, Title: notice.Title, Body: notice.Body})
	shown.OnClick(func() {
		mygo.RunOnMain(n.onClick)
		shown.Close()
	})
	err := shown.Show()
	n.health.NoteShowResult(err)
	if err != nil {
		componentLogger(componentNotify).Warn("notification not shown",
			"title", notice.Title, "denied", notificationsDenied(err), "err", err)
	}
}

// notificationsDenied tags the log line when the user has turned
// notifications off.
func notificationsDenied(err error) bool {
	return errors.Is(err, mygo.ErrNotificationsDenied)
}
