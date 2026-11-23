package updatecheck

import (
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Release is one published version as the feed lists it.
type Release struct {
	// Tag is the version tag, such as "v0.2.9-beta.14".
	Tag string
	// Prerelease is the feed's own flag; a tag with a prerelease part
	// (-beta.1) counts as one too.
	Prerelease bool
	// Draft marks an unpublished release; Newest never offers one.
	Draft bool
	// URL is the release page to open, or "" when the feed gave none or an
	// address that is not safe to open (see safeReleaseURL).
	URL string
	// Published is when the release went out; zero when unknown.
	Published time.Time
	// Notes is the release description.
	Notes string
}

// IsPrerelease reports whether the release is a beta: flagged so by the
// feed or tagged so by its version.
func (r Release) IsPrerelease() bool {
	return r.Prerelease || semver.Prerelease(r.Tag) != ""
}

// validTag reports whether tag is a full semantic version with a "v"
// prefix (v1.2.3, v1.2.3-beta.4). The shorthands semver.IsValid also
// accepts (v1, v1.2) are not release tags.
func validTag(tag string) bool {
	return semver.IsValid(tag) && strings.TrimSuffix(tag, semver.Build(tag)) == semver.Canonical(tag)
}

// ValidateCurrent returns ErrUnknownCurrentVersion unless current is a
// release version, which is what a check compares against. "dev" and ""
// are not.
func ValidateCurrent(current string) error {
	if !validTag(current) {
		return ErrUnknownCurrentVersion
	}
	return nil
}

// Newer reports whether version a is higher than b, by semantic version
// precedence: v0.2.9-beta.9 < v0.2.9-beta.10 < v0.2.9. Both must be valid
// tags.
func Newer(a, b string) bool {
	return validTag(a) && validTag(b) && semver.Compare(a, b) > 0
}

// Newest returns the highest version in releases that is greater than
// current, ignoring drafts and tags that are not semantic versions, and
// betas unless includePrereleases. The highest version wins, whatever the
// order or the dates of the list. It reports false when nothing is newer,
// and for a current that is not a release version (see ValidateCurrent).
func Newest(releases []Release, current string, includePrereleases bool) (Release, bool) {
	var best Release
	found := false
	for _, r := range releases {
		if !offerable(r, includePrereleases) || !Newer(r.Tag, current) {
			continue
		}
		if !found || Newer(r.Tag, best.Tag) {
			best, found = r, true
		}
	}
	return best, found
}

// offerable reports whether r may be offered to the user at all.
func offerable(r Release, includePrereleases bool) bool {
	return !r.Draft && validTag(r.Tag) && (includePrereleases || !r.IsPrerelease())
}
