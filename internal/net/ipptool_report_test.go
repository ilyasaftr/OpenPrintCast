package net

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// ipptoolStatus is the result ipptool -t prints at the end of a step line.
type ipptoolStatus string

const (
	stepPass ipptoolStatus = "PASS"
	stepFail ipptoolStatus = "FAIL"
	stepSkip ipptoolStatus = "SKIP"
)

// ipptoolStep is one test step of an ipptool -t report.
type ipptoolStep struct {
	Suite  string // base name of the test file the step belongs to
	Name   string // as printed: ipptool cuts names at 68 characters
	Status ipptoolStatus
	// Missing lists attributes named by "EXPECTED: <attribute>" lines.
	Missing []string
	// Other holds every further failure detail, except the received size
	// and status code, which change without meaning.
	Other []string
}

// ipptoolReport is a parsed ipptool -t output.
type ipptoolReport struct {
	Steps []ipptoolStep
	// Suites are the test files ipptool started, including included ones.
	Suites []string
	// Diagnostics are lines ipptool itself printed ("ipptool: ...").
	Diagnostics []string
}

var (
	ipptoolSuiteLine  = regexp.MustCompile(`^"(.+)":$`)
	ipptoolStepLine   = regexp.MustCompile(`^    (\S.*?)\s+\[(PASS|FAIL|SKIP)\]$`)
	ipptoolDetailLine = regexp.MustCompile(`^        (\S.*)$`)
	ipptoolMissing    = regexp.MustCompile(`^EXPECTED: ([A-Za-z0-9-]+)$`)
)

// parseIpptoolOutput parses ipptool -t output. It is pure and tolerant:
// lines it does not recognize are ignored.
func parseIpptoolOutput(output string) ipptoolReport {
	var report ipptoolReport
	suite := ""
	current := -1 // index of the step detail lines belong to
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case ipptoolSuiteLine.MatchString(line):
			suite = filepath.Base(ipptoolSuiteLine.FindStringSubmatch(line)[1])
			report.Suites = append(report.Suites, suite)
			current = -1
		case ipptoolStepLine.MatchString(line):
			m := ipptoolStepLine.FindStringSubmatch(line)
			report.Steps = append(report.Steps, ipptoolStep{Suite: suite, Name: m[1], Status: ipptoolStatus(m[2])})
			current = len(report.Steps) - 1
		case strings.HasPrefix(line, "ipptool:"):
			report.Diagnostics = append(report.Diagnostics, line)
			current = -1
		case current >= 0 && report.Steps[current].Status == stepFail && ipptoolDetailLine.MatchString(line):
			report.Steps[current].addDetail(ipptoolDetailLine.FindStringSubmatch(line)[1])
		}
	}
	return report
}

func (s *ipptoolStep) addDetail(detail string) {
	switch m := ipptoolMissing.FindStringSubmatch(detail); {
	case m != nil:
		s.Missing = append(s.Missing, m[1])
	case strings.HasPrefix(detail, "RECEIVED: "), strings.HasPrefix(detail, "status-code = "):
	default:
		s.Other = append(s.Other, detail)
	}
}

func (r ipptoolReport) count(status ipptoolStatus) int {
	return len(r.withStatus(status))
}

// withStatus returns the steps that ended with the given status.
func (r ipptoolReport) withStatus(status ipptoolStatus) []ipptoolStep {
	var steps []ipptoolStep
	for _, step := range r.Steps {
		if step.Status == status {
			steps = append(steps, step)
		}
	}
	return steps
}

// failures returns the failed steps with duplicates of the same suite and
// name merged, so an allowlist can key on suite and step name.
func (r ipptoolReport) failures() []ipptoolStep {
	var merged []ipptoolStep
	for _, step := range r.Steps {
		if step.Status != stepFail {
			continue
		}
		i := slices.IndexFunc(merged, func(m ipptoolStep) bool { return m.Suite == step.Suite && m.Name == step.Name })
		if i < 0 {
			merged = append(merged, step)
			continue
		}
		merged[i].Missing = append(merged[i].Missing, step.Missing...)
		merged[i].Other = append(merged[i].Other, step.Other...)
	}
	return merged
}
