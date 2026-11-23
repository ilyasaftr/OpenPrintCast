package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// tableCellPadding is what a table cell keeps free around its text: a text
// is cut with an ellipsis when it is wider than its column minus this.
const tableCellPadding = 20

// newReceivedApp is a Jobs page with one job per recency, as seen at
// jobsNow, in the given language and regional formats.
func newReceivedApp(t *testing.T, r reader) (*bridgeApp, *ui.Tester) {
	t.Helper()
	app := newTestApp(t)
	app.clock = func() time.Time { return jobsNow }
	statuses := []core.JobStatus{core.JobPrinting, core.JobFailed, core.JobCompleted, core.JobCancelled, core.JobCompleted}
	var jobs []core.JobEntry
	for i := len(sampleTimes) - 1; i >= 0; i-- { // the log is oldest first
		job := storedJob(fmt.Sprint("job", i), fmt.Sprint("doc", i, ".pdf"), statuses[i])
		job.CreatedAt = sampleTimes[i].Unix()
		jobs = append(jobs, job)
	}
	log := &core.JobLog{}
	log.Restore(jobs)
	app.shared = bridgenet.NewSharedState(app.printers, log)
	app.tab = tabJobs
	tt := ui.NewTester(app.view, 860, 620)
	tt.SetLocale(r.language)
	tt.SetRegion(r.region)
	return app, tt
}

// receivedLabels are what the Received cells of newReceivedApp say.
func receivedLabels(r reader, layouts timeLayouts) []string {
	var labels []string
	for _, when := range sampleTimes {
		labels = append(labels, formatJobTime(when.Unix(), jobsNow, r.dates(), layouts))
	}
	return labels
}

func TestReceivedColumnShowsEveryTierWithItsFullTimeAsDescription(t *testing.T) {
	for _, r := range []reader{readerEN24, readerENUS, readerID, readerDE, readerJA} {
		_, tt := newReceivedApp(t, r)
		if !tt.HasText("Received") || tt.HasText("Time") {
			t.Errorf("%s: the column must be headed Received: %q", r.name, tt.Texts())
		}
		described := map[string]string{}
		for _, a := range tt.AccessAll() {
			described[a.Name] = a.Description
		}
		for i, label := range receivedLabels(r, fullTimeLayouts) {
			if !tt.HasText(label) {
				t.Errorf("%s: no cell says %q; texts: %q", r.name, label, tt.Texts())
			}
			want := formatJobTimeDetail(sampleTimes[i].Unix(), jobsNow, r.dates())
			if got := described[label]; got != want {
				t.Errorf("%s: %q describes itself as %q, want %q", r.name, label, got, want)
			}
		}
	}
}

func TestReceivedColumnStaysFirstAndFollowsTheRegionLive(t *testing.T) {
	_, tt := newReceivedApp(t, readerEN24)
	received, _ := tt.Find("Received")
	document, _ := tt.Find("Document")
	if received.X >= document.X {
		t.Errorf("Received (x=%v) must come before Document (x=%v)", received.X, document.X)
	}
	if !tt.HasText("09:05:07") {
		t.Fatalf("24-hour clock missing: %q", tt.Texts())
	}
	tt.SetRegion(i18n.Region{Locale: "en-US", HourCycle: 12})
	if !tt.HasText("9:05:07 AM") || tt.HasText("09:05:07") {
		t.Errorf("the table did not follow the region change: %q", tt.Texts())
	}
}

func TestReceivedCompactLayoutKeepsEveryColumnAndAbbreviates(t *testing.T) {
	for _, r := range []reader{readerEN24, readerDE, readerJA} {
		_, tt := newReceivedApp(t, r)
		tt.SetSize(640, 460)
		for _, label := range append(receivedLabels(r, compactTimeLayouts), "Received", "Document", "Printer", "Status", "Cancelled") {
			rect, ok := tt.Find(label)
			if !ok || rect.X < 0 || rect.X+rect.W > 640 {
				t.Errorf("%s: %q is missing or outside the window: %v", r.name, label, rect)
			}
		}
		if _, ok := tt.Access("Cancel job doc0.pdf"); !ok {
			t.Errorf("%s: the compact layout lost the Cancel button", r.name)
		}
	}
}

// textMeter measures the width of single-line texts as the table draws them
// in a language, whose glyphs may differ.
func textMeter(language string) func(string) float32 {
	var text string
	var width float32
	tt := ui.NewTester(func(c *ui.Context) {
		width, _ = c.MeasureText(0, ui.Span{Text: text})
		ui.Text(c, "")
	}, 300, 100)
	tt.SetLocale(language)
	return func(s string) float32 {
		text = s
		tt.Frame()
		return width
	}
}

func TestReceivedLabelsFitTheirColumnsInEveryLayout(t *testing.T) {
	readers := []reader{readerEN24, readerENUS, readerID, readerDE, readerJA,
		{"id 12h", "id", i18n.Region{HourCycle: 12}}, {"de 12h", "de", i18n.Region{HourCycle: 12}},
		{"ja 12h", "ja", i18n.Region{HourCycle: 12}}, {"en-GB 12h", "en", i18n.Region{Locale: "en-GB", HourCycle: 12}}}
	for _, layout := range []struct {
		name    string
		layouts timeLayouts
		column  float32
	}{{"full", fullTimeLayouts, jobReceivedWidth}, {"compact", compactTimeLayouts, jobReceivedNarrow}} {
		for _, r := range readers {
			widest, text := widestLabel(r, layout.layouts, textMeter(r.language))
			if limit := layout.column - tableCellPadding; widest > limit {
				t.Errorf("%s layout, %s: %q is %v wide, the column holds %v", layout.name, r.name, text, widest, limit)
			}
		}
	}
}

// widestLabel sweeps every month, several hours and the days of a week to
// find the widest label a reader can see.
func widestLabel(r reader, layouts timeLayouts, width func(string) float32) (float32, string) {
	var widest float32
	var text string
	for _, year := range []int{2025, 2026} {
		for month := time.January; month <= time.December; month++ {
			for _, day := range []int{16, 18, 21, 22} {
				for _, hour := range []int{0, 12, 23} {
					when := time.Date(year, month, day, hour, 38, 38, 0, time.UTC)
					s := formatJobTime(when.Unix(), jobsNow, r.dates(), layouts)
					if w := width(s); w > widest {
						widest, text = w, s
					}
				}
			}
		}
	}
	return widest, text
}

func TestReceivedCellsAreNotClippedAtTheDefaultFont(t *testing.T) {
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		for _, r := range []reader{readerENUS, readerID, readerDE, readerJA} {
			width := textMeter(r.language)
			_, tt := newReceivedApp(t, r)
			tt.SetSize(size[0], size[1])
			layouts := fullTimeLayouts
			if size[0] < wideJobsWidth {
				layouts = compactTimeLayouts
			}
			for _, label := range receivedLabels(r, layouts) {
				rect, ok := tt.Find(label)
				if !ok || rect.W < width(label)-0.5 {
					t.Errorf("%dx%d %s: %q is cut to %v of %v", size[0], size[1], r.name, label, rect.W, width(label))
				}
			}
		}
	}
}

func TestReceivedRowsDoNotShift(t *testing.T) {
	_, tt := newReceivedApp(t, readerENUS)
	var xs []float32
	for _, label := range receivedLabels(readerENUS, fullTimeLayouts) {
		rect, _ := tt.Find(label)
		xs = append(xs, rect.X)
	}
	for _, x := range xs {
		if x != xs[0] {
			t.Fatalf("Received cells start at different x: %v", xs)
		}
	}
}

func TestSnapshotJobsAcrossEveryRecency(t *testing.T) {
	_, tt := newReceivedApp(t, readerENUS)
	snapshot(t, tt, "jobs-dates")
	tt.SetDark(true)
	snapshot(t, tt, "jobs-dates-dark")
	tt.SetSize(640, 460)
	snapshot(t, tt, "jobs-dates-min-dark")
	tt.SetDark(false)
	snapshot(t, tt, "jobs-dates-min")
	for _, r := range []reader{readerDE, readerID, readerJA} {
		tt.SetLocale(r.language)
		tt.SetRegion(r.region)
		snapshot(t, tt, "jobs-dates-min-"+r.language)
		tt.SetSize(860, 620)
		snapshot(t, tt, "jobs-dates-"+r.language)
		tt.SetSize(640, 460)
	}
}
