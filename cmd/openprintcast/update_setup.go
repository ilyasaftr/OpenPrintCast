package main

import (
	"errors"

	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

// updateFeedURL is the release feed this build checks. It is empty unless
// the build stamps it in, with -ldflags "-X main.updateFeedURL=<https URL>"
// (release.yml does, from the repository variable UPDATE_FEED_URL). Empty
// means update checking is not set up. The feed is the JSON array of
// GitHub's releases API, see the updatecheck package.
var updateFeedURL = ""

// Dev builds can try the feature without a release: OPENPRINTCAST_UPDATE_FEED
// names a feed, which is then compared against devComparisonVersion, lower
// than every release. Other builds ignore the variable.
const (
	updateFeedEnv        = "OPENPRINTCAST_UPDATE_FEED"
	devVersion           = "dev"
	devComparisonVersion = "v0.0.0-dev"
)

// updateSetup is how a build checks for updates: the phase it starts in
// (updateIdle when it can check), the feed and the version to compare.
type updateSetup struct {
	phase   updatePhase
	feedURL string
	current string
}

// resolveUpdateSetup decides how this build checks for updates from its
// version, the feed stamped in at build time and the environment. An
// invalid feed counts as none, and says so in the log.
func resolveUpdateSetup(version, builtFeed string, getenv func(string) string) updateSetup {
	if version == devVersion {
		return devUpdateSetup(getenv(updateFeedEnv))
	}
	if updatecheck.ValidateCurrent(version) != nil {
		return updateSetup{phase: updateDevBuild}
	}
	feed, ok := checkedFeedURL(builtFeed, "build")
	if !ok {
		return updateSetup{phase: updateNotSetUp}
	}
	return updateSetup{phase: updateIdle, feedURL: feed, current: version}
}

// devUpdateSetup is the setup of a dev build: none, unless the environment
// names a valid feed to try.
func devUpdateSetup(override string) updateSetup {
	if override == "" {
		return updateSetup{phase: updateDevBuild}
	}
	feed, ok := checkedFeedURL(override, "environment")
	if !ok {
		return updateSetup{phase: updateNotSetUp}
	}
	componentLogger(componentUpdate).Info("update feed taken from the environment", "variable", updateFeedEnv)
	return updateSetup{phase: updateIdle, feedURL: feed, current: devComparisonVersion}
}

// checkedFeedURL returns raw when it is a usable feed URL. An empty one is
// quietly "not set up"; an invalid one is logged, with where it came from.
func checkedFeedURL(raw, source string) (string, bool) {
	_, err := updatecheck.ParseFeedURL(raw)
	switch {
	case err == nil:
		return raw, true
	case !errors.Is(err, updatecheck.ErrNotSetUp):
		componentLogger(componentUpdate).Warn("ignoring the update feed: it must be an https URL", "source", source)
	}
	return "", false
}
