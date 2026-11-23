package main

import (
	"context"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/reach"
)

// fakeNetChecker answers every check with a settable assessment.
type fakeNetChecker struct {
	answer reach.Assessment
	calls  int
}

func (f *fakeNetChecker) Check(context.Context) reach.Assessment {
	f.calls++
	return f.answer
}

// fakeFixer records how the fix was asked for and can fail.
type fakeFixer struct {
	elevated bool
	err      error
	calls    []bool
	onApply  func()
}

func (f *fakeFixer) Elevated() bool { return f.elevated }

func (f *fakeFixer) Apply(_ context.Context, allowPublic bool) error {
	f.calls = append(f.calls, allowPublic)
	if f.onApply != nil {
		f.onApply()
	}
	return f.err
}

// fakeClipboard keeps what was copied.
type fakeClipboard struct{ texts []string }

func (f *fakeClipboard) WriteText(text string) { f.texts = append(f.texts, text) }

// inlineRun is a scanRunner that does the work and its hop back at once.
func inlineRun(work, done func()) {
	work()
	done()
}

// Verdicts the tests use, shaped like the classifier's.
var (
	publicVerdict = reach.Assessment{
		State: reach.StateBlocked, Cause: reach.CausePublicNetwork, Profile: reach.ProfilePublic,
		Fix: reach.FixOpenNetworkSettings, AltFix: reach.FixAllowOnPublic, ActiveProfiles: []string{reach.ProfilePublic},
	}
	noRuleVerdict = reach.Assessment{
		State: reach.StateBlocked, Cause: reach.CauseNoRule, Profile: reach.ProfilePrivate,
		Fix: reach.FixAllowFirewall, ActiveProfiles: []string{reach.ProfilePrivate},
	}
	inUseVerdict = reach.Assessment{
		State: reach.StateNotListening, Cause: reach.CauseOtherProgramOnPort, Owner: "cupsd",
		Detail: "the port is already in use by another program",
	}
	okVerdict = reach.Assessment{
		State: reach.StateOK, ActiveProfiles: []string{reach.ProfilePrivate}, Rules: []string{"OpenPrintCast (IPP)"},
	}
)

// netRig is an app wired to fake network seams.
type netRig struct {
	app       *bridgeApp
	checker   *fakeNetChecker
	fixer     *fakeFixer
	clipboard *fakeClipboard
	opener    *urlRecorder
}

// newNetRig builds the test app whose checker answers answer, with checks
// running inline.
func newNetRig(t *testing.T, answer reach.Assessment) *netRig {
	t.Helper()
	rig := &netRig{
		app: newTestApp(t), checker: &fakeNetChecker{answer: answer},
		fixer: &fakeFixer{}, clipboard: &fakeClipboard{}, opener: &urlRecorder{},
	}
	m := rig.app.network()
	m.checker, m.fixer, m.clipboard, m.runner = rig.checker, rig.fixer, rig.clipboard, inlineRun
	rig.app.openURL = rig.opener.open
	return rig
}

// netNeedsAttentionPill is the header pill's text when the check found a
// problem with the two test printers (one shared).
const netNeedsAttentionPill = "Sharing 1 of 2" + netAttentionSuffix
