package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/reach"
	"github.com/ilyasaftr/OpenPrintCast/internal/winfw"
)

// bannerView opens the Printers page of a rig whose first check has
// already answered, so the banner is on screen.
func bannerView(t *testing.T, answer reach.Assessment, w, h int) (*netRig, *ui.Tester) {
	t.Helper()
	rig := newNetRig(t, answer)
	tt := ui.NewTester(rig.app.view, w, h)
	runFirstCheck(tt, rig.app)
	return rig, tt
}

// runFirstCheck makes the server report listening and lets the check that
// follows run and answer.
func runFirstCheck(tt *ui.Tester, app *bridgeApp) {
	app.onServerListenChanged()
	tt.Frame()
	tt.Advance(netCheckStartDelay)
	tt.Frame()
}

func TestBannerTextsAndButtonsPerVerdict(t *testing.T) {
	tests := []struct {
		name    string
		answer  reach.Assessment
		texts   []string
		buttons []string
	}{
		{"public network", publicVerdict,
			[]string{"Other devices can't reach this PC", netPublicBody},
			[]string{"Open Network Settings", "Allow on This Network…", "Check Again", "Copy Diagnostics"}},
		{"no rule", noRuleVerdict,
			[]string{"Other devices can't reach this PC", "Windows Firewall is blocking port 631."},
			[]string{"Allow in Windows Firewall", "Check Again", "Copy Diagnostics"}},
		{"port in use", inUseVerdict,
			[]string{"OpenPrintCast can't accept connections", "Port 631 is already in use by cupsd. Quit that program, then check again."},
			[]string{"Check Again", "Copy Diagnostics"}},
		{"bind failed", reach.Assessment{State: reach.StateNotListening, Cause: reach.CauseBindFailed, Detail: "Windows did not allow the program to listen on the port"},
			[]string{"OpenPrintCast can't accept connections", "Couldn't start listening: Windows did not allow the program to listen on the port. Quit any program that uses port 631, then check again."},
			[]string{"Check Again", "Copy Diagnostics"}},
		{"rule of another program", reach.Assessment{State: reach.StateBlocked, Cause: reach.CauseBlockRule, Detail: "Block 631"},
			[]string{"Other devices can't reach this PC", "The Windows Firewall rule “Block 631” is blocking port 631. Turn that rule off in Windows Security, then check again."},
			[]string{"Check Again", "Copy Diagnostics"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, tt := bannerView(t, tc.answer, 860, 620)
			for _, text := range tc.texts {
				if !tt.HasText(text) {
					t.Errorf("missing %q; texts: %q", text, tt.Texts())
				}
			}
			for _, name := range tc.buttons {
				if _, ok := tt.AccessByRole(ui.RoleButton, name); !ok {
					t.Errorf("no button %q; texts: %q", name, tt.Texts())
				}
			}
			if _, ok := tt.AccessByRole(ui.RoleButton, "Details"); !ok && !tt.HasText("Details") {
				t.Error("no Details disclosure")
			}
		})
	}
}

func TestNoBannerWhenReachableOrUnknown(t *testing.T) {
	for name, answer := range map[string]reach.Assessment{"ok": okVerdict, "unknown": {Detail: "no PowerShell"}} {
		t.Run(name, func(t *testing.T) {
			_, tt := bannerView(t, answer, 860, 620)
			if tt.HasText(netUnreachableHeadline) || tt.HasText(netNotListeningHeadline) {
				t.Fatalf("banner shown for %s; texts: %q", name, tt.Texts())
			}
		})
	}
}

func TestBannerSnapshots(t *testing.T) {
	cases := map[string]reach.Assessment{"public": publicVerdict, "norule": noRuleVerdict, "notlistening": inUseVerdict}
	for name, answer := range cases {
		for _, size := range [][2]int{{860, 620}, {640, 460}} {
			t.Run(fmt.Sprintf("%s-%d", name, size[0]), func(t *testing.T) {
				_, tt := bannerView(t, answer, size[0], size[1])
				snapshot(t, tt, fmt.Sprintf("net-%s-%d", name, size[0]))
				tt.SetDark(true)
				snapshot(t, tt, fmt.Sprintf("net-%s-dark-%d", name, size[0]))
			})
		}
	}
}

func TestPublicSheetSnapshots(t *testing.T) {
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		rig, tt := bannerView(t, publicVerdict, size[0], size[1])
		if err := tt.Click("Allow on This Network…"); err != nil {
			t.Fatal(err)
		}
		if !rig.app.network().confirmOpen || !tt.HasText(netSheetBody) {
			t.Fatalf("no confirmation sheet; texts: %q", tt.Texts())
		}
		snapshot(t, tt, fmt.Sprintf("net-sheet-%d", size[0]))
		tt.SetDark(true)
		snapshot(t, tt, fmt.Sprintf("net-sheet-dark-%d", size[0]))
	}
}

func TestBannerIsAccessibleAndAnnouncedOnce(t *testing.T) {
	rig, tt := bannerView(t, publicVerdict, 860, 620)
	want := netUnreachableHeadline + ". " + netPublicBody
	if got := tt.Announcements(); !slices.Equal(got, []string{want}) {
		t.Fatalf("announced %q, want one announcement %q", got, want)
	}
	if _, ok := tt.Access(want); !ok {
		t.Fatalf("the banner has no accessible name %q", want)
	}
	for _, name := range []string{"Open Network Settings", "Allow on This Network…", "Check Again", "Copy Diagnostics"} {
		info, ok := tt.AccessByRole(ui.RoleButton, name)
		if !ok || info.Disabled {
			t.Errorf("button %q: %+v ok=%v", name, info, ok)
		}
	}
	tt.Frame()
	tt.Frame()
	if got := tt.Announcements(); len(got) != 0 {
		t.Fatalf("announced again: %q", got)
	}
	// Fixed, then broken again: it is a new appearance.
	rig.checker.answer = okVerdict
	rig.app.requestNetCheck(tt.Now(), false)
	tt.Frame()
	tt.Frame()
	rig.checker.answer = publicVerdict
	rig.app.requestNetCheck(tt.Now(), false)
	tt.Frame()
	tt.Frame()
	if got := tt.Announcements(); !slices.Equal(got, []string{want}) {
		t.Fatalf("announced %q when the problem came back", got)
	}
}

func TestBannerGoesAwayWhenFixedAndCannotBeDismissed(t *testing.T) {
	rig, tt := bannerView(t, noRuleVerdict, 860, 620)
	tt.Key(0, ui.KeyEscape)
	if !tt.HasText(netUnreachableHeadline) {
		t.Fatal("Escape dismissed the banner")
	}
	rig.checker.answer = okVerdict
	tt.Advance(netCheckPollEvery)
	tt.Frame()
	if tt.HasText(netUnreachableHeadline) || tt.HasText(netNeedsAttentionPill) {
		t.Fatalf("the banner stayed after the problem was fixed; texts: %q", tt.Texts())
	}
}

func TestStatusPillWarnsWithTextAndSymbol(t *testing.T) {
	_, tt := bannerView(t, noRuleVerdict, 860, 620)
	if !tt.HasText(netNeedsAttentionPill) {
		t.Fatalf("pill text missing; texts: %q", tt.Texts())
	}
	if _, ok := tt.Access("Status: " + netNeedsAttentionPill); !ok {
		t.Fatal("the pill's accessible name does not carry the warning")
	}
	_, healthy := bannerView(t, okVerdict, 860, 620)
	if healthy.HasText(netNeedsAttentionPill) || !healthy.HasText("Sharing 1 of 2") {
		t.Fatalf("a reachable PC shows the warning; texts: %q", healthy.Texts())
	}
}

func TestStatusPillKeepsTheOtherWarnings(t *testing.T) {
	rig := newNetRig(t, noRuleVerdict)
	rig.app.paused = true
	m := rig.app.network()
	m.state.Request(t0, false)
	m.state.Finish(noRuleVerdict)
	m.state.Tick(t0)
	if text, lvl := rig.app.status(); text != "Paused" || lvl != levelWarn {
		t.Fatalf("status = %q %v", text, lvl)
	}
}

func TestButtonsCallTheSeams(t *testing.T) {
	t.Run("open network settings", func(t *testing.T) {
		rig, tt := bannerView(t, publicVerdict, 860, 620)
		if err := tt.Click("Open Network Settings"); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(rig.opener.urls, []string{"ms-settings:network-status"}) {
			t.Fatalf("opened %q", rig.opener.urls)
		}
	})
	t.Run("open network settings fails", func(t *testing.T) {
		rig, tt := bannerView(t, publicVerdict, 860, 620)
		rig.opener.err = fmt.Errorf("no shell")
		_ = tt.Click("Open Network Settings")
		if !tt.HasText(networkSettingsText) {
			t.Fatalf("no toast; texts: %q", tt.Texts())
		}
	})
	t.Run("allow in windows firewall", func(t *testing.T) {
		rig, tt := bannerView(t, noRuleVerdict, 860, 620)
		rig.fixer.onApply = func() { rig.checker.answer = okVerdict }
		if err := tt.Click("Allow in Windows Firewall"); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		if !slices.Equal(rig.fixer.calls, []bool{false}) {
			t.Fatalf("fixer calls = %v, want one call without Public", rig.fixer.calls)
		}
		if !tt.HasText(fixSucceededText) || tt.HasText(netUnreachableHeadline) {
			t.Fatalf("no success toast or banner still there; texts: %q", tt.Texts())
		}
	})
	t.Run("check again", func(t *testing.T) {
		rig, tt := bannerView(t, noRuleVerdict, 860, 620)
		before := rig.checker.calls
		_ = tt.Click("Check Again")
		if rig.checker.calls != before+1 {
			t.Fatalf("calls = %d, want %d", rig.checker.calls, before+1)
		}
	})
}

func TestPublicNetworkNeedsAConfirmation(t *testing.T) {
	rig, tt := bannerView(t, publicVerdict, 860, 620)
	_ = tt.Click("Allow on This Network…")
	if len(rig.fixer.calls) != 0 {
		t.Fatal("the fix ran before the user confirmed")
	}
	for _, text := range []string{netSheetTitle, "Public networks are shared with strangers. Anyone on this network could reach OpenPrintCast."} {
		if !tt.HasText(text) {
			t.Fatalf("sheet lacks %q; texts: %q", text, tt.Texts())
		}
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if len(rig.fixer.calls) != 0 || rig.app.network().confirmOpen {
		t.Fatalf("Cancel ran the fix or left the sheet open: %v", rig.fixer.calls)
	}
	_ = tt.Click("Allow on This Network…")
	rig.fixer.onApply = func() { rig.checker.answer = okVerdict }
	if err := tt.Click("Allow"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !slices.Equal(rig.fixer.calls, []bool{true}) {
		t.Fatalf("fixer calls = %v, want one call with Public", rig.fixer.calls)
	}
	if !tt.HasText(fixSucceededText) {
		t.Fatalf("no success toast; texts: %q", tt.Texts())
	}
}

func TestFixOutcomesAreToasted(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		after   reach.Assessment
		toast   string
		recheck bool
	}{
		{"declined", winfw.ErrCancelled, noRuleVerdict, fixCancelledText, false},
		{"failed", fmt.Errorf("the firewall setup failed (exit code 1)\nmore"), noRuleVerdict, fixFailedText + "the firewall setup failed (exit code 1)", false},
		{"updated but still blocked", nil, publicVerdict, fixStillBlockedText, true},
		{"updated", nil, okVerdict, fixSucceededText, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rig, tt := bannerView(t, noRuleVerdict, 860, 620)
			rig.fixer.err = tc.err
			rig.fixer.onApply = func() { rig.checker.answer = tc.after }
			calls := rig.checker.calls
			_ = tt.Click("Allow in Windows Firewall")
			tt.Frame()
			if !tt.HasText(tc.toast) {
				t.Fatalf("no toast %q; texts: %q", tc.toast, tt.Texts())
			}
			if rechecked := rig.checker.calls > calls; rechecked != tc.recheck {
				t.Fatalf("rechecked = %v, want %v", rechecked, tc.recheck)
			}
		})
	}
}

func TestFixButtonShowsBusyAtOnce(t *testing.T) {
	rig, tt := bannerView(t, noRuleVerdict, 860, 620)
	held := &heldScan{}
	rig.app.network().runner = held.run
	_ = tt.Click("Allow in Windows Firewall")
	info, ok := tt.AccessByRole(ui.RoleButton, "Allow in Windows Firewall")
	if !ok || !info.Busy || !info.Disabled {
		t.Fatalf("button after the click: %+v ok=%v", info, ok)
	}
	_ = tt.Click("Allow in Windows Firewall")
	if !held.pending() || len(rig.fixer.calls) != 0 {
		t.Fatal("the work did not wait for its runner")
	}
	held.finish()
	if len(rig.fixer.calls) != 1 {
		t.Fatalf("fixer calls = %v", rig.fixer.calls)
	}
}

func TestCheckAgainShowsBusyForTheMinimumTime(t *testing.T) {
	rig, tt := bannerView(t, inUseVerdict, 860, 620)
	held := &heldScan{}
	rig.app.network().runner = held.run
	_ = tt.Click("Check Again")
	info, _ := tt.AccessByRole(ui.RoleButton, "Check Again")
	if !info.Busy || !info.Disabled {
		t.Fatalf("not busy right after the click: %+v", info)
	}
	held.finish()
	tt.Frame()
	if info, _ = tt.AccessByRole(ui.RoleButton, "Check Again"); !info.Busy {
		t.Fatal("the busy state ended at once")
	}
	tt.Advance(netCheckMinBusy)
	if info, _ = tt.AccessByRole(ui.RoleButton, "Check Again"); info.Busy {
		t.Fatal("still busy after the minimum time")
	}
}

func TestCopyDiagnosticsPutsPlainTextOnTheClipboard(t *testing.T) {
	rig, tt := bannerView(t, publicVerdict, 860, 620)
	rig.app.shared.ListenStatus()
	if err := tt.Click("Copy Diagnostics"); err != nil {
		t.Fatal(err)
	}
	if len(rig.clipboard.texts) != 1 || !tt.HasText(diagnosticsCopiedMsg) {
		t.Fatalf("clipboard = %q; texts: %q", rig.clipboard.texts, tt.Texts())
	}
	text := rig.clipboard.texts[0]
	for _, want := range []string{"Version: dev", "Computer: PC", "Address: 192.168.1.5", "Port: 631", "Server: not started yet",
		"Active network profiles: Public", "Matching firewall rules: none found", "Result: blocked (public network), profile Public"} {
		if !strings.Contains(text, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, text)
		}
	}
	for _, private := range []string{"Agung", "invoice.pdf", "EPSON", "Office"} {
		if strings.Contains(text, private) {
			t.Errorf("diagnostics contain %q:\n%s", private, text)
		}
	}
}

func TestDetailsShowTheDiagnostics(t *testing.T) {
	rig, tt := bannerView(t, noRuleVerdict, 860, 620)
	if tt.HasText("Active network profiles: Private") {
		t.Fatal("details are open from the start")
	}
	if err := tt.Click("Details"); err != nil {
		t.Fatal(err)
	}
	if !rig.app.network().detailsOpen || !strings.Contains(strings.Join(tt.Texts(), "\n"), "Active network profiles: Private") {
		t.Fatalf("details not shown; texts: %q", tt.Texts())
	}
	tt.Advance(time.Second) // the panel grows into view
	snapshot(t, tt, "net-details")
}

func TestSettingsShowsTheNetworkCheckRow(t *testing.T) {
	rig := newNetRig(t, okVerdict)
	rig.app.tab, rig.app.settingsSection = tabSettings, sectionNetwork
	tt := ui.NewTester(rig.app.view, 860, 620)
	for _, want := range []string{"Connection", "Network check", netNeverCheckedMsg} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q; texts: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "net-settings-unchecked")
	clickSettingsButton(t, tt, "Check Again")
	tt.Advance(netCheckMinBusy)
	if !tt.HasText(netReachableText) {
		t.Fatalf("no result; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "net-settings-ok")
	rig.checker.answer = publicVerdict
	clickSettingsButton(t, tt, "Check Again")
	tt.Advance(netCheckMinBusy)
	if !tt.HasText(netUnreachableHeadline) {
		t.Fatalf("no problem text; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "net-settings-problem")
	clickSettingsButton(t, tt, "Copy Diagnostics")
	if len(rig.clipboard.texts) != 1 {
		t.Fatalf("clipboard = %q", rig.clipboard.texts)
	}
	tt.SetDark(true)
	tt.SetSize(640, 460)
	snapshot(t, tt, "net-settings-problem-dark-640")
}

func TestSettingsRowSaysCouldntCheckForUnknown(t *testing.T) {
	rig := newNetRig(t, reach.Assessment{Detail: "no PowerShell"})
	rig.app.tab, rig.app.settingsSection = tabSettings, sectionNetwork
	tt := ui.NewTester(rig.app.view, 860, 620)
	clickSettingsButton(t, tt, "Check Again")
	tt.Advance(netCheckMinBusy)
	if !tt.HasText(netUnknownText) {
		t.Fatalf("texts: %q", tt.Texts())
	}
}

// clickSettingsButton scrolls the Settings page (in a default-size window)
// to a button and clicks it.
func clickSettingsButton(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	info := scrollToControl(t, tt, ui.RoleButton, name, windowHeight)
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
}
