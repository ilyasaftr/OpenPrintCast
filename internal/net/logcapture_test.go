package net

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"
)

// syncBuffer is a bytes.Buffer safe for concurrent logging.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// captureLogs installs a text handler at Debug level as the default logger
// and restores the previous default when the test ends.
func captureLogs(t *testing.T) *syncBuffer {
	t.Helper()
	out := &syncBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return out
}
