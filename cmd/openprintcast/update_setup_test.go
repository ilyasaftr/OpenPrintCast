package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"

	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

func envOf(value string) func(string) string {
	return func(name string) string {
		if name == updateFeedEnv {
			return value
		}
		return ""
	}
}

func TestResolveUpdateSetup(t *testing.T) {
	const feed = "https://updates.example.com/releases.json"
	for _, tc := range []struct {
		name, version, built, env string
		want                      updateSetup
	}{
		{"release build with a feed", "v0.2.9-beta.14", feed, "", updateSetup{updateIdle, feed, "v0.2.9-beta.14"}},
		{"release build without a feed", "v0.2.9-beta.14", "", "", updateSetup{phase: updateNotSetUp}},
		{"release build with a plain http feed", "v0.2.9-beta.14", "http://updates.example.com/r.json", "", updateSetup{phase: updateNotSetUp}},
		{"release build with junk", "v0.2.9-beta.14", "::nope", "", updateSetup{phase: updateNotSetUp}},
		{"release build ignores the environment", "v0.2.9-beta.14", "", feed, updateSetup{phase: updateNotSetUp}},
		{"environment does not replace the built feed", "v0.2.9-beta.14", feed, "https://other.example/x", updateSetup{updateIdle, feed, "v0.2.9-beta.14"}},
		{"odd version", "1.2", feed, "", updateSetup{phase: updateDevBuild}},
		{"dev build", "dev", "", "", updateSetup{phase: updateDevBuild}},
		{"dev build ignores the built feed", "dev", feed, "", updateSetup{phase: updateDevBuild}},
		{"dev build with an override", "dev", "", feed, updateSetup{updateIdle, feed, devComparisonVersion}},
		{"dev build with a plain http override", "dev", "", "http://localhost/r.json", updateSetup{phase: updateNotSetUp}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			captureLogs(t)
			if got := resolveUpdateSetup(tc.version, tc.built, envOf(tc.env)); got != tc.want {
				t.Fatalf("setup = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestInvalidFeedIsLoggedAndEmptyIsNot(t *testing.T) {
	logs := captureLogs(t)
	resolveUpdateSetup("v1.0.0", "", envOf(""))
	if logs.String() != "" {
		t.Fatalf("an empty feed was logged: %s", logs)
	}
	resolveUpdateSetup("v1.0.0", "http://insecure.example/x", envOf(""))
	resolveUpdateSetup("dev", "", envOf("ftp://x"))
	out := logs.String()
	if strings.Count(out, "ignoring the update feed") != 2 || !strings.Contains(out, "source=build") || !strings.Contains(out, "source=environment") {
		t.Fatalf("log = %s", out)
	}
}

func TestDevComparisonVersionIsBelowEveryRelease(t *testing.T) {
	if !updatecheck.Newer("v0.0.1-beta.1", devComparisonVersion) || !updatecheck.Newer("v0.2.9-beta.14", devComparisonVersion) {
		t.Fatal("a dev build with an override must see every release as newer")
	}
}

func TestBuiltFeedDefaultsToNotSetUp(t *testing.T) {
	if updateFeedURL != "" {
		t.Fatalf("updateFeedURL = %q: a source build must not ship a feed", updateFeedURL)
	}
}

func TestLastCheckedText(t *testing.T) {
	f := newCLDRDates("en", i18n.Region{})
	now := updateT0
	for _, tc := range []struct {
		ago  time.Duration
		want string
	}{
		{0, "Last checked now"},
		{30 * time.Second, "Last checked now"},
		{-time.Hour, "Last checked now"},
		{time.Minute, "Last checked 1 minute ago"},
		{5*time.Minute + 59*time.Second, "Last checked 5 minutes ago"},
		{59 * time.Minute, "Last checked 59 minutes ago"},
		{time.Hour, "Last checked 1 hour ago"},
		{23*time.Hour + 59*time.Minute, "Last checked 23 hours ago"},
		{24 * time.Hour, "Last checked yesterday"},
		{3 * 24 * time.Hour, "Last checked 3 days ago"},
		{40 * 24 * time.Hour, "Last checked 40 days ago"},
	} {
		if got := lastCheckedText(now, now.Add(-tc.ago), f); got != tc.want {
			t.Errorf("%v ago: %q, want %q", tc.ago, got, tc.want)
		}
	}
	if got := lastCheckedText(now, time.Time{}, f); got != "" {
		t.Errorf("unknown time: %q", got)
	}
}
