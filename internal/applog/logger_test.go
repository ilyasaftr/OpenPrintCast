package applog

import (
	"bytes"
	"errors"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var fixedNow = func() time.Time { return time.Date(2024, time.March, 9, 10, 0, 0, 0, time.Local) }

// isolate restores the process-wide loggers after the test.
func isolate(t *testing.T) {
	t.Helper()
	prevLogger, prevOut, prevFlags, prevPrefix := slog.Default(), log.Writer(), log.Flags(), log.Prefix()
	t.Cleanup(func() {
		slog.SetDefault(prevLogger)
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
	})
}

func initTest(t *testing.T, opts Options, tune fileTuning) *Logger {
	t.Helper()
	isolate(t)
	if opts.Dir == "" {
		opts.Dir = t.TempDir()
	}
	if opts.Stderr == nil {
		opts.Stderr = io.Discard
	}
	if opts.Now == nil {
		opts.Now = fixedNow
	}
	l, err := initLogger(opts, tune)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func readLog(t *testing.T, l *Logger) string {
	t.Helper()
	b, err := os.ReadFile(l.CurrentPath())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

type failingWriter struct{ calls int }

func (w *failingWriter) Write([]byte) (int, error) {
	w.calls++
	return 0, errors.New("invalid handle")
}

func TestInitWritesSlogAndStdlibLines(t *testing.T) {
	var mirror bytes.Buffer
	l := initTest(t, Options{Stderr: &mirror}, fileTuning{})
	slog.Info("hello", "k", "v")
	log.Printf("legacy %d", 7)

	got := readLog(t, l)
	slogLine := regexp.MustCompile(`(?m)^time="\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d{3}" level=INFO msg=hello k=v$`)
	stdLine := regexp.MustCompile(`(?m)^time="\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d{3}" level=INFO msg="legacy 7"$`)
	if !slogLine.MatchString(got) || !stdLine.MatchString(got) {
		t.Fatalf("unexpected log content:\n%s", got)
	}
	if strings.Contains(got, "source=") {
		t.Fatalf("source must not be logged:\n%s", got)
	}
	if mirror.String() != got {
		t.Fatalf("mirror differs from file:\n%q\n%q", mirror.String(), got)
	}
	if filepath.Base(l.CurrentPath()) != "openprintcast-20240309.log" {
		t.Fatalf("CurrentPath = %q", l.CurrentPath())
	}
}

func TestLevelFilteringAndLiveSwitch(t *testing.T) {
	l := initTest(t, Options{}, fileTuning{})
	if l.Verbose() {
		t.Fatal("default must not be verbose")
	}
	slog.Debug("hidden-1")
	l.SetVerbose(true)
	if !l.Verbose() {
		t.Fatal("Verbose() = false after SetVerbose(true)")
	}
	slog.Debug("shown")
	l.SetVerbose(false)
	slog.Debug("hidden-2")
	slog.Info("info")

	got := readLog(t, l)
	if !strings.Contains(got, "msg=shown") || !strings.Contains(got, "msg=info") ||
		strings.Contains(got, "hidden") {
		t.Fatalf("unexpected content:\n%s", got)
	}
}

func TestVerboseOptionStartsAtDebug(t *testing.T) {
	l := initTest(t, Options{Verbose: true}, fileTuning{})
	slog.Debug("dbg")
	if !l.Verbose() || !strings.Contains(readLog(t, l), "level=DEBUG msg=dbg") {
		t.Fatalf("verbose start failed:\n%s", readLog(t, l))
	}
}

func TestFailingMirrorDoesNotStopFileLogging(t *testing.T) {
	bad := &failingWriter{}
	l := initTest(t, Options{Stderr: bad}, fileTuning{})
	slog.Info("one")
	log.Print("two")
	slog.Info("three")
	got := readLog(t, l)
	for _, want := range []string{"msg=one", "msg=two", "msg=three"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s:\n%s", want, got)
		}
	}
	if bad.calls != 3 {
		t.Fatalf("mirror calls = %d, want 3", bad.calls)
	}
}

func TestIgnoreErrorsReportsFullWrite(t *testing.T) {
	n, err := ignoreErrors{&failingWriter{}}.Write([]byte("abc"))
	if n != 3 || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
}

func TestUnusableDirFallsBackToTempDir(t *testing.T) {
	temp := t.TempDir()
	prev := tempDir
	tempDir = func() string { return temp }
	t.Cleanup(func() { tempDir = prev })
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	l := initTest(t, Options{Dir: filepath.Join(blocker, "logs")}, fileTuning{})
	slog.Info("fallback")
	if filepath.Dir(l.CurrentPath()) != filepath.Join(temp, "OpenPrintCast") {
		t.Fatalf("CurrentPath = %q", l.CurrentPath())
	}
	if !strings.Contains(readLog(t, l), "msg=fallback") {
		t.Fatal("fallback file missing the line")
	}
}

func TestInitFailsWhenNothingIsUsable(t *testing.T) {
	isolate(t)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	prev := tempDir
	tempDir = func() string { return blocker }
	t.Cleanup(func() { tempDir = prev })
	before := slog.Default()
	if _, err := Init(Options{Dir: filepath.Join(blocker, "x"), Stderr: io.Discard}); err == nil {
		t.Fatal("expected error")
	}
	if slog.Default() != before {
		t.Fatal("default logger changed despite failure")
	}
}

func TestInitDefaultDirUsesLocalAppData(t *testing.T) {
	local := t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	isolate(t)
	l, err := Init(Options{Stderr: io.Discard, Now: fixedNow})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if filepath.Dir(l.CurrentPath()) != filepath.Join(local, "OpenPrintCast") {
		t.Fatalf("CurrentPath = %q", l.CurrentPath())
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("LOCALAPPDATA", filepath.Join("base", "Local"))
	if got, want := DefaultDir(), filepath.Join("base", "Local", "OpenPrintCast"); got != want {
		t.Fatalf("DefaultDir = %q, want %q", got, want)
	}
	t.Setenv("LOCALAPPDATA", "")
	if got, want := DefaultDir(), filepath.Join(os.TempDir(), "OpenPrintCast"); got != want {
		t.Fatalf("DefaultDir fallback = %q, want %q", got, want)
	}
}

func TestCurrentPathFollowsRollover(t *testing.T) {
	l := initTest(t, Options{}, fileTuning{maxBytes: 200, keep: 10})
	first := l.CurrentPath()
	for i := 0; i < 10; i++ {
		slog.Info("rolling line", "i", i)
	}
	seq := regexp.MustCompile(`openprintcast-20240309-\d+\.log$`)
	if l.CurrentPath() == first || !seq.MatchString(l.CurrentPath()) {
		t.Fatalf("CurrentPath did not follow rollover: %q", l.CurrentPath())
	}
	if !strings.Contains(readLog(t, l), "msg=\"rolling line\"") {
		t.Fatal("latest file has no lines")
	}
}

func TestCloseRestoresDefaultAndIsIdempotent(t *testing.T) {
	l := initTest(t, Options{}, fileTuning{})
	slog.Info("before")
	path := l.CurrentPath()
	if slog.Default().Handler() != l.handler {
		t.Fatal("handler not installed")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if slog.Default().Handler() == l.handler {
		t.Fatal("default logger not restored")
	}
	before, _ := os.ReadFile(path)
	slog.Info("after")
	log.Print("after std")
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatalf("file written after Close:\n%s", after)
	}
}

func TestCloseLeavesForeignHandlerInPlace(t *testing.T) {
	l := initTest(t, Options{}, fileTuning{})
	other := slog.New(slog.NewTextHandler(io.Discard, nil))
	slog.SetDefault(other)
	l.Close()
	if slog.Default() != other {
		t.Fatal("Close replaced a handler it does not own")
	}
}

func TestConcurrentLogging(t *testing.T) {
	const goroutines, perGoroutine = 8, 100
	l := initTest(t, Options{Verbose: true}, fileTuning{maxBytes: 1024, keep: 1000})
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				slog.Info("line", "g", g, "i", i)
				if i%10 == 0 {
					l.SetVerbose(i%20 == 0)
					_ = l.Verbose()
					_ = l.CurrentPath()
				}
			}
		}()
	}
	wg.Wait()
	l.Close()
	assertLineCount(t, filepath.Dir(l.CurrentPath()), goroutines*perGoroutine)
}

func assertLineCount(t *testing.T, dir string, want int) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			if !strings.Contains(line, "level=INFO msg=line g=") {
				t.Fatalf("torn or foreign line %q", line)
			}
			total++
		}
	}
	if total != want {
		t.Fatalf("lines = %d, want %d", total, want)
	}
}
