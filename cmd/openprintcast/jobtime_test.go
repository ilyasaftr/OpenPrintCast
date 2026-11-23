package main

import (
	"testing"
	"time"
	_ "time/tzdata" // the DST tests need zones even on a machine without zoneinfo

	"github.com/egoist/mygo/i18n"
)

// A reader of a language with the regional formats they chose.
type reader struct {
	name     string
	language string
	region   i18n.Region
}

var (
	readerEN24 = reader{"en 24h", "en", i18n.Region{HourCycle: 24}}
	readerENUS = reader{"en-US 12h", "en", i18n.Region{Locale: "en-US", HourCycle: 12}}
	readerID   = reader{"id", "id", i18n.Region{}}
	readerDE   = reader{"de", "de", i18n.Region{}}
	readerJA   = reader{"ja", "ja", i18n.Region{}}
)

func (r reader) dates() dateFormatter { return newCLDRDates(r.language, r.region) }

// jobsNow is a Tuesday afternoon; the sample times below are one per recency.
var jobsNow = time.Date(2026, 9, 22, 15, 0, 0, 0, time.UTC)

var (
	sampleToday     = time.Date(2026, 9, 22, 9, 5, 7, 0, time.UTC)
	sampleYesterday = time.Date(2026, 9, 21, 23, 59, 30, 0, time.UTC)
	sampleThisWeek  = time.Date(2026, 9, 18, 8, 30, 0, 0, time.UTC)
	sampleThisYear  = time.Date(2026, 3, 4, 7, 8, 0, 0, time.UTC)
	sampleEarlier   = time.Date(2025, 12, 31, 22, 0, 0, 0, time.UTC)
	sampleTimes     = []time.Time{sampleToday, sampleYesterday, sampleThisWeek, sampleThisYear, sampleEarlier}
)

func TestFormatJobTimeTiers(t *testing.T) {
	for _, tc := range []struct {
		reader  reader
		layouts timeLayouts
		want    [5]string // today, yesterday, this week, this year, earlier
	}{
		{readerEN24, fullTimeLayouts, [5]string{"09:05:07", "Yesterday, 23:59", "Fri 08:30", "Mar 4, 07:08", "Dec 31, 2025"}},
		{readerENUS, fullTimeLayouts, [5]string{"9:05:07 AM", "Yesterday, 11:59 PM", "Fri 8:30 AM", "Mar 4, 7:08 AM", "Dec 31, 2025"}},
		{readerID, fullTimeLayouts, [5]string{"09.05.07", "Kemarin, 23.59", "Jum 08.30", "4 Mar, 07.08", "31 Des 2025"}},
		{readerDE, fullTimeLayouts, [5]string{"09:05:07", "Gestern, 23:59", "Fr., 08:30", "4. März, 07:08", "31. Dez. 2025"}},
		{readerJA, fullTimeLayouts, [5]string{"9:05:07", "昨日, 23:59", "8:30 (金)", "3月4日 7:08", "2025年12月31日"}},
		{readerEN24, compactTimeLayouts, [5]string{"09:05", "Yesterday", "Fri", "Mar 4", "Dec 31, 2025"}},
		{readerENUS, compactTimeLayouts, [5]string{"9:05 AM", "Yesterday", "Fri", "Mar 4", "Dec 31, 2025"}},
		{readerID, compactTimeLayouts, [5]string{"09.05", "Kemarin", "Jum", "4 Mar", "31 Des 2025"}},
		{readerDE, compactTimeLayouts, [5]string{"09:05", "Gestern", "Fr", "4. März", "31. Dez. 2025"}},
		{readerJA, compactTimeLayouts, [5]string{"9:05", "昨日", "金", "3月4日", "2025年12月31日"}},
	} {
		for i, when := range sampleTimes {
			if got := formatJobTime(when.Unix(), jobsNow, tc.reader.dates(), tc.layouts); got != tc.want[i] {
				t.Errorf("%s tier %d: got %q, want %q", tc.reader.name, i, got, tc.want[i])
			}
		}
	}
}

func TestFormatJobTimeDetailSaysEverything(t *testing.T) {
	for _, tc := range []struct {
		reader reader
		when   time.Time
		want   string
	}{
		{readerEN24, sampleToday, "Tuesday, September 22, 2026 at 09:05:07"},
		{readerENUS, sampleEarlier, "Wednesday, December 31, 2025 at 10:00:00 PM"},
		{readerID, sampleYesterday, "Senin, 21 September 2026 pukul 23.59.30"},
		{readerDE, sampleThisWeek, "Freitag, 18. September 2026 um 08:30:00"},
		{readerJA, sampleThisYear, "2026年3月4日水曜日 7:08:00"},
	} {
		if got := formatJobTimeDetail(tc.when.Unix(), jobsNow, tc.reader.dates()); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.reader.name, got, tc.want)
		}
	}
}

func TestRegionalFormatsDecideOrderAndClockNotTheLanguage(t *testing.T) {
	german := reader{"en with German formats", "en", i18n.Region{Locale: "de-DE", HourCycle: 24}}
	got := formatJobTime(sampleThisYear.Unix(), jobsNow, german.dates(), fullTimeLayouts)
	if want := "4 Mar, 07:08"; got != want {
		t.Errorf("English words in the day-first order of Germany: got %q, want %q", got, want)
	}
	twelve := reader{"de with 12 hours", "de", i18n.Region{HourCycle: 12}}
	got = formatJobTime(sampleToday.Unix(), jobsNow, twelve.dates(), fullTimeLayouts)
	if want := "9:05:07 AM"; got != want {
		t.Errorf("the reader's 12-hour clock must win over the locale's: got %q, want %q", got, want)
	}
}

func TestFormatJobTimeBoundaries(t *testing.T) {
	d := readerEN24.dates()
	at := func(y int, m time.Month, day, h, min int) time.Time {
		return time.Date(y, m, day, h, min, 0, 0, time.UTC)
	}
	for _, tc := range []struct {
		name      string
		now, when time.Time
		want      string
	}{
		{"23:59 the day before is yesterday", at(2026, 9, 22, 0, 1), at(2026, 9, 21, 23, 59), "Yesterday, 23:59"},
		{"00:01 the same day is today", at(2026, 9, 22, 23, 59), at(2026, 9, 22, 0, 1), "00:01:00"},
		{"the last day of a month is yesterday", at(2026, 10, 1, 9, 0), at(2026, 9, 30, 18, 0), "Yesterday, 18:00"},
		{"31 Dec is yesterday on 1 Jan", at(2027, 1, 1, 0, 30), at(2026, 12, 31, 23, 59), "Yesterday, 23:59"},
		{"30 Dec is a weekday on 1 Jan", at(2027, 1, 1, 0, 30), at(2026, 12, 30, 12, 0), "Wed 12:00"},
		{"six days ago still names the weekday", at(2026, 9, 22, 9, 0), at(2026, 9, 16, 12, 0), "Wed 12:00"},
		{"seven days ago has a date", at(2026, 9, 22, 9, 0), at(2026, 9, 15, 12, 0), "Sep 15, 12:00"},
		{"1 Jan of this year has a date and time", at(2026, 9, 22, 9, 0), at(2026, 1, 1, 8, 0), "Jan 1, 08:00"},
		{"a week back across new year is the earlier year", at(2027, 1, 3, 9, 0), at(2026, 12, 26, 8, 0), "Dec 26, 2026"},
		{"a clock ahead of ours is today", at(2026, 9, 22, 9, 0), at(2026, 9, 22, 9, 4), "09:04:00"},
		{"a day ahead is still today", at(2026, 9, 22, 9, 0), at(2026, 9, 23, 9, 0), "09:00:00"},
		{"a year ahead is still today", at(2026, 9, 22, 9, 0), at(2027, 9, 22, 9, 0), "09:00:00"},
	} {
		if got := formatJobTime(tc.when.Unix(), tc.now, d, fullTimeLayouts); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestFormatJobTimeCountsCalendarDaysAcrossDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	d := readerEN24.dates()
	for _, tc := range []struct {
		name      string
		now, when time.Time
		want      string
	}{
		// Spring forward: 23 hours from 03:30 to 03:30, still yesterday.
		{"23 hours but yesterday", time.Date(2026, 3, 8, 3, 30, 0, 0, ny), time.Date(2026, 3, 7, 3, 30, 0, 0, ny), "Yesterday, 03:30"},
		// Fall back: 25 hours from 23:50 on the 31st to 00:10 on the 2nd, two days.
		{"25 hours but two days", time.Date(2026, 11, 2, 0, 10, 0, 0, ny), time.Date(2026, 10, 31, 23, 50, 0, 0, ny), "Sat 23:50"},
		// The 25-hour day: 00:30 EDT and 23:30 EST of 1 Nov are both that day.
		{"the longest day is one day", time.Date(2026, 11, 1, 23, 30, 0, 0, ny), time.Date(2026, 11, 1, 0, 30, 0, 0, ny), "00:30:00"},
		{"a zone ahead of UTC reads the job in its own zone", time.Date(2026, 9, 22, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)), time.Date(2026, 9, 21, 20, 0, 0, 0, time.UTC), "04:00:00"},
	} {
		if got := formatJobTime(tc.when.Unix(), tc.now, d, fullTimeLayouts); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestUnknownJobTimeIsADash(t *testing.T) {
	d := readerEN24.dates()
	for _, unix := range []int64{0, -1, -86400 * 365} {
		if got := formatJobTime(unix, jobsNow, d, fullTimeLayouts); got != noJobTime {
			t.Errorf("unix %d: time = %q", unix, got)
		}
		if got := formatJobTimeDetail(unix, jobsNow, d); got != noJobTime {
			t.Errorf("unix %d: detail = %q", unix, got)
		}
	}
}

func TestCapitalizeFirst(t *testing.T) {
	for in, want := range map[string]string{
		"yesterday": "Yesterday",
		"kemarin":   "Kemarin",
		"éhier":     "Éhier",
		"вчера":     "Вчера",
		"昨日":        "昨日",
		"أمس":       "أمس",
		"":          "",
	} {
		if got := capitalizeFirst(in); got != want {
			t.Errorf("capitalizeFirst(%q) = %q, want %q", in, got, want)
		}
	}
}
