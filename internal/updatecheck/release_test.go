package updatecheck

import (
	"errors"
	"testing"
)

// fixtureReleases is an unordered list with drafts, betas, a stable
// release and tags that are not versions.
func fixtureReleases() []Release {
	return []Release{
		{Tag: "v0.2.9-beta.10", Prerelease: true},
		{Tag: "v0.2.9-beta.9", Prerelease: true},
		{Tag: "v9.9.9", Draft: true},
		{Tag: "nightly"},
		{Tag: "v1"},
		{Tag: "0.3.0"},
		{Tag: ""},
		{Tag: "v0.2.9-beta.14", Prerelease: true},
		{Tag: "v0.2.9-beta.2", Prerelease: true},
		{Tag: "v0.2.8"},
	}
}

func TestNewestPicksTheHighestVersionNotTheNewestByList(t *testing.T) {
	for _, tc := range []struct {
		name, current string
		prereleases   bool
		want          string
	}{
		// The draft v9.9.9 and the invalid tags are higher or odd but never win.
		{"beta.9 to beta.10 and beyond", "v0.2.9-beta.9", true, "v0.2.9-beta.14"},
		{"betas ignored when not included", "v0.2.7", false, "v0.2.8"},
		{"betas win when included", "v0.2.7", true, "v0.2.9-beta.14"},
		{"beta.2 is below beta.10 numerically", "v0.2.9-beta.2", true, "v0.2.9-beta.14"},
		{"older stable than current beta is no update", "v0.2.9-beta.14", false, ""},
		{"current newer than everything", "v1.0.0", true, ""},
		{"equal version is no update", "v0.2.9-beta.14", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Newest(fixtureReleases(), tc.current, tc.prereleases)
			if ok != (tc.want != "") || got.Tag != tc.want {
				t.Fatalf("Newest(%q, beta=%v) = %q, %v; want %q", tc.current, tc.prereleases, got.Tag, ok, tc.want)
			}
		})
	}
}

func TestStableReleaseOutranksItsBetas(t *testing.T) {
	releases := []Release{{Tag: "v0.2.9", Prerelease: false}, {Tag: "v0.2.9-beta.20", Prerelease: true}}
	got, ok := Newest(releases, "v0.2.9-beta.14", true)
	if !ok || got.Tag != "v0.2.9" {
		t.Fatalf("Newest = %q, %v; want v0.2.9", got.Tag, ok)
	}
}

func TestPrereleaseTagCountsEvenWhenTheFeedForgotTheFlag(t *testing.T) {
	releases := []Release{{Tag: "v0.2.9-beta.15", Prerelease: false}}
	if got, ok := Newest(releases, "v0.2.9-beta.14", false); ok {
		t.Fatalf("a beta tag was offered to someone who excluded betas: %+v", got)
	}
	if _, ok := Newest(releases, "v0.2.9-beta.14", true); !ok {
		t.Fatal("the beta was not offered to someone who included betas")
	}
}

func TestNewestWithoutAReleaseVersionOffersNothing(t *testing.T) {
	for _, current := range []string{"dev", "", "v1", "0.2.9", "garbage"} {
		if got, ok := Newest(fixtureReleases(), current, true); ok {
			t.Errorf("current %q: offered %+v", current, got)
		}
	}
}

func TestValidateCurrent(t *testing.T) {
	for current, valid := range map[string]bool{
		"v0.2.9-beta.14": true, "v1.2.3": true, "v1.2.3+meta": true,
		"dev": false, "": false, "v1": false, "v1.2": false, "1.2.3": false,
	} {
		err := ValidateCurrent(current)
		if valid && err != nil || !valid && !errors.Is(err, ErrUnknownCurrentVersion) {
			t.Errorf("ValidateCurrent(%q) = %v, want valid=%v", current, err, valid)
		}
	}
}

func TestNewerOrdersPrereleases(t *testing.T) {
	ordered := []string{"v0.2.9-beta.2", "v0.2.9-beta.9", "v0.2.9-beta.10", "v0.2.9", "v0.2.10"}
	for i := range ordered {
		for j := range ordered {
			if got := Newer(ordered[i], ordered[j]); got != (i > j) {
				t.Errorf("Newer(%s, %s) = %v", ordered[i], ordered[j], got)
			}
		}
	}
	if Newer("junk", "v1.0.0") || Newer("v1.0.0", "junk") {
		t.Error("an invalid tag compared as newer")
	}
}
