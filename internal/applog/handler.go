package applog

import (
	"io"
	"log/slog"
)

const timeLayout = "2006-01-02 15:04:05.000"

// ignoreErrors swallows write errors, so a dead stderr (invalid handle in a
// GUI process) can never fail a logging call or stop an io.MultiWriter.
type ignoreErrors struct{ io.Writer }

func (w ignoreErrors) Write(p []byte) (int, error) {
	_, _ = w.Writer.Write(p)
	return len(p), nil
}

func newHandler(out io.Writer, level slog.Leveler) slog.Handler {
	return slog.NewTextHandler(out, &slog.HandlerOptions{
		Level:       level,
		ReplaceAttr: formatTime,
	})
}

// formatTime renders the top-level time attribute in local time.
func formatTime(groups []string, a slog.Attr) slog.Attr {
	if len(groups) == 0 && a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
		a.Value = slog.StringValue(a.Value.Time().Local().Format(timeLayout))
	}
	return a
}

// tolerant wraps w so its write errors are ignored.
func tolerant(w io.Writer) io.Writer {
	return ignoreErrors{w}
}
