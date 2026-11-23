package net

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A skipped step is not a passed step. ipptool skips a step when the printer
// does not advertise the feature it needs (SKIP-IF-NOT-DEFINED on a variable
// that an earlier EXPECT ... DEFINE-MATCH sets from a printer attribute), so
// every skip below is a feature the bridge does not offer, and the reason says
// which attribute leaves it out. Steps that were skipped for a missing sample
// document no longer exist: the fetch script provides every document.

// skipRule accepts the skipped steps of one test file whose printed name
// matches pattern, and requires that exactly count steps match. Steps are
// attributed to the first rule that matches, so rules must not overlap.
type skipRule struct {
	suite   string
	pattern *regexp.Regexp
	count   int
	reason  string
}

// skipNames builds a rule for steps with exactly these printed names. A name
// that occurs twice in the test file is listed twice.
func skipNames(suite, reason string, names ...string) skipRule {
	quoted := make([]string, len(names))
	for i, name := range names {
		quoted[i] = regexp.QuoteMeta(name)
	}
	return skipRule{suite, regexp.MustCompile("^(" + strings.Join(quoted, "|") + ")$"), len(names), reason}
}

func (r skipRule) matches(step ipptoolStep) bool {
	return step.Suite == r.suite && r.pattern.MatchString(step.Name)
}

const (
	ipp11Suite       = "ipp-1.1.test"
	everywhereSuite  = "ipp-everywhere.test"
	rasterStepPrefix = `^Print .+ @ `
)

// noDuplexSkips is accepted only against a driver that offers no duplex: with
// one, sides-supported lists the two-sided keywords, OPTIONAL_DUPLEX is
// defined and these steps run.
var noDuplexSkips = skipNames(ipp11Suite, "sides-supported is one-sided only, so the bridge offers no duplex (OPTIONAL_DUPLEX undefined)",
	"Print-Job with A4 PDF, Duplex",
	"Print-Job with US Letter PDF, Duplex")

// expectedConformanceSkips lists every skipped step the bridge accepts from a
// driver that reports and accepts duplex printing.
var expectedConformanceSkips = []skipRule{
	skipNames(ipp11Suite, "Print-URI and Send-URI are not offered on purpose: taking a URL from a client would make "+
		"this PC fetch arbitrary addresses (SSRF). operations-supported omits them, so the suite never sends them "+
		"(OPTIONAL_PRINT_URI, OPTIONAL_SEND_URI undefined); a client that sends them anyway gets "+
		"server-error-operation-not-supported (TestUnsupportedOperationsAreNotSupported)",
		"RFC 8011 section 4.2.2: Print-URI Operation",
		"Print-URI with bad URI: Print-URI Operation",
		"RFC 8011 section 4.2.4: Create-Job Operation",
		"RFC 8011 section 4.3.2: Send-URI Operation",
		"Send-URI with bad URI: Create-Job Operation",
		"Send-URI with bad URI: Send-URI Operation (bad URI)",
		"Send-URI with bad URI: Cancel-Job Operation"),
	skipNames(ipp11Suite, "document-format-supported lacks application/postscript: the bridge renders PDF, raster and "+
		"images only (OPTIONAL_POSTSCRIPT undefined)",
		"Print-Job with A4 PostScript",
		"Print-Job with A4 PostScript, Duplex",
		"Print-Job with US Letter PostScript",
		"Print-Job with US Letter PostScript, Duplex"),
	skipNames(ipp11Suite, "media-supported has no 4x6 size, and print-quality-supported offers normal only "+
		"(OPTIONAL_4X6_MEDIA, OPTIONAL_DRAFT_QUALITY, OPTIONAL_HIGH_QUALITY undefined)",
		"Print-Job with Color JPEG on 4x6",
		"Print-Job with Grayscale JPEG on 4x6",
		"Print-Job with JPEG on 4x6, Draft Quality",
		"Print-Job with JPEG on 4x6, Normal Quality",
		"Print-Job with JPEG on 4x6, High Quality"),
	skipNames(ipp11Suite, "job-sheets-supported lacks standard: the bridge prints no banner pages (OPTIONAL_STANDARD_SHEET undefined)",
		"Print-Job with A4 PDF and Standard Sheet",
		"Print-Job with US Letter PDF and Standard Sheet",
		"Print-Job with A4 PDF and Standard Sheet",
		"Print-Job with US Letter PDF and Standard Sheet"),
	skipNames(ipp11Suite, "number-up-supported lacks 2: the bridge prints one page per sheet (OPTIONAL_2UP undefined)",
		"Print-Job with A4 PDF, 2-Up",
		"Print-Job with US Letter PDF, 2-Up",
		"Print-Job with A4 PDF, 2-Up",
		"Print-Job with US Letter PDF, 2-Up"),
	skipNames(ipp11Suite, "print-quality-supported offers normal only (OPTIONAL_DRAFT_QUALITY undefined)",
		"Print-Job with A4 PDF, Draft Quality",
		"Print-Job with US Letter PDF, Draft Quality"),
	skipNames(ipp11Suite, "operations-supported omits Hold-Job and Release-Job: the bridge has no held jobs (OPTIONAL_HOLD_JOB undefined)",
		"Print-Job with job-hold-until",
		"Release-Job"),
	{everywhereSuite, regexp.MustCompile(rasterStepPrefix + `(150|180|360|600|720)dpi, `), 360,
		"pwg-raster-document-resolution-supported lists 300 dpi only (HAVE_150DPI, HAVE_180DPI, HAVE_360DPI, " +
			"HAVE_600DPI, HAVE_720DPI undefined)"},
	{everywhereSuite, regexp.MustCompile(rasterStepPrefix + `300dpi, (black-1|cmyk-8|srgb-16)`), 39,
		"pwg-raster-document-type-supported lists sgray_8 and srgb_8 only (HAVE_BLACK_1, HAVE_CMYK_8, " +
			"HAVE_SRGB_16 undefined)"},
}

// skipProblems compares the skipped steps of a report with the run's
// expected count and with the accepted skip rules.
func skipProblems(report ipptoolReport, run conformanceRun, rules []skipRule) []string {
	var problems []string
	if skips := report.count(stepSkip); skips != run.skips {
		problems = append(problems, fmt.Sprintf("%s: %d steps skip, expected %d", run.suite, skips, run.skips))
	}
	matched := make([]int, len(rules))
	for _, step := range report.withStatus(stepSkip) {
		i := slices.IndexFunc(rules, func(r skipRule) bool { return r.matches(step) })
		if i < 0 {
			problems = append(problems, fmt.Sprintf("%s: %q now skips, it ran before: fix the cause or add a skipRule", step.Suite, step.Name))
			continue
		}
		matched[i]++
	}
	for i, rule := range rules {
		if slices.Contains(report.Suites, rule.suite) && matched[i] != rule.count {
			problems = append(problems, fmt.Sprintf("%s: %d steps skip for %q, expected %d: update the skipRule (%s)",
				rule.suite, matched[i], rule.pattern, rule.count, rule.reason))
		}
	}
	slices.Sort(problems)
	return problems
}

// acceptedDeviations explains, one line each, every failure and skip of the
// report that the allowlists accept, so the test log says why the bridge is
// not fully green.
func acceptedDeviations(report ipptoolReport, known map[conformanceStep]knownFailure, rules []skipRule) []string {
	var lines []string
	for _, step := range report.failures() {
		allowed := known[conformanceStep{step.Suite, step.Name}]
		for _, attr := range sortedUnique(step.Missing) {
			lines = append(lines, fmt.Sprintf("accepted failure: %s: %q lacks %s: %s", step.Suite, step.Name, attr, allowed[attr]))
		}
	}
	for _, rule := range rules {
		if slices.Contains(report.Suites, rule.suite) {
			lines = append(lines, fmt.Sprintf("accepted skip: %s: %d steps: %s", rule.suite, rule.count, rule.reason))
		}
	}
	return lines
}
