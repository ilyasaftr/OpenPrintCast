package updatecheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const feedPath = "/releases"

// feedJSON is a feed in GitHub's shape: drafts, a beta, a stable release
// and links of every kind, in no particular order.
const feedJSON = `[
 {"tag_name":"v0.2.9-beta.10","prerelease":true,"draft":false,"html_url":"HOST/o/r/releases/tag/v0.2.9-beta.10","published_at":"2026-07-01T10:00:00Z","body":"ten","id":1},
 {"tag_name":"v0.2.9-beta.9","prerelease":true,"draft":false,"html_url":"https://github.com/o/r/releases/tag/v0.2.9-beta.9","published_at":"2026-06-30T10:00:00Z","body":"nine"},
 {"tag_name":"v0.3.0","prerelease":false,"draft":true,"html_url":"","published_at":null,"body":null},
 {"tag_name":"v0.2.8","prerelease":false,"draft":false,"html_url":"http://github.com/o/r/releases/tag/v0.2.8","published_at":"not a date","body":""},
 {"tag_name":"v0.2.7","html_url":"https://evil.example/o/r","published_at":"2026-01-01T00:00:00Z"},
 {"tag_name":"v0.2.6","html_url":"https://user:pw@github.com/o/r"},
 {"tag_name":"v0.2.5","html_url":"javascript:alert(1)"}
]`

// feedServer serves handler over TLS and returns a Feed that trusts it.
func feedServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Feed) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	return srv, NewFeed(Options{URL: srv.URL + feedPath, Version: "v0.2.9-beta.14", Client: srv.Client()})
}

func serveJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := "https://" + r.Host
		_, _ = w.Write([]byte(strings.ReplaceAll(body, "HOST", host)))
	}
}

func releaseByTag(t *testing.T, releases []Release, tag string) Release {
	t.Helper()
	for _, r := range releases {
		if r.Tag == tag {
			return r
		}
	}
	t.Fatalf("no release %q in %+v", tag, releases)
	return Release{}
}

func TestFetchParsesTheFeedInGitHubsShape(t *testing.T) {
	srv, feed := feedServer(t, serveJSON(feedJSON))
	releases, err := feed.Fetch(context.Background())
	if err != nil || len(releases) != 7 {
		t.Fatalf("Fetch = %d releases, %v", len(releases), err)
	}
	ten := releaseByTag(t, releases, "v0.2.9-beta.10")
	want := Release{Tag: "v0.2.9-beta.10", Prerelease: true, URL: srv.URL + "/o/r/releases/tag/v0.2.9-beta.10",
		Published: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC), Notes: "ten"}
	if ten != want {
		t.Fatalf("release = %+v, want %+v", ten, want)
	}
	if draft := releaseByTag(t, releases, "v0.3.0"); !draft.Draft || draft.URL != "" {
		t.Fatalf("draft = %+v", draft)
	}
	if bad := releaseByTag(t, releases, "v0.2.8"); !bad.Published.IsZero() {
		t.Fatalf("an unreadable date became %v", bad.Published)
	}
}

func TestFetchDropsReleaseLinksThatAreNotSafeToOpen(t *testing.T) {
	_, feed := feedServer(t, serveJSON(feedJSON))
	releases, err := feed.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for tag, wantLink := range map[string]bool{
		"v0.2.9-beta.10": true,  // the feed's own host
		"v0.2.9-beta.9":  true,  // github.com over https
		"v0.2.8":         false, // http
		"v0.2.7":         false, // another host
		"v0.2.6":         false, // credentials in the address
		"v0.2.5":         false, // not a web address
		"v0.3.0":         false, // no link at all
	} {
		r := releaseByTag(t, releases, tag)
		if (r.URL != "") != wantLink {
			t.Errorf("%s: URL = %q, want a link: %v", tag, r.URL, wantLink)
		}
	}
	if newest, ok := Newest(releases, "v0.2.8", true); !ok || newest.Tag != "v0.2.9-beta.10" {
		t.Fatalf("a release with a dropped link must still count: %+v, %v", newest, ok)
	}
	if r := releaseByTag(t, releases, "v0.2.7"); r.Tag == "" {
		t.Fatal("a release with an unsafe link was dropped altogether")
	}
}

func TestFetchSendsTheUserAgentAndAcceptHeaders(t *testing.T) {
	var agent, accept, method string
	_, feed := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		agent, accept, method = r.UserAgent(), r.Header.Get("Accept"), r.Method
		_, _ = w.Write([]byte("[]"))
	})
	if _, err := feed.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if agent != "OpenPrintCast/v0.2.9-beta.14 (update check)" || accept != "application/vnd.github+json" || method != http.MethodGet {
		t.Fatalf("agent=%q accept=%q method=%q", agent, accept, method)
	}
}

func TestFetchReusesTheAnswerOnNotModified(t *testing.T) {
	var requests atomic.Int32
	var conditional atomic.Value
	_, feed := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		conditional.Store(r.Header.Get("If-None-Match"))
		if r.Header.Get("If-None-Match") == `"abc"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"abc"`)
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0"}]`))
	})
	first, err := feed.Fetch(context.Background())
	if err != nil || len(first) != 1 || conditional.Load() != "" {
		t.Fatalf("first fetch = %+v, %v, If-None-Match %q", first, err, conditional.Load())
	}
	first[0].Tag = "tampered" // the caller's copy must not leak into the cache
	second, err := feed.Fetch(context.Background())
	if err != nil || len(second) != 1 || second[0].Tag != "v1.0.0" {
		t.Fatalf("second fetch = %+v, %v", second, err)
	}
	if conditional.Load() != `"abc"` || requests.Load() != 2 {
		t.Fatalf("If-None-Match = %q after %d requests", conditional.Load(), requests.Load())
	}
}

func TestNotModifiedWithoutAnAnswerToReuseIsAnError(t *testing.T) {
	_, feed := feedServer(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotModified) })
	var status *StatusError
	if _, err := feed.Fetch(context.Background()); !errors.As(err, &status) || status.Code != http.StatusNotModified {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchTypedHTTPFailures(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name       string
		code       int
		headers    map[string]string
		is         error
		retryAfter time.Duration
	}{
		{name: "not found", code: 404, is: ErrFeedNotFound},
		{name: "429", code: 429, is: ErrRateLimited},
		{name: "429 with a wait", code: 429, headers: map[string]string{"Retry-After": "120"}, is: ErrRateLimited, retryAfter: 2 * time.Minute},
		{name: "403 quota used up", code: 403, headers: map[string]string{"X-RateLimit-Remaining": "0"}, is: ErrRateLimited},
		{name: "403 with the reset time", code: 403, is: ErrRateLimited, retryAfter: 90 * time.Second, headers: map[string]string{
			"X-RateLimit-Remaining": "0", "X-RateLimit-Reset": strconv.FormatInt(now.Add(90*time.Second).Unix(), 10)}},
		{name: "403 with a date", code: 403, is: ErrRateLimited, retryAfter: time.Hour,
			headers: map[string]string{"Retry-After": now.Add(time.Hour).Format(http.TimeFormat)}},
		{name: "403 for another reason", code: 403, headers: map[string]string{"X-RateLimit-Remaining": "42"}},
		{name: "server error", code: 500},
		{name: "redirect without a place", code: 300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.code)
			}))
			defer srv.Close()
			feed := NewFeed(Options{URL: srv.URL, Client: srv.Client(), Now: func() time.Time { return now }})
			_, err := feed.Fetch(context.Background())
			var status *StatusError
			var limit *RateLimitError
			switch {
			case tc.is != nil:
				if !errors.Is(err, tc.is) {
					t.Fatalf("err = %v, want %v", err, tc.is)
				}
			case !errors.As(err, &status) || status.Code != tc.code:
				t.Fatalf("err = %v, want a StatusError for %d", err, tc.code)
			}
			if errors.As(err, &limit) && limit.RetryAfter != tc.retryAfter {
				t.Fatalf("RetryAfter = %v, want %v", limit.RetryAfter, tc.retryAfter)
			}
		})
	}
}

func TestFetchMalformedAnswers(t *testing.T) {
	for name, body := range map[string]string{
		"not json":       "<html>nope</html>",
		"an object":      `{"tag_name":"v1.0.0"}`,
		"null":           "null",
		"empty":          "",
		"cut off":        `[{"tag_name":"v1.0`,
		"wrong type":     `[{"tag_name":42}]`,
		"array of lists": `[[1,2]]`,
	} {
		t.Run(name, func(t *testing.T) {
			_, feed := feedServer(t, serveJSON(body))
			if _, err := feed.Fetch(context.Background()); !errors.Is(err, ErrMalformedFeed) {
				t.Fatalf("err = %v, want ErrMalformedFeed", err)
			}
		})
	}
}

func TestFetchRefusesAnOversizedBody(t *testing.T) {
	huge := `[{"tag_name":"v1.0.0","body":"` + strings.Repeat("a", maxFeedBytes) + `"}]`
	_, feed := feedServer(t, serveJSON(huge))
	if _, err := feed.Fetch(context.Background()); !errors.Is(err, ErrFeedTooLarge) {
		t.Fatalf("err = %v, want ErrFeedTooLarge", err)
	}
}

func TestFetchAcceptsABodyJustUnderTheLimit(t *testing.T) {
	filler := strings.Repeat("a", maxFeedBytes-200)
	_, feed := feedServer(t, serveJSON(`[{"tag_name":"v1.0.0","body":"`+filler+`"}]`))
	if releases, err := feed.Fetch(context.Background()); err != nil || len(releases) != 1 {
		t.Fatalf("Fetch = %d releases, %v", len(releases), err)
	}
}

func TestFetchRefusesRedirectsToPlainHTTP(t *testing.T) {
	var reached atomic.Bool
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
		_, _ = w.Write([]byte("[]"))
	}))
	defer plain.Close()
	_, feed := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/x", http.StatusFound)
	})
	if _, err := feed.Fetch(context.Background()); !errors.Is(err, ErrInsecureRedirect) {
		t.Fatalf("err = %v, want ErrInsecureRedirect", err)
	}
	if reached.Load() {
		t.Fatal("the plain HTTP server was contacted")
	}
}

func TestFetchFollowsHTTPSRedirectsButNotForever(t *testing.T) {
	_, feed := feedServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == feedPath {
			http.Redirect(w, r, "/moved", http.StatusMovedPermanently)
			return
		}
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0"}]`))
	})
	if releases, err := feed.Fetch(context.Background()); err != nil || len(releases) != 1 {
		t.Fatalf("Fetch = %v, %v", releases, err)
	}
	_, loop := feedServer(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, feedPath, http.StatusFound) })
	if _, err := loop.Fetch(context.Background()); !errors.Is(err, ErrInsecureRedirect) {
		t.Fatalf("a redirect loop ended with %v", err)
	}
}

func TestFetchRefusesFeedURLsThatAreNotHTTPS(t *testing.T) {
	var requests atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer plain.Close()
	for _, raw := range []string{plain.URL + feedPath, "ftp://example.com/x", "file:///etc/passwd", "https://", "//example.com/x", "not a url", "%zz"} {
		_, err := NewFeed(Options{URL: raw}).Fetch(context.Background())
		if !errors.Is(err, ErrInvalidFeedURL) {
			t.Errorf("%q: err = %v, want ErrInvalidFeedURL", raw, err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("a request was sent to a URL that is not https")
	}
}

func TestFetchWithoutAFeedURLIsNotSetUp(t *testing.T) {
	if _, err := NewFeed(Options{}).Fetch(context.Background()); !errors.Is(err, ErrNotSetUp) {
		t.Fatalf("err = %v, want ErrNotSetUp", err)
	}
}

func TestFetchTimeoutAndUnreachableHostAreNetworkErrors(t *testing.T) {
	srv, feed := feedServer(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	feed.timeout = 30 * time.Millisecond
	if _, err := feed.Fetch(context.Background()); !errors.Is(err, ErrNetwork) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: err = %v", err)
	}
	srv.Close()
	if _, err := feed.Fetch(context.Background()); !errors.Is(err, ErrNetwork) {
		t.Fatalf("closed server: err = %v", err)
	}
}

func TestFetchStopsWhenTheCallerCancels(t *testing.T) {
	_, feed := feedServer(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	if _, err := feed.Fetch(ctx); !errors.Is(err, context.Canceled) || errors.Is(err, ErrNetwork) {
		t.Fatalf("err = %v, want the caller's cancellation", err)
	}
}

func TestNewFeedLeavesTheCallersClientAlone(t *testing.T) {
	client := &http.Client{Timeout: time.Minute}
	NewFeed(Options{URL: "https://example.com", Client: client})
	if client.CheckRedirect != nil || client.Timeout != time.Minute {
		t.Fatalf("the injected client was changed: %+v", client)
	}
}

func TestParseFeedURL(t *testing.T) {
	if _, err := ParseFeedURL("https://example.com/releases.json"); err != nil {
		t.Fatalf("a good URL was refused: %v", err)
	}
	if _, err := ParseFeedURL(""); !errors.Is(err, ErrNotSetUp) {
		t.Fatalf("empty: %v", err)
	}
	for _, raw := range []string{"http://example.com/x", "https://u:p@example.com/x", "https:///x", "example.com/x"} {
		if _, err := ParseFeedURL(raw); !errors.Is(err, ErrInvalidFeedURL) {
			t.Errorf("%q: %v", raw, err)
		}
	}
}
