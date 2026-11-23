package main

import (
	"strings"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestSummarizePrioritizesWhatNeedsAttention(t *testing.T) {
	cases := []struct {
		paused         bool
		shared, total  int
		online         bool
		wantText       string
		wantLevel      level
		wantBannerText bool
	}{
		{true, 2, 3, true, "Paused", levelWarn, true},
		{false, 2, 3, false, "No network", levelError, true},
		{false, 0, 0, true, "No printers found", levelWarn, false},
		{false, 0, 3, true, "Nothing shared", levelWarn, false},
		{false, 2, 3, true, "Sharing 2 of 3", levelOK, false},
	}
	for _, c := range cases {
		text, lvl := summarize(c.paused, c.shared, c.total, c.online)
		if text != c.wantText || lvl != c.wantLevel {
			t.Errorf("summarize(%+v) = %q,%v", c, text, lvl)
		}
		msg, _ := bannerMessage(c.paused, c.online)
		if (msg != "") != c.wantBannerText {
			t.Errorf("bannerMessage(%+v) = %q", c, msg)
		}
	}
}

func TestConnectionLine(t *testing.T) {
	if got := connectionLine("PC", "", 631); !strings.Contains(got, "Not connected") {
		t.Fatalf("offline: %q", got)
	}
	if got := connectionLine("PC", "192.168.1.5", 631); !strings.Contains(got, "PC") || !strings.Contains(got, "192.168.1.5:631") {
		t.Fatalf("online: %q", got)
	}
}

func TestJobStatusAlwaysHasText(t *testing.T) {
	for _, s := range []core.JobStatus{core.JobPending, core.JobPrinting, core.JobCompleted, core.JobFailed, core.JobCancelled} {
		if label, _ := jobStatus(s); label == "" {
			t.Errorf("status %q has no label", s)
		}
	}
	if _, lvl := jobStatus(core.JobFailed); lvl != levelError {
		t.Error("failed jobs must be an error")
	}
}

func TestNewestFirstAndFinished(t *testing.T) {
	jobs := []core.JobEntry{{ID: "a", Status: core.JobCompleted}, {ID: "b", Status: core.JobPending}}
	got := newestFirst(jobs)
	if got[0].ID != "b" || jobs[0].ID != "a" {
		t.Fatalf("newestFirst must reverse a copy: %v / %v", got, jobs)
	}
	if !hasFinishedJobs(jobs) || hasFinishedJobs(jobs[1:]) {
		t.Fatal("hasFinishedJobs mismatch")
	}
}

func TestJobFieldFallbacks(t *testing.T) {
	if jobPages(0) != "—" || jobPages(3) != "3" || jobDocument("  ") != "—" || jobDocument("a.pdf") != "a.pdf" {
		t.Fatal("job field fallbacks wrong")
	}
}

func TestJobPagesShowsTheCountOrADashForUnknown(t *testing.T) {
	for _, tc := range []struct {
		pages int
		want  string
	}{{-1, "—"}, {0, "—"}, {1, "1"}, {12, "12"}, {123, "123"}} {
		if got := jobPages(tc.pages); got != tc.want {
			t.Errorf("jobPages(%d) = %q, want %q", tc.pages, got, tc.want)
		}
	}
}

func TestValidateAirPrintName(t *testing.T) {
	if validateAirPrintName("Office (PC)") != "" {
		t.Fatal("short name must be valid")
	}
	if validateAirPrintName(strings.Repeat("x", 64)) == "" {
		t.Fatal("64-byte name must be rejected")
	}
}

func TestValidateAirPrintNameCountsBytesAndSaysSo(t *testing.T) {
	if validateAirPrintName(strings.Repeat("x", maxDNSLabelBytes)) != "" {
		t.Fatal("a name of exactly the limit must be valid")
	}
	// 32 two-byte letters are 32 characters but 64 bytes.
	got := validateAirPrintName(strings.Repeat("é", maxDNSLabelBytes/2+1))
	want := "Name is too long. It can be up to 63 bytes; accented and non-Latin characters use more than one."
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestPlural(t *testing.T) {
	if plural(1, "printer") != "1 printer" || plural(2, "printer") != "2 printers" {
		t.Fatal("plural wrong")
	}
}
