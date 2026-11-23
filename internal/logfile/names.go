package logfile

import (
	"cmp"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	dateLayout = "20060102"
	nameSuffix = ".log"
	firstSeq   = 1
	minSeq     = firstSeq + 1 // sequence numbers are only written from 2 on
)

// logName identifies one log file: its day and its sequence within that day.
type logName struct {
	date string // YYYYMMDD, fixed width so lexical order equals date order
	seq  int
}

func (n logName) compare(o logName) int {
	if c := cmp.Compare(n.date, o.date); c != 0 {
		return c
	}
	return cmp.Compare(n.seq, o.seq)
}

func dayOf(t time.Time) string { return t.Format(dateLayout) }

// formatName builds "<prefix>-YYYYMMDD.log" or "<prefix>-YYYYMMDD-N.log" (N >= 2).
func formatName(prefix string, n logName) string {
	if n.seq < minSeq {
		return prefix + "-" + n.date + nameSuffix
	}
	return prefix + "-" + n.date + "-" + strconv.Itoa(n.seq) + nameSuffix
}

// parseName is the strict inverse of formatName; foreign names report false.
func parseName(prefix, name string) (logName, bool) {
	rest, ok := strings.CutPrefix(name, prefix+"-")
	if !ok {
		return logName{}, false
	}
	rest, ok = strings.CutSuffix(rest, nameSuffix)
	if !ok {
		return logName{}, false
	}
	date, seqText, hasSeq := strings.Cut(rest, "-")
	if _, err := time.Parse(dateLayout, date); err != nil {
		return logName{}, false
	}
	n := logName{date: date, seq: firstSeq}
	if hasSeq {
		seq, err := strconv.Atoi(seqText)
		if err != nil {
			return logName{}, false
		}
		n.seq = seq
	}
	return n, formatName(prefix, n) == name
}

// listNames returns the well-formed log files (not directories) in dir.
func listNames(dir, prefix string) ([]logName, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []logName
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if n, ok := parseName(prefix, e.Name()); ok {
			names = append(names, n)
		}
	}
	return names, nil
}
