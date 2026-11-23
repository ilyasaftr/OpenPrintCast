package logfile

import "testing"

func TestFormatName(t *testing.T) {
	tests := []struct {
		n    logName
		want string
	}{
		{logName{"20240102", 1}, "app-20240102.log"},
		{logName{"20240102", 2}, "app-20240102-2.log"},
		{logName{"20240102", 12}, "app-20240102-12.log"},
	}
	for _, tt := range tests {
		if got := formatName("app", tt.n); got != tt.want {
			t.Errorf("formatName(%v) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestParseName(t *testing.T) {
	valid := map[string]logName{
		"app-20240102.log":    {"20240102", 1},
		"app-20240102-2.log":  {"20240102", 2},
		"app-20240102-10.log": {"20240102", 10},
	}
	for name, want := range valid {
		got, ok := parseName("app", name)
		if !ok || got != want {
			t.Errorf("parseName(%q) = %v, %v; want %v", name, got, ok, want)
		}
	}
	invalid := []string{
		"", "app.log", "app-20240102", "app-20240102.txt", "other-20240102.log",
		"app-2024010.log", "app-20241302.log", "app-20240102-1.log",
		"app-20240102-02.log", "app-20240102-+2.log", "app-20240102-x.log",
		"app-20240102-2-3.log", "app-20240102-0.log", "app-20240102.log.bak",
		"app-b-20240102.log", "xapp-20240102.log",
	}
	for _, name := range invalid {
		if got, ok := parseName("app", name); ok {
			t.Errorf("parseName(%q) = %v, want rejected", name, got)
		}
	}
}

func TestParseNameWithHyphenatedPrefix(t *testing.T) {
	n, ok := parseName("openprintcast", "openprintcast-20240102-3.log")
	if !ok || n != (logName{"20240102", 3}) {
		t.Fatalf("got %v, %v", n, ok)
	}
	if _, ok := parseName("airprint", "openprintcast-20240102.log"); ok {
		t.Fatal("prefix airprint must not match openprintcast files")
	}
}

func TestRoundTrip(t *testing.T) {
	for seq := 1; seq < 30; seq++ {
		n := logName{"20231231", seq}
		got, ok := parseName("p", formatName("p", n))
		if !ok || got != n {
			t.Fatalf("round trip of %v gave %v, %v", n, got, ok)
		}
	}
}

func TestCompareIsNumericInSequence(t *testing.T) {
	if (logName{"20240101", 9}).compare(logName{"20240101", 10}) >= 0 {
		t.Fatal("seq 9 must sort before 10")
	}
	if (logName{"20240101", 50}).compare(logName{"20240102", 1}) >= 0 {
		t.Fatal("date dominates sequence")
	}
}
