package net

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// realAbortOutput is what ipptool 2.4.7 prints when a FILE document is
// missing: the steps before it, then one "ipptool:" line.
const realAbortOutput = `"/usr/share/cups/ipptool/ipp-everywhere.test":
    Step one                                                             [PASS]
ipptool: Filename "pwg-raster-samples-150dpi/black-1/color.jpg-4x6-black-1-150dpi.pwg" (mapped to "/usr/share/cups/ipptool/pwg-raster-samples-150dpi/black-1/color.jpg-4x6-black-1-150dpi.pwg") on line 192 of "/usr/share/cups/ipptool/ipp-everywhere.test" cannot be read.
`

func TestRatchetFailsWhenIpptoolStopsEarly(t *testing.T) {
	report := parseIpptoolOutput(realAbortOutput)
	run := conformanceRun{suite: "ipp-everywhere.test"}
	problems := strings.Join(conformanceProblems(report, run, expectedConformanceSkips), "\n")
	if strings.Count(problems, "\n") != 1 {
		t.Errorf("problems = %q, want only the abort and its hint, not the counts it caused", problems)
	}
	for _, want := range []string{"ipp-everywhere.test: ipptool stopped early", "cannot be read", fetchScript} {
		if !strings.Contains(problems, want) {
			t.Errorf("problems = %q, want mention of %q", problems, want)
		}
	}
}

func TestRatchetExplainsOtherIpptoolDiagnostics(t *testing.T) {
	report := parseIpptoolOutput("\"/x/s.test\":\n    A step    [PASS]\nipptool: Unable to connect to \"127.0.0.1\" on port 1 - Host is down\n")
	problems := abortProblems(report, conformanceRun{suite: "s.test", passFloor: 1})
	if len(problems) != 1 || !strings.Contains(problems[0], "Unable to connect") || strings.Contains(problems[0], fetchScript) {
		t.Fatalf("problems = %q, want the diagnostic without the fetch hint", problems)
	}
}

func TestWithStatusSelectsSteps(t *testing.T) {
	report := parseIpptoolOutput("\"/x/s.test\":\n    A    [PASS]\n    B    [SKIP]\n    C    [SKIP]\n")
	if got := report.withStatus(stepSkip); len(got) != 2 || got[0].Name != "B" || got[1].Name != "C" {
		t.Fatalf("skipped steps = %+v", got)
	}
}

func skipFixture() (ipptoolReport, conformanceRun, []skipRule) {
	report := parseIpptoolOutput(`"/x/suite.test":
    Runs    [PASS]
    Needs duplex    [SKIP]
    Needs duplex again    [SKIP]
    Needs 4x6    [SKIP]
`)
	rules := []skipRule{
		skipNames("suite.test", "no duplex", "Needs duplex", "Needs duplex again"),
		skipNames("suite.test", "no 4x6", "Needs 4x6"),
	}
	return report, conformanceRun{suite: "suite.test", passFloor: 1, skips: 3}, rules
}

func TestSkipRatchetAcceptsExactlyTheRules(t *testing.T) {
	report, run, rules := skipFixture()
	if problems := skipProblems(report, run, rules); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestSkipProblems(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ipptoolReport, *conformanceRun, *[]skipRule)
		want   string
	}{
		{"a step that ran now skips", func(r *ipptoolReport, run *conformanceRun, _ *[]skipRule) {
			r.Steps[0].Status = stepSkip
			run.skips = 4
		}, `suite.test: "Runs" now skips, it ran before`},
		{"skip count changes", func(_ *ipptoolReport, run *conformanceRun, _ *[]skipRule) {
			run.skips = 5
		}, "suite.test: 3 steps skip, expected 5"},
		{"an accepted skip now runs", func(r *ipptoolReport, run *conformanceRun, _ *[]skipRule) {
			r.Steps[3].Status = stepPass
			run.skips = 2
		}, "0 steps skip for"},
		{"a rule counts a duplicate more than the report has", func(_ *ipptoolReport, _ *conformanceRun, rules *[]skipRule) {
			(*rules)[1] = skipNames("suite.test", "no 4x6", "Needs 4x6", "Needs 4x6")
		}, "expected 2: update the skipRule (no 4x6)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, run, rules := skipFixture()
			tc.mutate(&report, &run, &rules)
			problems := strings.Join(skipProblems(report, run, rules), "\n")
			if !strings.Contains(problems, tc.want) {
				t.Fatalf("problems = %q, want mention of %q", problems, tc.want)
			}
		})
	}
}

func TestSkipRulesIgnoreSuitesThatDidNotRun(t *testing.T) {
	report, run, rules := skipFixture()
	rules = append(rules, skipNames("other.test", "elsewhere", "Not run"))
	if problems := skipProblems(report, run, rules); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
}

func TestSkipNamesMatchExactNamesOnly(t *testing.T) {
	rule := skipNames("s.test", "reason", "Print-Job (A4) [x]")
	for name, want := range map[string]bool{
		"Print-Job (A4) [x]":       true,
		"Print-Job (A4) [x] again": false,
		"Print-Job A4 x":           false,
	} {
		if got := rule.matches(ipptoolStep{Suite: "s.test", Name: name}); got != want {
			t.Errorf("matches(%q) = %v, want %v", name, got, want)
		}
	}
	if rule.matches(ipptoolStep{Suite: "t.test", Name: "Print-Job (A4) [x]"}) {
		t.Error("a rule matched a step of another test file")
	}
}

func TestAcceptedDeviationsExplainEachItem(t *testing.T) {
	report := parseIpptoolOutput(`"/x/suite.test":
    Known step    [FAIL]
        EXPECTED: attr-a
        EXPECTED: attr-b
    Needs duplex    [SKIP]
`)
	known := map[conformanceStep]knownFailure{{"suite.test", "Known step"}: {"attr-a": "reason a", "attr-b": "reason b"}}
	rules := []skipRule{skipNames("suite.test", "no duplex", "Needs duplex"), skipNames("other.test", "elsewhere", "x")}
	got := strings.Join(acceptedDeviations(report, known, rules), "\n")
	want := `accepted failure: suite.test: "Known step" lacks attr-a: reason a
accepted failure: suite.test: "Known step" lacks attr-b: reason b
accepted skip: suite.test: 1 steps: no duplex`
	if got != want {
		t.Fatalf("explanations:\n%s\nwant:\n%s", got, want)
	}
}

func TestExpectedConformanceSkipsAreDocumented(t *testing.T) {
	for _, rule := range oneSidedConformanceSkips() {
		if rule.count <= 0 || strings.TrimSpace(rule.reason) == "" {
			t.Errorf("rule %v needs a positive count and a reason", rule.pattern)
		}
	}
}

func TestDocumentSearchPrefersTheFetchedDirectory(t *testing.T) {
	data, system := t.TempDir(), t.TempDir()
	env := ipptoolEnv{dataDir: data, testDir: system}
	if _, found := env.findDocument("a.pdf"); found {
		t.Fatal("found a document that exists nowhere")
	}
	writeFile(t, filepath.Join(system, "a.pdf"))
	if path, found := env.findDocument("a.pdf"); !found || path != filepath.Join(system, "a.pdf") {
		t.Fatalf("findDocument = %q, %v, want the CUPS data directory copy", path, found)
	}
	writeFile(t, filepath.Join(data, "a.pdf"))
	if path, _ := env.findDocument("a.pdf"); path != filepath.Join(data, "a.pdf") {
		t.Fatalf("findDocument = %q, want the fetched copy first", path)
	}
}

func TestIpptoolRunsInTheFetchedDirectory(t *testing.T) {
	data := t.TempDir()
	if cmd := (ipptoolEnv{tool: "ipptool", dataDir: data}).command("-t"); cmd.Dir != data {
		t.Fatalf("ipptool runs in %q, want %q", cmd.Dir, data)
	}
	if cmd := (ipptoolEnv{tool: "ipptool", dataDir: filepath.Join(data, "missing")}).command("-t"); cmd.Dir != "" {
		t.Fatalf("ipptool runs in %q, want the current directory when nothing was fetched", cmd.Dir)
	}
}

func TestDirectoriesFollowTheEnvironment(t *testing.T) {
	t.Setenv(cupsDataEnv, "")
	t.Setenv(cupsTestDataEnv, "")
	if got := cupsTestDir(); got != filepath.FromSlash("/usr/share/cups/ipptool") {
		t.Errorf("cupsTestDir = %q", got)
	}
	if got := testDataDir(); !filepath.IsAbs(got) || !strings.HasSuffix(got, filepath.FromSlash(defaultTestDataDir)) {
		t.Errorf("testDataDir = %q, want the absolute default", got)
	}
	cache := t.TempDir()
	t.Setenv(cupsDataEnv, "/opt/cups")
	t.Setenv(cupsTestDataEnv, cache)
	if got := cupsTestDir(); got != filepath.FromSlash("/opt/cups/ipptool") {
		t.Errorf("cupsTestDir = %q", got)
	}
	if got := testDataDir(); got != cache {
		t.Errorf("testDataDir = %q, want %q", got, cache)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
