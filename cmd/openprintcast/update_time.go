package main

import (
	"time"

	"github.com/egoist/mygo/i18n"
)

// lastCheckedPrefix leads the relative time of the last check; day is the
// largest unit it counts in.
const (
	lastCheckedPrefix = "Last checked "
	day               = 24 * time.Hour
)

// relativeFormatter writes a time offset in the reader's language, as
// "5 minutes ago" (negative values are the past). cldrDates is one.
type relativeFormatter interface {
	Relative(value float64, unit i18n.RelativeTimeUnit) string
}

// lastCheckedText says how long before now a check finished, in whole
// minutes, hours or days, rounded down: "Last checked 5 minutes ago". It is
// empty when the time is unknown. A time in the future (a clock that was
// set back) reads as now.
func lastCheckedText(now, checkedAt time.Time, f relativeFormatter) string {
	if checkedAt.IsZero() {
		return ""
	}
	age := max(now.Sub(checkedAt), 0)
	var (
		value float64
		unit  i18n.RelativeTimeUnit
	)
	switch {
	case age < time.Minute:
		value, unit = 0, i18n.RelativeSecond
	case age < time.Hour:
		value, unit = float64(age/time.Minute), i18n.RelativeMinute
	case age < day:
		value, unit = float64(age/time.Hour), i18n.RelativeHour
	default:
		value, unit = float64(age/day), i18n.RelativeDay
	}
	return lastCheckedPrefix + f.Relative(-value, unit)
}
