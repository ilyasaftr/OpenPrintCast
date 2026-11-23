package main

import (
	"testing"
	"time"
)

var scanEpoch = time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

func TestScanOutcomeWording(t *testing.T) {
	tests := []struct {
		name    string
		outcome scanOutcome
		want    string
	}{
		{"one added", scanOutcome{added: 1, total: 3}, "1 new printer added"},
		{"several added", scanOutcome{added: 2, total: 4}, "2 new printers added"},
		{"one removed", scanOutcome{removed: 1, total: 1}, "1 printer removed"},
		{"several removed", scanOutcome{removed: 3, total: 0}, "3 printers removed"},
		{"added and removed", scanOutcome{added: 2, removed: 1, total: 3}, "2 new printers added, 1 removed"},
		{"unchanged", scanOutcome{total: 3}, "No new printers found"},
		{"nothing at all", scanOutcome{}, "No printers found"},
		{"failure", scanOutcome{failed: true, total: 3}, "Couldn't read the printer list"},
		{"failure wins over counts", scanOutcome{failed: true, added: 1}, "Couldn't read the printer list"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.outcome.message(); got != tc.want {
				t.Fatalf("message = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestScanStateStartsIdle(t *testing.T) {
	s := newScanState()
	s.Advance(scanEpoch)
	if s.Busy() || s.Status() != (scanStatus{}) || s.TakeAnnouncement() != "" || s.NextWake(scanEpoch) != 0 {
		t.Fatalf("a fresh state is not idle: %+v", s.Status())
	}
}

func TestScanBeginIsAcknowledgedAtOnce(t *testing.T) {
	s := newScanState()
	if !s.Begin(scanEpoch) {
		t.Fatal("Begin refused on an idle state")
	}
	if st := s.Status(); !st.Busy || st.Text != scanBusyText {
		t.Fatalf("status right after Begin = %+v", st)
	}
	if got := s.TakeAnnouncement(); got != scanBusyText {
		t.Fatalf("announcement = %q", got)
	}
	if got := s.TakeAnnouncement(); got != "" {
		t.Fatalf("announcement repeated: %q", got)
	}
}

func TestScanBeginWhileBusyIsIgnored(t *testing.T) {
	s := newScanState()
	s.Begin(scanEpoch)
	s.TakeAnnouncement()
	if s.Begin(scanEpoch.Add(time.Second)) {
		t.Fatal("a second Begin was accepted while scanning")
	}
	if s.startedAt != scanEpoch || s.TakeAnnouncement() != "" {
		t.Fatal("an ignored Begin changed the state")
	}
	s.Finish(scanOutcome{total: 1})
	if s.Begin(scanEpoch.Add(time.Second)) {
		t.Fatal("Begin accepted while the result is held back")
	}
}

func TestScanResultWaitsForTheMinimumBusyTime(t *testing.T) {
	s := newScanState()
	s.Begin(scanEpoch)
	s.TakeAnnouncement()
	s.Finish(scanOutcome{added: 1, total: 2})

	s.Advance(scanEpoch.Add(scanMinBusy - time.Millisecond))
	if !s.Busy() || s.Status().Text != scanBusyText {
		t.Fatalf("result shown %v too early: %+v", time.Millisecond, s.Status())
	}
	if wake := s.NextWake(scanEpoch.Add(scanMinBusy - time.Millisecond)); wake != time.Millisecond {
		t.Fatalf("NextWake = %v, want 1ms", wake)
	}
	if s.TakeAnnouncement() != "" {
		t.Fatal("a held-back result was announced")
	}

	s.Advance(scanEpoch.Add(scanMinBusy))
	if s.Busy() || s.Status() != (scanStatus{Text: "1 new printer added"}) {
		t.Fatalf("status at the minimum = %+v", s.Status())
	}
	if got := s.TakeAnnouncement(); got != "1 new printer added" {
		t.Fatalf("announcement = %q", got)
	}
}

func TestScanSlowerThanTheMinimumShowsItsResultAtOnce(t *testing.T) {
	s := newScanState()
	s.Begin(scanEpoch)
	s.Advance(scanEpoch.Add(3 * time.Second))
	if !s.Busy() {
		t.Fatal("state left busy without a result")
	}
	if wake := s.NextWake(scanEpoch.Add(3 * time.Second)); wake != 0 {
		t.Fatalf("NextWake = %v while only the scan can end the wait", wake)
	}
	s.Finish(scanOutcome{total: 2})
	s.Advance(scanEpoch.Add(3 * time.Second))
	if s.Busy() || s.Status().Text != scanNoChangeText {
		t.Fatalf("status = %+v", s.Status())
	}
}

func TestScanFinishIsIgnoredWhenNoScanRuns(t *testing.T) {
	s := newScanState()
	s.Finish(scanOutcome{added: 5})
	s.Advance(scanEpoch.Add(time.Hour))
	if s.Status() != (scanStatus{}) {
		t.Fatalf("a stray Finish produced %+v", s.Status())
	}
}

func TestScanGoodResultFadesAfterItsTime(t *testing.T) {
	s := newScanState()
	s.Begin(scanEpoch)
	s.Finish(scanOutcome{total: 1})
	shown := scanEpoch.Add(scanMinBusy)
	s.Advance(shown)
	if wake := s.NextWake(shown); wake != scanResultVisible {
		t.Fatalf("NextWake = %v, want %v", wake, scanResultVisible)
	}
	s.Advance(shown.Add(scanResultVisible - time.Millisecond))
	if s.Status().Text == "" {
		t.Fatal("result faded early")
	}
	s.Advance(shown.Add(scanResultVisible))
	if s.Status() != (scanStatus{}) || s.NextWake(shown.Add(scanResultVisible)) != 0 {
		t.Fatalf("result did not fade: %+v", s.Status())
	}
}

func TestScanFailureStaysUntilTheNextScan(t *testing.T) {
	s := newScanState()
	s.Begin(scanEpoch)
	s.Finish(scanOutcome{failed: true})
	s.Advance(scanEpoch.Add(scanMinBusy))
	s.Advance(scanEpoch.Add(time.Hour))
	if st := s.Status(); !st.Failed || st.Text != scanFailedText {
		t.Fatalf("failure faded: %+v", st)
	}
	if !s.Begin(scanEpoch.Add(time.Hour)) || s.Status().Failed {
		t.Fatal("the next scan did not replace the failure")
	}
}

func TestScanCanRunAgainAfterAResult(t *testing.T) {
	s := newScanState()
	for i := 0; i < 3; i++ {
		start := scanEpoch.Add(time.Duration(i) * time.Minute)
		if !s.Begin(start) {
			t.Fatalf("scan %d refused", i)
		}
		s.Finish(scanOutcome{total: i})
		s.Advance(start.Add(scanMinBusy))
		if s.Busy() {
			t.Fatalf("scan %d never completed", i)
		}
	}
}
