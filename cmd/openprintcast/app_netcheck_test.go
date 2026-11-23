package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
	"github.com/ilyasaftr/OpenPrintCast/internal/reach"
	"github.com/ilyasaftr/OpenPrintCast/internal/winfw"
)

func TestFirstCheckRunsAboutThreeSecondsAfterTheServerReports(t *testing.T) {
	rig := newNetRig(t, publicVerdict)
	tt := ui.NewTester(rig.app.view, 860, 620)
	tt.Advance(time.Minute)
	if rig.checker.calls != 0 {
		t.Fatal("a check ran before the server reported anything")
	}
	rig.app.onServerListenChanged()
	tt.Frame()
	tt.Advance(netCheckStartDelay - time.Second)
	if rig.checker.calls != 0 || tt.HasText(netUnreachableHeadline) {
		t.Fatal("the check ran before the delay")
	}
	tt.Advance(time.Second)
	tt.Frame()
	if rig.checker.calls != 1 || !tt.HasText(netUnreachableHeadline) {
		t.Fatalf("calls = %d; texts: %q", rig.checker.calls, tt.Texts())
	}
}

func TestScanAgainChecksTheNetworkToo(t *testing.T) {
	rig := newNetRig(t, noRuleVerdict)
	scanner, held := &fakeScanner{printers: listed(epson, pdf)}, &heldScan{}
	rig.app.scanner, rig.app.runScan = scanner, held.run
	tt := ui.NewTester(rig.app.view, 860, 620)
	if err := tt.Click("Scan Again"); err != nil {
		t.Fatal(err)
	}
	if rig.checker.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", rig.checker.calls)
	}
	tt.Frame()
	if !tt.HasText(netUnreachableHeadline) {
		t.Fatalf("no banner after Scan Again; texts: %q", tt.Texts())
	}
}

func TestPollsEveryMinuteWhileTheBannerAndWindowAreVisible(t *testing.T) {
	app := newNetRig(t, noRuleVerdict)
	tt := ui.NewTester(app.app.view, 860, 620)
	runFirstCheck(tt, app.app)
	base := app.checker.calls
	tt.Advance(netCheckPollEvery - time.Second)
	if app.checker.calls != base {
		t.Fatal("polled before a minute")
	}
	tt.Advance(time.Second)
	if app.checker.calls != base+1 {
		t.Fatalf("calls = %d, want %d after a minute", app.checker.calls, base+1)
	}
	tt.Frame() // the window redraws when the answer arrives
	tt.Advance(netCheckPollEvery + time.Second)
	if app.checker.calls != base+2 {
		t.Fatalf("calls = %d, want %d after two minutes", app.checker.calls, base+2)
	}
}

func TestDoesNotPollWithoutAProblemOrInTheTray(t *testing.T) {
	healthy := newNetRig(t, okVerdict)
	tt := ui.NewTester(healthy.app.view, 860, 620)
	runFirstCheck(tt, healthy.app)
	base := healthy.checker.calls
	tt.Advance(10 * netCheckPollEvery)
	if healthy.checker.calls != base {
		t.Fatalf("polled %d times although all is well", healthy.checker.calls-base)
	}

	broken := newNetRig(t, noRuleVerdict)
	tt = ui.NewTester(broken.app.view, 860, 620)
	runFirstCheck(tt, broken.app)
	base = broken.checker.calls
	broken.app.network().state.SetVisible(false) // hidden in the tray
	tt.Advance(10 * netCheckPollEvery)
	if broken.checker.calls != base {
		t.Fatalf("polled %d times while hidden", broken.checker.calls-base)
	}
	broken.app.network().state.SetVisible(true) // shown again after ten minutes
	tt.Frame()
	if broken.checker.calls != base+1 {
		t.Fatalf("calls = %d, want one check when the window came back", broken.checker.calls)
	}
}

func TestShowingTheWindowAfterFiveMinutesChecksAgain(t *testing.T) {
	rig := newNetRig(t, okVerdict)
	tt := ui.NewTester(rig.app.view, 860, 620)
	runFirstCheck(tt, rig.app)
	base := rig.checker.calls
	state := rig.app.network().state
	state.SetVisible(false)
	tt.Advance(time.Minute)
	state.SetVisible(true)
	tt.Frame()
	if rig.checker.calls != base {
		t.Fatal("checked again after one minute")
	}
	state.SetVisible(false)
	tt.Advance(netCheckStaleAfter)
	state.SetVisible(true)
	tt.Frame()
	if rig.checker.calls != base+1 {
		t.Fatalf("calls = %d, want %d", rig.checker.calls, base+1)
	}
}

func TestChecksDoNotOverlapThroughTheApp(t *testing.T) {
	rig := newNetRig(t, noRuleVerdict)
	held := &heldScan{}
	rig.app.network().runner = held.run
	if !rig.app.requestNetCheck(t0, false) || rig.app.requestNetCheck(t0, true) {
		t.Fatal("a second check started over the first")
	}
}

func TestNoCheckerMeansNothingHappens(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	app.onServerListenChanged()
	tt.Advance(time.Hour)
	if app.requestNetCheck(t0, true) || app.netProblem() || tt.HasText(netUnreachableHeadline) {
		t.Fatal("a check ran without a checker")
	}
}

func TestVerdictChangesAreLoggedOnceNotOnEveryPoll(t *testing.T) {
	logs := captureLogs(t)
	rig := newNetRig(t, noRuleVerdict)
	tt := ui.NewTester(rig.app.view, 860, 620)
	runFirstCheck(tt, rig.app)
	for i := 0; i < 5; i++ {
		tt.Advance(netCheckPollEvery + time.Second)
		tt.Frame() // the window redraws when the answer arrives
	}
	if rig.checker.calls < 5 {
		t.Fatalf("only %d checks ran", rig.checker.calls)
	}
	if n := strings.Count(logs.String(), "network check found a problem"); n != 1 {
		t.Fatalf("problem logged %d times:\n%s", n, logs.String())
	}
	if !strings.Contains(logs.String(), "level=WARN") {
		t.Fatal("a problem is not logged as a warning")
	}
	rig.checker.answer = okVerdict
	for i := 0; i < 2; i++ {
		tt.Advance(netCheckPollEvery + time.Second)
		tt.Frame()
	}
	if n := strings.Count(logs.String(), "msg=\"network check\""); n != 1 {
		t.Fatalf("recovery logged %d times:\n%s", n, logs.String())
	}
	if strings.Contains(logs.String(), "Agung") {
		t.Fatal("the log names a user")
	}
}

func TestElevatedProcessFixesAProblemSilentlyOnce(t *testing.T) {
	rig := newNetRig(t, noRuleVerdict)
	rig.fixer.elevated = true
	tt := ui.NewTester(rig.app.view, 860, 620)
	runFirstCheck(tt, rig.app)
	if !slices.Equal(rig.fixer.calls, []bool{false}) {
		t.Fatalf("fixer calls = %v, want one call without Public", rig.fixer.calls)
	}
	if tt.HasText(fixSucceededText) || tt.HasText(fixFailedText) {
		t.Fatalf("the silent fix showed a toast; texts: %q", tt.Texts())
	}
	tt.Advance(netCheckPollEvery) // still blocked: no second attempt
	if len(rig.fixer.calls) != 1 {
		t.Fatalf("fixer calls = %v, want exactly one per run", rig.fixer.calls)
	}
}

func TestElevatedProcessNeverFixesPublicNetworksOrUnknownProblems(t *testing.T) {
	for name, answer := range map[string]reach.Assessment{"public": publicVerdict, "in use": inUseVerdict, "ok": okVerdict} {
		rig := newNetRig(t, answer)
		rig.fixer.elevated = true
		tt := ui.NewTester(rig.app.view, 860, 620)
		runFirstCheck(tt, rig.app)
		if len(rig.fixer.calls) != 0 {
			t.Errorf("%s: the elevated process changed the firewall: %v", name, rig.fixer.calls)
		}
	}
}

func TestUnelevatedProcessDoesNothingButCheck(t *testing.T) {
	rig := newNetRig(t, noRuleVerdict)
	tt := ui.NewTester(rig.app.view, 860, 620)
	runFirstCheck(tt, rig.app)
	if len(rig.fixer.calls) != 0 || !tt.HasText(netUnreachableHeadline) {
		t.Fatalf("fixer calls = %v; texts: %q", rig.fixer.calls, tt.Texts())
	}
}

func TestSilentFixFailureOnlyLogs(t *testing.T) {
	logs := captureLogs(t)
	rig := newNetRig(t, noRuleVerdict)
	rig.fixer.elevated, rig.fixer.err = true, errors.New("netsh failed")
	tt := ui.NewTester(rig.app.view, 860, 620)
	runFirstCheck(tt, rig.app)
	if tt.HasText(fixFailedText) || !strings.Contains(logs.String(), "firewall update failed") {
		t.Fatalf("texts: %q\nlog:\n%s", tt.Texts(), logs.String())
	}
}

func TestOnlyOneFixRunsAtATime(t *testing.T) {
	rig := newNetRig(t, noRuleVerdict)
	held := &heldScan{}
	rig.app.network().runner = held.run
	rig.app.applyFirewallFix(false, false)
	rig.app.applyFirewallFix(true, false)
	held.finish()
	if !slices.Equal(rig.fixer.calls, []bool{false}) {
		t.Fatalf("fixer calls = %v", rig.fixer.calls)
	}
}

func TestPanickingCheckerOrFixerDoNotCrash(t *testing.T) {
	if got := safeCheck(context.Background(), panicChecker{}); got.State != reach.StateUnknown || got.Problem() {
		t.Fatalf("got %v", got)
	}
	if err := safeApply(context.Background(), &panicFixer{}, false); err == nil {
		t.Fatal("a panic in the fixer was not turned into an error")
	}
}

type panicChecker struct{}

func (panicChecker) Check(context.Context) reach.Assessment { panic("boom") }

type panicFixer struct{ fakeFixer }

func (*panicFixer) Apply(context.Context, bool) error { panic("boom") }

func TestChecksAndFixesRunUnderTheAppContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan context.Context, 2)
	app := newTestApp(t)
	m := app.network()
	m.ctx, m.runner = ctx, inlineRun
	m.checker = ctxChecker{got}
	m.fixer = ctxFixer{got}
	app.requestNetCheck(t0, false)
	app.applyFirewallFix(false, false)
	cancel()
	for i := 0; i < 2; i++ {
		if (<-got).Err() == nil {
			t.Fatal("the work does not stop when the app quits")
		}
	}
}

type ctxChecker struct{ got chan context.Context }

func (c ctxChecker) Check(ctx context.Context) reach.Assessment {
	c.got <- ctx
	return reach.Assessment{}
}

type ctxFixer struct{ got chan context.Context }

func (ctxFixer) Elevated() bool { return false }
func (c ctxFixer) Apply(ctx context.Context, _ bool) error {
	c.got <- ctx
	return nil
}

func TestReachListenAdaptsTheServerState(t *testing.T) {
	shared := bridgenet.NewSharedState(nil, nil)
	if got := reachListen(shared)(); got.Attempted || got.Listening {
		t.Fatalf("a fresh server reads as %+v", got)
	}
}

func TestDiagnosticsTextHasOnlyNonPersonalFacts(t *testing.T) {
	facts := diagnosticsFacts{Version: "1.2.3", Host: "OFFICE-PC", IP: "10.0.0.7", Port: 631, Server: "listening on [::]:631"}
	got := diagnosticsText(facts, reach.Assessment{
		State: reach.StateBlocked, Cause: reach.CauseBlockRule, Profile: "Private", ActiveProfiles: []string{"Private", "Public"},
		Rules: []string{"OpenPrintCast (IPP)", "openprintcast.exe"}, Detail: "openprintcast.exe",
	}, true)
	want := "OpenPrintCast network diagnostics\n" +
		"Version: 1.2.3\nComputer: OFFICE-PC\nAddress: 10.0.0.7\nPort: 631\nServer: listening on [::]:631\n" +
		"Active network profiles: Private, Public\n" +
		"Matching firewall rules: OpenPrintCast (IPP), openprintcast.exe\n" +
		"Result: blocked (block rule), profile Private, openprintcast.exe\n"
	if got != want {
		t.Fatalf("diagnostics:\n%s\nwant:\n%s", got, want)
	}
	if text := diagnosticsText(facts, reach.Assessment{}, false); !strings.Contains(text, "Result: "+netNeverCheckedMsg) || !strings.Contains(text, "Active network profiles: unknown") {
		t.Fatalf("unchecked diagnostics:\n%s", text)
	}
}

func TestDescribeNetworkOnlyForProblems(t *testing.T) {
	for name, a := range map[string]reach.Assessment{"ok": okVerdict, "unknown": {Cause: reach.CauseUnknown}, "none": {}} {
		if _, ok := describeNetwork(a); ok {
			t.Errorf("%s has a banner", name)
		}
	}
	b, ok := describeNetwork(publicVerdict)
	if !ok || b.Primary != reach.FixOpenNetworkSettings || b.Alt != reach.FixAllowOnPublic || b.Headline != netUnreachableHeadline {
		t.Fatalf("banner = %+v ok=%v", b, ok)
	}
}

func TestNetworkStatusText(t *testing.T) {
	tests := []struct {
		busy, has bool
		a         reach.Assessment
		want      string
	}{
		{true, true, okVerdict, netCheckingText},
		{false, false, reach.Assessment{}, netNeverCheckedMsg},
		{false, true, okVerdict, netReachableText},
		{false, true, noRuleVerdict, netUnreachableHeadline},
		{false, true, inUseVerdict, netNotListeningHeadline},
		{false, true, reach.Assessment{Detail: "x"}, netUnknownText},
	}
	for _, tc := range tests {
		if got := networkStatusText(tc.busy, tc.has, tc.a); got != tc.want {
			t.Errorf("networkStatusText(%v, %v, %v) = %q, want %q", tc.busy, tc.has, tc.a, got, tc.want)
		}
	}
}

func TestRunCommandLine(t *testing.T) {
	var calls []bool
	previous := setupFirewall
	t.Cleanup(func() { setupFirewall = previous })
	setupFirewall = func(allowPublic bool) error {
		calls = append(calls, allowPublic)
		return nil
	}
	tests := []struct {
		args    []string
		handled bool
		code    int
		calls   []bool
	}{
		{nil, false, exitOK, nil},
		{[]string{"--something-else"}, false, exitOK, nil},
		{[]string{"--allow-public"}, false, exitOK, nil},
		{[]string{"--setup-firewall"}, true, exitOK, []bool{false}},
		{[]string{"--setup-firewall", "--allow-public"}, true, exitOK, []bool{true}},
		{[]string{"--setup-firewall", "--nonsense"}, true, exitBadUsage, nil},
	}
	for _, tc := range tests {
		calls = nil
		handled, code := runCommandLine(tc.args)
		if handled != tc.handled || code != tc.code || !slices.Equal(calls, tc.calls) {
			t.Errorf("runCommandLine(%q) = %v, %d with calls %v; want %v, %d, %v", tc.args, handled, code, calls, tc.handled, tc.code, tc.calls)
		}
	}
	setupFirewall = func(bool) error { return errors.New("netsh failed") }
	if handled, code := runCommandLine([]string{"--setup-firewall"}); !handled || code != exitFailed {
		t.Errorf("a failed setup returned %v, %d", handled, code)
	}
}

func TestElevatedFlagsMatchTheParser(t *testing.T) {
	if got := winfw.ParseCommand([]string{winfw.SetupFlag, winfw.AllowPublicFlag}); !got.Setup || !got.AllowPublic {
		t.Fatalf("parser = %+v", got)
	}
}

// TestMainHandlesTheCommandLineBeforeTheLock keeps the elevated copy from
// being turned away by the single-instance lock or starting the UI.
func TestMainHandlesTheCommandLineBeforeTheLock(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	command, lock := strings.Index(text, "runCommandLine("), strings.Index(text, "RequestSingleInstanceLock(")
	if command < 0 || lock < 0 || command > lock {
		t.Fatalf("runCommandLine at %d, lock at %d: the command line must come first", command, lock)
	}
}
