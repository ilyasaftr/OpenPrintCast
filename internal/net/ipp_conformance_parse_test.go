package net

import (
	"slices"
	"strings"
	"testing"
)

const cannedIpptoolOutput = `"/usr/share/cups/ipptool/ipp-1.1.test":
    RFC 8011 section 4.2.1: Print-Job Operation                          [PASS]
    RFC 8011 section 4.2.5: Get-Printer-Attributes Operation (default)   [PASS]
        printer-uri-supported (uri) = ipp://127.0.0.1:1/ipp/print/hp
        media-supported (keyword) = iso_a4_210x297mm
    RFC 8011 section 4.2.2: Print-URI Operation                          [SKIP]
    Print-Job with copies                                                [PASS]
ipptool: Filename "document-a4.pdf" on line 12 cannot be read.
some unrelated garbage [FAIL]
"/usr/share/cups/ipptool/ipp-everywhere.test":
    PWG 5100.14 section 5.1/5.2 - Required Operations and Attributes     [FAIL]
        RECEIVED: 4893 bytes in response
        status-code = successful-ok (successful-ok)
        EXPECTED: printer-device-id
        EXPECTED: printer-supply
        EXPECTED: media-col-ready WITH-VALUE "x"
        GOT: media-col-ready = 3
    Another step                                                         [FAIL]
        EXPECTED: STATUS successful-ok
    Last step                                                            [PASS]
        EXPECTED: ignored-on-pass
`

func TestParseIpptoolOutput(t *testing.T) {
	report := parseIpptoolOutput(cannedIpptoolOutput)
	if want := []string{"ipp-1.1.test", "ipp-everywhere.test"}; !slices.Equal(report.Suites, want) {
		t.Fatalf("suites = %v, want %v", report.Suites, want)
	}
	if len(report.Diagnostics) != 1 || !strings.Contains(report.Diagnostics[0], "cannot be read") {
		t.Fatalf("diagnostics = %v", report.Diagnostics)
	}
	if got := [3]int{report.count(stepPass), report.count(stepFail), report.count(stepSkip)}; got != [3]int{4, 2, 1} {
		t.Fatalf("pass/fail/skip = %v, want [4 2 1]", got)
	}
	steps := report.Steps
	if steps[0].Suite != "ipp-1.1.test" || steps[0].Name != "RFC 8011 section 4.2.1: Print-Job Operation" || steps[0].Status != stepPass {
		t.Fatalf("first step = %+v", steps[0])
	}
	if len(steps[1].Missing)+len(steps[1].Other) != 0 {
		t.Fatalf("details of a passing step were kept: %+v", steps[1])
	}
	failed := report.failures()
	if len(failed) != 2 {
		t.Fatalf("failures = %+v", failed)
	}
	first := failed[0]
	if first.Suite != "ipp-everywhere.test" || first.Name != "PWG 5100.14 section 5.1/5.2 - Required Operations and Attributes" {
		t.Fatalf("failed step = %+v", first)
	}
	if want := []string{"printer-device-id", "printer-supply"}; !slices.Equal(first.Missing, want) {
		t.Errorf("missing = %v, want %v", first.Missing, want)
	}
	wantOther := []string{`EXPECTED: media-col-ready WITH-VALUE "x"`, "GOT: media-col-ready = 3"}
	if !slices.Equal(first.Other, wantOther) {
		t.Errorf("other = %q, want %q", first.Other, wantOther)
	}
	if want := []string{"EXPECTED: STATUS successful-ok"}; !slices.Equal(failed[1].Other, want) {
		t.Errorf("other = %q, want %q", failed[1].Other, want)
	}
}

func TestParseIpptoolOutputToleratesNoise(t *testing.T) {
	for _, output := range []string{"", "\n\n", "garbage\r\n    [PASS]\n        EXPECTED: x\n"} {
		if report := parseIpptoolOutput(output); len(report.Steps) != 0 || len(report.Suites) != 0 {
			t.Errorf("parseIpptoolOutput(%q) = %+v, want nothing", output, report)
		}
	}
	report := parseIpptoolOutput("\"/a/b.test\":\r\n    Step one    [FAIL]\r\n        EXPECTED: attr-a\r\n")
	if len(report.Steps) != 1 || !slices.Equal(report.Steps[0].Missing, []string{"attr-a"}) || report.Suites[0] != "b.test" {
		t.Fatalf("CRLF output parsed as %+v", report)
	}
}

func TestFailuresMergeRepeatedStepNames(t *testing.T) {
	report := parseIpptoolOutput(`"/x/s.test":
    Same name    [FAIL]
        EXPECTED: a
    Same name    [FAIL]
        EXPECTED: b
`)
	failed := report.failures()
	if len(failed) != 1 || !slices.Equal(failed[0].Missing, []string{"a", "b"}) {
		t.Fatalf("failures = %+v", failed)
	}
}

func ratchetFixture() (ipptoolReport, conformanceRun, map[conformanceStep]knownFailure) {
	report := parseIpptoolOutput(`"/x/suite.test":
    Good step    [PASS]
    Good step 2  [PASS]
    Known step   [FAIL]
        EXPECTED: attr-a
        EXPECTED: attr-b
`)
	known := map[conformanceStep]knownFailure{
		{"suite.test", "Known step"}: {"attr-a": "reason a", "attr-b": "reason b"},
	}
	return report, conformanceRun{suite: "suite.test", passFloor: 2}, known
}

func TestRatchetAcceptsExactlyTheAllowlist(t *testing.T) {
	report, run, known := ratchetFixture()
	if problems := ratchetProblems(report, run, known); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestRatchetProblems(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ipptoolReport, *conformanceRun, map[conformanceStep]knownFailure)
		want   string
	}{
		{"unlisted failure", func(_ *ipptoolReport, _ *conformanceRun, k map[conformanceStep]knownFailure) {
			delete(k, conformanceStep{"suite.test", "Known step"})
		}, "new failure, missing [attr-a attr-b]"},
		{"new missing attribute", func(_ *ipptoolReport, _ *conformanceRun, k map[conformanceStep]knownFailure) {
			delete(k[conformanceStep{"suite.test", "Known step"}], "attr-b")
		}, "new missing attribute attr-b"},
		{"allowlisted attribute passes", func(_ *ipptoolReport, _ *conformanceRun, k map[conformanceStep]knownFailure) {
			k[conformanceStep{"suite.test", "Known step"}]["attr-c"] = "reason c"
		}, "attr-c now passes: remove it from knownConformanceFailures"},
		{"allowlisted step passes", func(r *ipptoolReport, _ *conformanceRun, _ map[conformanceStep]knownFailure) {
			r.Steps[2].Status = stepPass
		}, `"Known step" no longer fails: remove it from knownConformanceFailures`},
		{"pass count drops", func(r *ipptoolReport, _ *conformanceRun, _ map[conformanceStep]knownFailure) {
			r.Steps[0].Status = stepSkip
		}, "1 steps pass, below the floor of 2"},
		{"unexpected detail", func(r *ipptoolReport, _ *conformanceRun, _ map[conformanceStep]knownFailure) {
			r.Steps[2].Other = []string{"GOT: x"}
		}, `unexpected failure detail "GOT: x"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, run, known := ratchetFixture()
			tc.mutate(&report, &run, known)
			problems := strings.Join(ratchetProblems(report, run, known), "\n")
			if !strings.Contains(problems, tc.want) {
				t.Fatalf("problems = %q, want mention of %q", problems, tc.want)
			}
		})
	}
}

func TestRatchetIgnoresAllowlistOfSuitesThatDidNotRun(t *testing.T) {
	report, run, known := ratchetFixture()
	known[conformanceStep{"other.test", "Elsewhere"}] = knownFailure{"attr": "reason"}
	if problems := ratchetProblems(report, run, known); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestKnownConformanceFailuresHaveReasons(t *testing.T) {
	for step, failure := range knownConformanceFailures {
		if len(failure) == 0 {
			t.Errorf("%v has no attributes", step)
		}
		for attr, reason := range failure {
			if strings.TrimSpace(reason) == "" {
				t.Errorf("%v: %s has no reason", step, attr)
			}
		}
	}
}
