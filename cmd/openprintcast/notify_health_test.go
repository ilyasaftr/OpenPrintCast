package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// fakeStatusReader serves a fixed notification status.
type fakeStatusReader struct{ status notificationStatus }

func (f fakeStatusReader) Status() notificationStatus { return f.status }

func TestNotificationHealthRecordsADenial(t *testing.T) {
	var h notificationHealth
	if h.Status() != notificationsAvailable {
		t.Fatal("the zero value must say notifications are available")
	}
	h.NoteShowResult(fmt.Errorf("show: %w", mygo.ErrNotificationsDenied))
	if h.Status() != notificationsBlocked {
		t.Fatalf("status = %v after a denial, want blocked", h.Status())
	}
	h.NoteShowResult(errors.New("some other failure"))
	if h.Status() != notificationsBlocked {
		t.Fatal("an unrelated error must not change the status")
	}
	h.NoteShowResult(nil)
	if h.Status() != notificationsAvailable {
		t.Fatal("a shown notice means Windows allows them again")
	}
}

func TestNotificationHealthRecordsUnsupportedSystems(t *testing.T) {
	var probed, dropped notificationHealth
	probed.ProbeSupport(func() bool { return false })
	dropped.NoteUnsupported()
	for name, h := range map[string]*notificationHealth{"probe": &probed, "dropped notice": &dropped} {
		if h.Status() != notificationsUnsupported {
			t.Errorf("%s: status = %v, want unsupported", name, h.Status())
		}
	}
	var supported notificationHealth
	supported.ProbeSupport(func() bool { return true })
	if supported.Status() != notificationsAvailable {
		t.Fatal("a supported system was marked unsupported")
	}
}

func TestNotificationHealthLogsARefusalOnce(t *testing.T) {
	logged := captureLogs(t)
	var h notificationHealth
	for range 3 {
		h.NoteShowResult(mygo.ErrNotificationsDenied)
	}
	if got := strings.Count(logged.String(), "notifications are blocked by the system"); got != 1 {
		t.Fatalf("logged %d times, want once; log: %q", got, logged.String())
	}
}

func TestNotificationHealthIsNilSafeAndThreadSafe(t *testing.T) {
	var none *notificationHealth
	none.NoteUnsupported()
	none.NoteShowResult(mygo.ErrNotificationsDenied)
	none.ProbeSupport(func() bool { return false })
	if none.Status() != notificationsAvailable {
		t.Fatal("a nil health must say available")
	}
	var h notificationHealth
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.NoteShowResult(mygo.ErrNotificationsDenied)
			_ = h.Status()
			h.NoteShowResult(nil)
		}()
	}
	wg.Wait()
}

func TestNotifyDetailExplainsWhatToDo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader notificationStatusReader
		want   string
	}{
		{"unknown", nil, notifyOnFailureDetail},
		{"available", fakeStatusReader{notificationsAvailable}, notifyOnFailureDetail},
		{"blocked", fakeStatusReader{notificationsBlocked},
			"Windows is blocking notifications for OpenPrintCast. Allow them in Windows Settings > System > Notifications."},
		{"unsupported", fakeStatusReader{notificationsUnsupported}, "Notifications aren't available on this system."},
	} {
		if got := notifyDetail(tc.reader); got != tc.want {
			t.Errorf("%s: detail = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSettingsShowsWhyNotificationsAreBlocked(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status notificationStatus
		detail string
		hidden string
	}{
		{"blocked", notificationsBlocked, notifyBlockedDetail, notifyOnFailureDetail},
		{"unsupported", notificationsUnsupported, notifyUnsupportedDetail, notifyOnFailureDetail},
	} {
		app := newTestApp(t)
		app.tab = tabSettings
		app.notifyHealth = &notificationHealth{}
		switch tc.status {
		case notificationsBlocked:
			app.notifyHealth.NoteShowResult(mygo.ErrNotificationsDenied)
		default:
			app.notifyHealth.NoteUnsupported()
		}
		tt := ui.NewTester(app.view, 860, 620)
		if !tt.HasText(tc.detail) || tt.HasText(tc.hidden) {
			t.Errorf("%s: texts = %q", tc.name, tt.Texts())
		}
		snapshot(t, tt, "settings-notifications-"+tc.name)
	}
}
