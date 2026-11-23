package main

import (
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/i18n"
)

// Skeletons are CLDR's field lists: the locale picks the order, the
// separators, the names and the hour cycle (j: 12 or 24 hours as the reader's
// regional formats say).
const (
	skeletonClock       = "jms"           // 14:05:09 or 2:05:09 PM
	skeletonClockShort  = "jm"            // 14:05 or 2:05 PM
	skeletonWeekday     = "E"             // Tue
	skeletonWeekdayTime = "Ejm"           // Tue 14:05
	skeletonDay         = "MMMd"          // 22 Sep
	skeletonDayTime     = "MMMdjm"        // 22 Sep, 14:05
	skeletonDate        = "yMMMd"         // 22 Sep 2025
	skeletonFull        = "yMMMMEEEEdjms" // Tuesday, 22 September 2026 at 14:05:09
)

// recency is how long ago a job was received, in calendar days.
type recency int

const (
	recentToday     recency = iota // today, or the future (a skewed clock)
	recentYesterday                // the day before today
	recentThisWeek                 // 2 to 6 days ago: the weekday says which
	recentThisYear                 // a week or more ago, in this year
	recentEarlier                  // in an earlier year
)

// daysThisWeek is the oldest age, in calendar days, that a weekday name
// still tells apart from the same weekday a week earlier.
const daysThisWeek = 6

// timeLayout is how one recency is written: the skeleton of its date and
// time ("" when it has none), led by the word for yesterday when it asks.
type timeLayout struct {
	skeleton      string
	yesterdayWord bool
}

// timeLayouts has one layout per recency.
type timeLayouts [recentEarlier + 1]timeLayout

// fullTimeLayouts is the Jobs table when it has room: the time of day
// whenever the day is recent enough to be named.
var fullTimeLayouts = timeLayouts{
	recentToday:     {skeleton: skeletonClock},
	recentYesterday: {skeleton: skeletonClockShort, yesterdayWord: true},
	recentThisWeek:  {skeleton: skeletonWeekdayTime},
	recentThisYear:  {skeleton: skeletonDayTime},
	recentEarlier:   {skeleton: skeletonDate},
}

// compactTimeLayouts is the narrow window's: the clock only for today, as
// Apple Mail's list does, the full time is in the tooltip.
var compactTimeLayouts = timeLayouts{
	recentToday:     {skeleton: skeletonClockShort},
	recentYesterday: {yesterdayWord: true},
	recentThisWeek:  {skeleton: skeletonWeekday},
	recentThisYear:  {skeleton: skeletonDay},
	recentEarlier:   {skeleton: skeletonDate},
}

const (
	// noJobTime stands for a job whose time is unknown.
	noJobTime = "—"
	// yesterdayGlue joins "Yesterday" to its time.
	yesterdayGlue = ", "
	// daysYesterday is the age of yesterday in calendar days.
	daysYesterday = 1
)

// dateFormatter writes dates in the reader's language and regional formats,
// so the Jobs table never hard-codes an order, a month name or a clock.
type dateFormatter interface {
	// Format writes t, in its own location, for a CLDR skeleton.
	Format(t time.Time, skeleton string) string
	// Yesterday is the word for the day before today, capitalized.
	Yesterday() string
}

// cldrDates is the dateFormatter of mygo's CLDR data: the formats locale
// (the app's language read with the reader's regional formats, see
// Region.FormatLocale) and the hour cycle the reader chose (0: the locale's).
type cldrDates struct {
	locale    string
	hourCycle int
}

// newCLDRDates writes the app's language with the reader's regional formats.
func newCLDRDates(language string, region i18n.Region) cldrDates {
	return cldrDates{locale: region.FormatLocale(language), hourCycle: region.HourCycle}
}

func (d cldrDates) Format(t time.Time, skeleton string) string {
	return i18n.FormatDate(d.locale, t, i18n.DateOptions{Skeleton: skeleton, HourCycle: d.hourCycle})
}

func (d cldrDates) Yesterday() string {
	word := i18n.FormatRelativeTime(d.locale, -daysYesterday, i18n.RelativeDay, i18n.RelativeTimeOptions{Auto: true})
	return capitalizeFirst(word)
}

// Relative writes a time offset as "5 minutes ago"; zero is "now".
func (d cldrDates) Relative(value float64, unit i18n.RelativeTimeUnit) string {
	return i18n.FormatRelativeTime(d.locale, value, unit, i18n.RelativeTimeOptions{Auto: true})
}

// capitalizeFirst upper-cases the first letter, rune-safely: a language
// whose letters have no case, or an empty word, comes back unchanged.
func capitalizeFirst(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

// daysBefore counts the calendar days from t's date to now's, by date, not
// by 24-hour spans, so midnight and DST changes cannot shift it. Both are
// read in now's location; a t in the future gives a negative count.
func daysBefore(now, t time.Time) int {
	today := midnightUTC(now)
	then := midnightUTC(t.In(now.Location()))
	return int(today.Sub(then).Hours() / 24)
}

// midnightUTC is the date of t as a UTC midnight, which has no DST.
func midnightUTC(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// recencyOf classifies t, in now's location, by calendar days before now.
func recencyOf(now, t time.Time) recency {
	days := daysBefore(now, t)
	switch {
	case days <= 0:
		return recentToday
	case days == daysYesterday:
		return recentYesterday
	case days <= daysThisWeek:
		return recentThisWeek
	case t.Year() == now.Year():
		return recentThisYear
	}
	return recentEarlier
}

// formatJobTime says when a job was received the way Apple Mail and Messages
// date their items, as short as its age allows, in the given layouts (see
// fullTimeLayouts): today the clock, yesterday its name, within the last week
// the weekday, this year the day and month, earlier years the date. An
// unknown time is a dash.
func formatJobTime(unix int64, now time.Time, f dateFormatter, layouts timeLayouts) string {
	if unix <= 0 {
		return noJobTime
	}
	t := time.Unix(unix, 0).In(now.Location())
	layout := layouts[recencyOf(now, t)]
	switch {
	case !layout.yesterdayWord:
		return f.Format(t, layout.skeleton)
	case layout.skeleton == "":
		return f.Yesterday()
	}
	return f.Yesterday() + yesterdayGlue + f.Format(t, layout.skeleton)
}

// formatJobTimeDetail is the complete, unambiguous date and time of a job,
// with the weekday, the month spelled out, the year and the seconds: the
// tooltip and accessible description of the abbreviated cell.
func formatJobTimeDetail(unix int64, now time.Time, f dateFormatter) string {
	if unix <= 0 {
		return noJobTime
	}
	return f.Format(time.Unix(unix, 0).In(now.Location()), skeletonFull)
}

// jobTimes dates every cell of one frame of the Jobs table against a single
// reading of the clock, so no two rows disagree about what "today" is.
type jobTimes struct {
	now     time.Time
	dates   dateFormatter
	layouts timeLayouts
}

// label is the abbreviated time shown in a cell.
func (j jobTimes) label(unix int64) string { return formatJobTime(unix, j.now, j.dates, j.layouts) }

// detail is the cell's full date and time.
func (j jobTimes) detail(unix int64) string { return formatJobTimeDetail(unix, j.now, j.dates) }
