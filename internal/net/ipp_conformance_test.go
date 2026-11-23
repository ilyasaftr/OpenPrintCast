package net

import (
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The ratchet runs CUPS's ipptool conformance suites against the in-process
// server and fails when the result gets worse (a new failure, or fewer
// passing steps than the recorded floor) or better (an allowlisted failure
// now passes, so the allowlist must shrink). Update the floors and the
// allowlist deliberately, in the change that causes the difference.
//
// Skipped steps are ratcheted as well (see expectedConformanceSkips): a step
// that ran before and now skips fails the run by name, and so does a run
// that stopped early because a sample document could not be read.
//
// The suites run to the end: ipptool is given -I, because it would stop at
// the first failing step (the allowlisted one in ipp-everywhere.test comes
// first) and silently leave the 430-odd raster steps behind it unrun. The
// sample documents come from scripts/fetch-cups-testdata.sh.

// Passing and skipped steps per ipptool run as recorded with ipptool 2.4.7.
// Each suite includes the previous one (ipp-everywhere includes ipp-2.0,
// which includes ipp-1.1), so the counts include the included steps.
// The first set is for a driver that reports and accepts duplex, the second
// for a one-sided driver, which also skips the duplex steps.
const (
	ipp11PassFloor      = 38
	ipp20PassFloor      = 39
	everywherePassFloor = 73

	ipp11Skips      = 28
	ipp20Skips      = 28
	everywhereSkips = 427

	simplexIPP11PassFloor      = 36
	simplexIPP20PassFloor      = 37
	simplexEverywherePassFloor = 71

	simplexIPP11Skips      = 30
	simplexIPP20Skips      = 30
	simplexEverywhereSkips = 429
)

// fileNotReadable is the part of ipptool's message that says a FILE document
// could not be opened.
const fileNotReadable = "cannot be read"

// conformanceStep identifies a step by test file and printed step name.
type conformanceStep struct{ suite, name string }

// knownFailure is an accepted failure: the exact attributes the step reports
// as missing, each with the reason it is accepted.
type knownFailure map[string]string

// knownConformanceFailures lists every failing step the bridge accepts.
var knownConformanceFailures = map[conformanceStep]knownFailure{}

// conformanceRun is one ipptool invocation with its recorded pass floor and
// its exact number of skipped steps.
type conformanceRun struct {
	suite     string
	version   string // ipptool -V value, empty for the tool default
	passFloor int
	skips     int
}

// ipp-everywhere is run as IPP 2.0, the version PWG 5100.14 requires; it
// gives the same result as the default today.
var conformanceRuns = []conformanceRun{
	{"ipp-1.1.test", "", ipp11PassFloor, ipp11Skips},
	{"ipp-2.0.test", "2.0", ipp20PassFloor, ipp20Skips},
	{"ipp-everywhere.test", "2.0", everywherePassFloor, everywhereSkips},
}

// simplexConformanceRuns are the same runs against a one-sided driver.
var simplexConformanceRuns = []conformanceRun{
	{"ipp-1.1.test", "", simplexIPP11PassFloor, simplexIPP11Skips},
	{"ipp-2.0.test", "2.0", simplexIPP20PassFloor, simplexIPP20Skips},
	{"ipp-everywhere.test", "2.0", simplexEverywherePassFloor, simplexEverywhereSkips},
}

// conformancePrinterHost is the address the conformance printer reports for
// itself, as a network printer's Windows port would.
const conformancePrinterHost = "192.168.1.50"

// TestIPPConformanceRatchet runs the suites against a duplex-capable driver
// whose printer reports CMYK toner levels: the duplex and supply steps run
// and must pass.
func TestIPPConformanceRatchet(t *testing.T) {
	// The driver offers A4 and US Letter like most real ones, so the suites
	// also print on Letter.
	driver := newSupplySpooler(cmykSupplies(), conformancePrinterHost)
	driver.duplex = true
	runConformanceRatchet(t, driver, conformanceRuns, expectedConformanceSkips)
}

// TestIPPConformanceRatchetOneSided proves a driver without duplex behaves as
// before duplex support: sides-supported is one-sided only and the duplex
// steps skip.
func TestIPPConformanceRatchetOneSided(t *testing.T) {
	driver := newSupplySpooler(cmykSupplies(), conformancePrinterHost)
	runConformanceRatchet(t, driver, simplexConformanceRuns, oneSidedConformanceSkips())
}

// TestIPPConformanceWithoutSupplyData proves the bridge claims no supplies it
// has no data for: against a driver that knows none, the suite misses
// exactly the supply attributes (besides the allowlisted one) and still
// completes. It is the honest state of every printer without a readable
// network port.
func TestIPPConformanceWithoutSupplyData(t *testing.T) {
	uri, _, tool := startConformanceServer(t, &mediaSpooler{&fakeSpooler{}})
	output, err := tool.command("-I", "-t", "-V", "2.0", "-f", tool.document(t, requestDocument), uri, tool.suite("ipp-everywhere.test")).CombinedOutput()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("running ipptool: %v", err)
	}
	report := parseIpptoolOutput(string(output))
	step := conformanceStep{"ipp-everywhere.test", "PWG 5100.14 section 5.1/5.2 - Required Operations and Attributes"}
	var missing []string
	for _, failed := range report.failures() {
		if key := (conformanceStep{failed.Suite, failed.Name}); key != step {
			t.Errorf("unexpected failing step %q: %v", failed.Name, failed.Missing)
			continue
		}
		missing = sortedUnique(failed.Missing)
	}
	want := []string{"ipp-features-supported", "printer-supply", "printer-supply-description", "printer-supply-info-uri"}
	if !slices.Equal(missing, want) {
		t.Fatalf("missing attributes = %v, want %v", missing, want)
	}
}

// oneSidedConformanceSkips adds the duplex steps to the accepted skips.
func oneSidedConformanceSkips() []skipRule {
	return append([]skipRule{noDuplexSkips}, expectedConformanceSkips...)
}

func runConformanceRatchet(t *testing.T, spooler Spooler, runs []conformanceRun, rules []skipRule) {
	uri, _, tool := startConformanceServer(t, spooler)
	document := tool.document(t, requestDocument)
	for _, run := range runs {
		t.Run(run.suite, func(t *testing.T) {
			args := []string{"-I", "-t", "-f", document}
			if run.version != "" {
				args = append([]string{"-V", run.version}, args...)
			}
			args = append(args, uri, tool.suite(run.suite))
			// ipptool exits non-zero whenever a step fails, so only the
			// report is judged.
			output, err := tool.command(args...).CombinedOutput()
			var exitErr *exec.ExitError
			if err != nil && !errors.As(err, &exitErr) {
				t.Fatalf("running ipptool: %v", err)
			}
			report := parseIpptoolOutput(string(output))
			problems := conformanceProblems(report, run, rules)
			for _, d := range report.Diagnostics {
				t.Log(d)
			}
			for _, line := range acceptedDeviations(report, knownConformanceFailures, rules) {
				t.Log(line)
			}
			if len(problems) > 0 {
				t.Fatalf("%s\n\nipptool output:\n%s", strings.Join(problems, "\n"), output)
			}
			if pass := report.count(stepPass); pass > run.passFloor {
				t.Logf("%d steps pass, floor is %d: raise the floor constant", pass, run.passFloor)
			}
		})
	}
}

// conformanceProblems judges a report against every allowlist and floor. A
// run that stopped early reports only that: its counts say nothing.
func conformanceProblems(report ipptoolReport, run conformanceRun, rules []skipRule) []string {
	if aborted := abortProblems(report, run); len(aborted) > 0 {
		return aborted
	}
	return append(ratchetProblems(report, run, knownConformanceFailures),
		skipProblems(report, run, rules)...)
}

// ratchetProblems compares a report with the allowlist and the run's floor
// and describes every way the result differs.
func ratchetProblems(report ipptoolReport, run conformanceRun, known map[conformanceStep]knownFailure) []string {
	var problems []string
	if pass := report.count(stepPass); pass < run.passFloor {
		problems = append(problems, fmt.Sprintf("%s: %d steps pass, below the floor of %d", run.suite, pass, run.passFloor))
	}
	failed := make(map[conformanceStep]bool)
	for _, step := range report.failures() {
		key := conformanceStep{step.Suite, step.Name}
		failed[key] = true
		problems = append(problems, failureProblems(key, step, known[key])...)
	}
	for key, allowed := range known {
		if !slices.Contains(report.Suites, key.suite) || failed[key] {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s: %q no longer fails: remove it from knownConformanceFailures: %v",
			key.suite, key.name, sortedKeys(allowed)))
	}
	slices.Sort(problems)
	return problems
}

// abortProblems describes every message ipptool printed itself: it prints
// one when it gives up, which leaves the rest of the suite unrun.
func abortProblems(report ipptoolReport, run conformanceRun) []string {
	var problems []string
	for _, diagnostic := range report.Diagnostics {
		problem := fmt.Sprintf("%s: ipptool stopped early, the suite is incomplete: %s", run.suite, diagnostic)
		if strings.Contains(diagnostic, fileNotReadable) {
			problem += fmt.Sprintf("\nA sample document is missing: run %s (see its header for the directory the tests read)", fetchScript)
		}
		problems = append(problems, problem)
	}
	return problems
}

// failureProblems judges one failed step against its allowlist entry, which
// is nil for a step that is not allowlisted.
func failureProblems(key conformanceStep, step ipptoolStep, allowed knownFailure) []string {
	var problems []string
	where := fmt.Sprintf("%s: %q", key.suite, key.name)
	if allowed == nil {
		problems = append(problems, fmt.Sprintf("%s: new failure, missing %v", where, sortedUnique(step.Missing)))
	}
	for _, attr := range sortedUnique(step.Missing) {
		if allowed != nil && allowed[attr] == "" {
			problems = append(problems, fmt.Sprintf("%s: new missing attribute %s", where, attr))
		}
	}
	for _, attr := range sortedKeys(allowed) {
		if !slices.Contains(step.Missing, attr) {
			problems = append(problems, fmt.Sprintf("%s: %s now passes: remove it from knownConformanceFailures", where, attr))
		}
	}
	for _, detail := range step.Other {
		problems = append(problems, fmt.Sprintf("%s: unexpected failure detail %q", where, detail))
	}
	return problems
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func sortedUnique(values []string) []string {
	out := slices.Clone(values)
	slices.Sort(out)
	return slices.Compact(out)
}
