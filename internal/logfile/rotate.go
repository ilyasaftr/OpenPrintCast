package logfile

import (
	"os"
	"time"
)

func (w *Writer) rollIfNeeded(incoming int64) {
	now := w.opts.Now()
	if !w.shouldRoll(now, incoming) || now.Before(w.retryAt) {
		return
	}
	if err := w.switchTo(w.nextName(now)); err != nil {
		w.opts.OnError(err)
		w.retryAt = now.Add(retryInterval)
		return
	}
	w.retryAt = time.Time{}
	w.prune()
}

func (w *Writer) shouldRoll(now time.Time, incoming int64) bool {
	if dayOf(now) != w.name.date {
		return true
	}
	return w.size > 0 && w.size+incoming > w.opts.MaxBytes
}

// nextName is the first file of a new day or the next sequence of the same day.
func (w *Writer) nextName(now time.Time) logName {
	if day := dayOf(now); day != w.name.date {
		return w.newestFor(day)
	}
	return logName{date: w.name.date, seq: w.name.seq + 1}
}

// newestFor returns the highest existing sequence of day, or its first file.
func (w *Writer) newestFor(day string) logName {
	newest := logName{date: day, seq: firstSeq}
	names, err := listNames(w.opts.Dir, w.opts.Prefix)
	if err != nil {
		w.opts.OnError(err)
		return newest
	}
	for _, n := range names {
		if n.date == day && n.seq > newest.seq {
			newest = n
		}
	}
	return newest
}

// switchTo opens name (skipping files that are already full) and only then
// closes the previous file, so a failure leaves the old file in use.
func (w *Writer) switchTo(name logName) error {
	f, size, name, err := w.openAvailable(name)
	if err != nil {
		return err
	}
	old := w.file
	w.file, w.size, w.name = f, size, name
	if old != nil {
		if err := old.Close(); err != nil {
			w.opts.OnError(err)
		}
	}
	return nil
}

func (w *Writer) openAvailable(name logName) (*os.File, int64, logName, error) {
	for {
		f, size, err := openAppend(w.path(name))
		if err != nil {
			return nil, 0, name, err
		}
		if size < w.opts.MaxBytes {
			return f, size, name, nil
		}
		f.Close()
		name.seq++
	}
}

func openAppend(path string) (*os.File, int64, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, filePerm)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}
