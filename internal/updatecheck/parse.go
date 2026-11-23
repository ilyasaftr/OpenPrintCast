package updatecheck

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// trustedLinkHost is the one host, besides the feed's own, whose release
// pages are offered to the user.
const trustedLinkHost = "github.com"

// ParseFeedURL checks a feed address: ErrNotSetUp when empty,
// ErrInvalidFeedURL unless it is an absolute https URL with a host and
// without credentials.
func ParseFeedURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, ErrNotSetUp
	}
	target, err := url.Parse(raw)
	if err != nil || target.Scheme != schemeHTTPS || target.Hostname() == "" || target.User != nil {
		return nil, ErrInvalidFeedURL
	}
	return target, nil
}

// wireRelease is one element of the feed; fields the app does not use are
// left out and ignored.
type wireRelease struct {
	TagName     string `json:"tag_name"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
}

// parseFeed decodes the feed's JSON array. feedHost is the host the feed
// was fetched from, which release pages may share.
func parseFeed(data []byte, feedHost string) ([]Release, error) {
	var wire []wireRelease
	if err := json.Unmarshal(data, &wire); err != nil || wire == nil {
		return nil, fmt.Errorf("%w: expected a JSON array of releases: %v", ErrMalformedFeed, err)
	}
	releases := make([]Release, len(wire))
	for i, w := range wire {
		releases[i] = Release{
			Tag:        w.TagName,
			Prerelease: w.Prerelease,
			Draft:      w.Draft,
			URL:        safeReleaseURL(w.HTMLURL, feedHost),
			Published:  parsePublished(w.PublishedAt),
			Notes:      w.Body,
		}
	}
	return releases, nil
}

// parsePublished reads an RFC 3339 time; anything else is the zero time,
// since a bad date must not hide a release.
func parsePublished(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// safeReleaseURL returns raw when it is safe to show and open, else "". The
// feed is not trusted to point anywhere: only an https address without
// credentials on the feed's own host or on github.com passes.
func safeReleaseURL(raw, feedHost string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != schemeHTTPS || u.User != nil || u.Hostname() == "" {
		return ""
	}
	host := strings.ToLower(u.Host)
	if host != strings.ToLower(feedHost) && host != trustedLinkHost {
		return ""
	}
	return u.String()
}
