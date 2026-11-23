package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
	"github.com/ilyasaftr/OpenPrintCast/internal/reach"
	"github.com/ilyasaftr/OpenPrintCast/internal/winfw"
)

// networkSettingsURI opens the Windows Network status page.
const networkSettingsURI = "ms-settings:network-status"

// Toast wording of the firewall fix and the other network actions.
const (
	fixCancelledText     = "The firewall change was cancelled."
	fixSucceededText     = "Firewall updated. Other devices can reach this PC."
	fixStillBlockedText  = "The firewall was updated, but other devices still can't reach this PC."
	fixFailedText        = "Couldn't update the firewall: "
	networkSettingsText  = "Couldn't open Network Settings."
	diagnosticsCopiedMsg = "Diagnostics copied"
)

// networkChecker runs one reachability check. It may be slow, so it runs
// off the UI thread; reach.Checker is the production one.
type networkChecker interface {
	Check(ctx context.Context) reach.Assessment
}

// firewallFixer changes Windows Firewall so other devices can connect.
// Apply may show the administrator prompt and blocks until it is done;
// winfw.Fixer is the production one.
type firewallFixer interface {
	Elevated() bool
	Apply(ctx context.Context, allowPublic bool) error
}

// clipboardWriter puts plain text on the clipboard; mygo.Clipboard is the
// production one.
type clipboardWriter interface {
	WriteText(text string)
}

// networkModel is the UI-owned state of the reachability check and of the
// firewall fix. Only the UI thread touches it.
type networkModel struct {
	state *netCheckState

	// checker, fixer, clipboard and runner are the seams; a nil checker
	// means no check is possible, a nil clipboard the system one and a nil
	// runner a goroutine.
	checker   networkChecker
	fixer     firewallFixer
	clipboard clipboardWriter
	runner    scanRunner
	// ctx ends the check and the fix when the app quits.
	ctx context.Context

	fixing       bool
	afterFix     bool
	autoFixTried bool
	confirmOpen  bool
	detailsOpen  bool
	announced    string
	lastVerdict  string
}

// network is the app's check model, created on first use.
func (a *bridgeApp) network() *networkModel {
	if a.net == nil {
		a.net = &networkModel{state: newNetCheckState(), ctx: context.Background()}
	}
	return a.net
}

// newNetworkChecker is the production checker over the server's state.
func newNetworkChecker(listen func() reach.Listen, port int) networkChecker {
	return reach.Checker{Collector: reach.NewCollector(), Listen: listen, LocalIP: bridgenet.OutboundIP, Port: port}
}

// setupNetworkCheck wires the production checker, fixer and clipboard, and
// makes the server's listening reports trigger a check.
func (a *bridgeApp) setupNetworkCheck() {
	m := a.network()
	m.checker = newNetworkChecker(reachListen(a.shared), bridgenet.DefaultPort)
	m.fixer = winfw.Fixer{Port: bridgenet.DefaultPort}
	m.clipboard = mygo.Clipboard
	a.shared.ObserveListen(func(bridgenet.ListenStatus) { mygo.RunOnMain(a.onServerListenChanged) })
}

// onServerListenChanged runs on the UI thread when the server started
// listening or failed to; the check follows shortly.
func (a *bridgeApp) onServerListenChanged() {
	a.network().state.NoteServerEvent()
	a.invalidateWindow()
}

// netProblem reports whether the latest check found a problem to show.
func (a *bridgeApp) netProblem() bool { return a.network().state.Problem() }

// netTick runs once a frame on the UI thread: it moves the check on to now,
// starts what is due and asks for the frame that has more to do.
func (a *bridgeApp) netTick(c *ui.Context) {
	m := a.network()
	if m.checker == nil {
		return
	}
	now := c.Now()
	out := m.state.Tick(now)
	if out.Revealed {
		a.onNetworkResult(m.state.Result())
	}
	if out.Start {
		a.launchNetCheck()
	}
	if !m.state.Problem() {
		m.announced = ""
	}
	if wake := m.state.NextWake(now); wake > 0 {
		c.After(wake)
	}
}

// requestNetCheck starts a check at now unless one is running; manual
// keeps the busy state up long enough to be seen.
func (a *bridgeApp) requestNetCheck(now time.Time, manual bool) bool {
	m := a.network()
	if m.checker == nil || !m.state.Request(now, manual) {
		return false
	}
	a.launchNetCheck()
	return true
}

// launchNetCheck runs the checker off the UI thread and hands its answer
// back to the state on it.
func (a *bridgeApp) launchNetCheck() {
	m := a.network()
	var answer reach.Assessment
	checker, ctx := m.checker, m.ctx
	m.run()(func() { answer = safeCheck(ctx, checker) }, func() {
		m.state.Finish(answer)
		a.invalidateWindow()
	})
}

// run is the runner in use: the injected one, else a goroutine.
func (m *networkModel) run() scanRunner {
	if m.runner != nil {
		return m.runner
	}
	return runOffUIThread
}

// safeCheck turns a panic of the checker into an unknown answer, so a
// fault cannot take the app down from its goroutine.
func safeCheck(ctx context.Context, checker networkChecker) (answer reach.Assessment) {
	defer func() {
		if r := recover(); r != nil {
			answer = reach.Assessment{Detail: fmt.Sprintf("the network check panicked: %v", r)}
		}
	}()
	return checker.Check(ctx)
}

// Without a firewall exception Windows drops inbound mDNS/IPP and iOS sees
// nothing; an elevated process closes that gap itself, as the best-effort
// startup step used to.
//
// onNetworkResult handles a newly shown answer: it logs a change of the
// verdict once, tells the user the outcome of a fix, and applies the fix
// silently once when this process may already change the firewall.
func (a *bridgeApp) onNetworkResult(res reach.Assessment) {
	m := a.network()
	a.logVerdictChange(res)
	if m.afterFix {
		m.afterFix = false
		a.postFixOutcome(res)
	}
	if res.State == reach.StateBlocked && res.Fix == reach.FixAllowFirewall && !m.autoFixTried &&
		m.fixer != nil && m.fixer.Elevated() {
		m.autoFixTried = true
		a.applyFirewallFix(false, true)
	}
}

// logVerdictChange logs the verdict when it differs from the last one
// logged, never on every poll.
func (a *bridgeApp) logVerdictChange(res reach.Assessment) {
	m := a.network()
	verdict := res.String()
	if verdict == m.lastVerdict {
		return
	}
	m.lastVerdict = verdict
	log := componentLogger(componentFirewall)
	if res.Problem() {
		log.Warn("network check found a problem", "state", res.State, "cause", res.Cause, "profile", res.Profile,
			"detail", res.Detail, "owner", res.Owner, "rules", strings.Join(res.Rules, ", "))
		return
	}
	log.Info("network check", "state", res.State, "profiles", strings.Join(res.ActiveProfiles, ", "), "detail", res.Detail)
}

// postFixOutcome says whether the fix worked, judged by the re-check.
func (a *bridgeApp) postFixOutcome(res reach.Assessment) {
	if res.State == reach.StateOK {
		a.notices.Post(ui.Toast{ID: fixSucceededText, Title: fixSucceededText, Type: "success"})
		return
	}
	toast := ui.Toast{ID: fixStillBlockedText, Title: fixStillBlockedText, Type: "error", Timeout: errorNoticeTimeout}
	if b, ok := describeNetwork(res); ok {
		toast.Description = b.Body
	}
	a.notices.Post(toast)
}

// applyFirewallFix runs the fix off the UI thread and applies its result
// on it. silent is the automatic attempt of an elevated process: it only
// logs. It does nothing while a fix is running.
func (a *bridgeApp) applyFirewallFix(allowPublic, silent bool) {
	m := a.network()
	if m.fixer == nil || m.fixing {
		return
	}
	m.fixing = true
	var err error
	fixer, ctx := m.fixer, m.ctx
	m.run()(func() { err = safeApply(ctx, fixer, allowPublic) }, func() { a.finishFirewallFix(err, silent) })
}

// safeApply runs the fixer and turns a panic into an error.
func safeApply(ctx context.Context, fixer firewallFixer, allowPublic bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("the firewall fix panicked: %v", r)
		}
	}()
	return fixer.Apply(ctx, allowPublic)
}

// finishFirewallFix reports the fix's result and re-checks.
func (a *bridgeApp) finishFirewallFix(err error, silent bool) {
	m := a.network()
	m.fixing = false
	log := componentLogger(componentFirewall)
	switch {
	case err == nil:
		log.Info("firewall rules updated", "silent", silent)
		m.afterFix = !silent
		a.requestNetCheck(a.currentTime(), false)
	case isCancelled(err):
		log.Info("firewall change cancelled by the user")
		a.notices.Post(ui.Toast{ID: fixCancelledText, Title: fixCancelledText})
	default:
		log.Error("firewall update failed", "err", err, "silent", silent)
		if !silent {
			a.notices.PostError(fixFailedText + firstLine(err.Error()))
		}
	}
	a.invalidateWindow()
}

func isCancelled(err error) bool { return errors.Is(err, winfw.ErrCancelled) }

// firstLine is the text up to the first line break.
func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

// openNetworkSettings opens the Windows Network status page.
func (a *bridgeApp) openNetworkSettings(c *ui.Context) {
	open := a.openURL
	if open == nil {
		open = shellOpenURL
	}
	if err := open(networkSettingsURI); err != nil {
		componentLogger(componentApp).Error("open network settings failed", "err", err)
		c.Toast(networkSettingsText)
	}
}

// copyDiagnostics puts the network diagnostics on the clipboard.
func (a *bridgeApp) copyDiagnostics(c *ui.Context) {
	clip := a.network().clipboard
	if clip == nil {
		clip = mygo.Clipboard
	}
	clip.WriteText(a.diagnostics())
	c.Toast(diagnosticsCopiedMsg)
}

// diagnostics is the text of Copy Diagnostics and of the banner's Details.
func (a *bridgeApp) diagnostics() string {
	state := a.network().state
	facts := diagnosticsFacts{Version: version, Host: a.host, IP: a.lanIP(), Port: bridgenet.DefaultPort, Server: a.serverLine()}
	return diagnosticsText(facts, state.Result(), state.HasResult())
}

// serverLine says whether the IPP server is listening.
func (a *bridgeApp) serverLine() string {
	if a.shared == nil {
		return "unknown"
	}
	status := a.shared.ListenStatus()
	if !status.Attempted() {
		return "not started yet"
	}
	return status.String()
}

// reachListen adapts the server's listening state for the checker.
func reachListen(r bridgenet.ListenStatusReader) func() reach.Listen {
	return func() reach.Listen {
		s := r.ListenStatus()
		return reach.Listen{Attempted: s.Attempted(), Listening: s.Listening, Addr: s.Addr, Err: s.Err}
	}
}
