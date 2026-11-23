package main

import (
	"testing"
	"time"
)

var t0 = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

// answered returns a state that has shown verdict at t0.
func answered(verdict ...bool) *netCheckState {
	s := newNetCheckState()
	s.Request(t0, false)
	if len(verdict) > 0 && verdict[0] {
		s.Finish(publicVerdict)
	} else {
		s.Finish(okVerdict)
	}
	s.Tick(t0)
	return s
}

func TestServerEventStartsACheckAfterTheDelay(t *testing.T) {
	s := newNetCheckState()
	s.NoteServerEvent()
	if out := s.Tick(t0); out.Start {
		t.Fatal("the check started before the delay")
	}
	if wake := s.NextWake(t0); wake != netCheckStartDelay {
		t.Fatalf("wake = %v, want %v", wake, netCheckStartDelay)
	}
	if out := s.Tick(t0.Add(netCheckStartDelay - time.Millisecond)); out.Start {
		t.Fatal("started a moment early")
	}
	if out := s.Tick(t0.Add(netCheckStartDelay)); !out.Start || !s.Busy() {
		t.Fatalf("no check after the delay: %+v busy=%v", out, s.Busy())
	}
}

func TestAnotherServerEventRestartsTheDelay(t *testing.T) {
	s := newNetCheckState()
	s.NoteServerEvent()
	s.Tick(t0)
	s.NoteServerEvent() // the retry 10 s later, say
	s.Tick(t0.Add(2 * time.Second))
	if out := s.Tick(t0.Add(netCheckStartDelay)); out.Start {
		t.Fatal("the second report did not push the check back")
	}
	if out := s.Tick(t0.Add(2*time.Second + netCheckStartDelay)); !out.Start {
		t.Fatal("no check after the second report")
	}
}

func TestChecksNeverOverlap(t *testing.T) {
	s := newNetCheckState()
	if !s.Request(t0, true) {
		t.Fatal("first request refused")
	}
	if s.Request(t0, true) || s.Request(t0, false) {
		t.Fatal("a second check started while one runs")
	}
	s.NoteServerEvent()
	if out := s.Tick(t0.Add(time.Hour)); out.Start {
		t.Fatal("a due check started over a running one")
	}
}

func TestMinimumBusyTimeAppliesToManualChecksOnly(t *testing.T) {
	manual := newNetCheckState()
	manual.Request(t0, true)
	manual.Finish(okVerdict)
	if out := manual.Tick(t0.Add(netCheckMinBusy - time.Millisecond)); out.Revealed || !manual.Busy() {
		t.Fatal("a manual check showed its answer before the minimum busy time")
	}
	if wake := manual.NextWake(t0.Add(100 * time.Millisecond)); wake != netCheckMinBusy-100*time.Millisecond {
		t.Fatalf("wake = %v", wake)
	}
	if out := manual.Tick(t0.Add(netCheckMinBusy)); !out.Revealed || manual.Busy() || !manual.HasResult() {
		t.Fatalf("no answer after the minimum: %+v", out)
	}

	auto := newNetCheckState()
	auto.Request(t0, false)
	auto.Finish(okVerdict)
	if out := auto.Tick(t0); !out.Revealed || auto.Busy() {
		t.Fatal("an automatic check was held back")
	}
}

func TestNetFinishWithoutARunningCheckIsIgnored(t *testing.T) {
	s := newNetCheckState()
	s.Finish(publicVerdict)
	if out := s.Tick(t0); out.Revealed || s.HasResult() {
		t.Fatal("an answer from nowhere was shown")
	}
}

func TestPollsEveryMinuteOnlyWhileAProblemIsShown(t *testing.T) {
	problem := answered(true)
	if wake := problem.NextWake(t0); wake != netCheckPollEvery {
		t.Fatalf("wake = %v, want %v", wake, netCheckPollEvery)
	}
	if out := problem.Tick(t0.Add(netCheckPollEvery - time.Second)); out.Start {
		t.Fatal("polled early")
	}
	if out := problem.Tick(t0.Add(netCheckPollEvery)); !out.Start {
		t.Fatal("did not poll after a minute")
	}

	healthy := answered(false)
	if out := healthy.Tick(t0.Add(time.Hour)); out.Start {
		t.Fatal("polled although there is no problem")
	}
	if healthy.NextWake(t0) != 0 {
		t.Fatal("a healthy state asks for wake-ups")
	}
}

func TestNeverPollsOrStartsWhileTheWindowIsHidden(t *testing.T) {
	s := answered(true)
	s.SetVisible(false)
	s.NoteServerEvent()
	if out := s.Tick(t0.Add(time.Hour)); out.Start || s.Busy() {
		t.Fatal("a check started in a hidden window")
	}
	if s.NextWake(t0.Add(time.Hour)) != 0 {
		t.Fatal("a hidden window asks for wake-ups")
	}
}

func TestShowingTheWindowChecksWhenTheAnswerIsStale(t *testing.T) {
	s := answered(false)
	s.SetVisible(false)
	s.SetVisible(true)
	if out := s.Tick(t0.Add(netCheckStaleAfter - time.Second)); out.Start {
		t.Fatal("checked again within five minutes")
	}
	s.SetVisible(false)
	s.SetVisible(true)
	if out := s.Tick(t0.Add(netCheckStaleAfter)); !out.Start {
		t.Fatal("no check after five minutes")
	}
}

func TestShowingTheWindowChecksWhenNothingWasChecked(t *testing.T) {
	s := newNetCheckState()
	s.SetVisible(false)
	s.SetVisible(true)
	if out := s.Tick(t0); !out.Start {
		t.Fatal("a first show without an answer did not check")
	}
}

func TestSettingVisibleWhileVisibleChangesNothing(t *testing.T) {
	s := answered(false)
	s.SetVisible(true)
	if out := s.Tick(t0.Add(time.Hour)); out.Start {
		t.Fatal("a repeated show started a check")
	}
}

func TestRequestClearsAPendingTrigger(t *testing.T) {
	s := newNetCheckState()
	s.NoteServerEvent()
	s.Tick(t0)
	s.Request(t0.Add(time.Second), true)
	s.Finish(okVerdict)
	s.Tick(t0.Add(2 * time.Second))
	if out := s.Tick(t0.Add(time.Hour)); out.Start {
		t.Fatal("the trigger fired a second check")
	}
}
