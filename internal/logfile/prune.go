package logfile

import "slices"

// prune deletes the oldest log files beyond Keep, never the one being written.
// Failed deletions are reported and naturally retried at the next rollover.
func (w *Writer) prune() {
	names, err := listNames(w.opts.Dir, w.opts.Prefix)
	if err != nil {
		w.opts.OnError(err)
		return
	}
	slices.SortFunc(names, logName.compare)
	excess := len(names) - w.opts.Keep
	for _, n := range names {
		if excess <= 0 {
			return
		}
		if n == w.name {
			continue
		}
		if err := w.remove(w.path(n)); err != nil {
			w.opts.OnError(err)
		}
		excess--
	}
}
