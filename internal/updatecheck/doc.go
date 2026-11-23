// Package updatecheck asks a release feed whether a newer version of the
// app exists. It only reads: nothing is downloaded or installed.
//
// The feed is a JSON array in the shape of GitHub's releases API
// (https://api.github.com/repos/OWNER/REPO/releases): each element has
// tag_name, prerelease, draft, html_url, published_at and body, and other
// fields are ignored. A static file with the same shape works as well, so
// the feed can be hosted anywhere that serves HTTPS.
//
// The package is portable: it has no UI, no OS and no app imports. Its
// network use is limited to one HTTPS GET per Fetch. The Go HTTP client
// honors the standard proxy environment variables (HTTPS_PROXY,
// NO_PROXY); the Windows system proxy settings are not read, so a PC that
// reaches the internet only through such a proxy reports a network error.
package updatecheck
