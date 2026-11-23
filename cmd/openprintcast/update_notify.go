package main

import (
	"fmt"

	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

// Wording of the update notice. The constant ID makes a newer notice
// replace one still shown.
const (
	updateNoticeID     = "update-available"
	updateNoticeTitle  = "Update available"
	updateNoticeFormat = appTitle + " %s is ready to download."
)

// updateAnnouncer tells the user once per version, in this run, that an
// update exists.
type updateAnnouncer struct {
	notifier  notifier
	announced string // tag of the last version announced
}

// Announce posts the notice for release unless it is the version the user
// skipped or was already announced. It reports whether it posted.
func (a *updateAnnouncer) Announce(release updatecheck.Release, skipped string) bool {
	if release.Tag == skipped || release.Tag == a.announced {
		return false
	}
	a.announced = release.Tag
	a.notifier.Notify(userNotice{
		ID: updateNoticeID, Title: updateNoticeTitle, Body: fmt.Sprintf(updateNoticeFormat, release.Tag),
	})
	return true
}
