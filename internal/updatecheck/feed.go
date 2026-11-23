package updatecheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"
)

// Network rules of a fetch.
const (
	// fetchTimeout bounds the whole exchange, body included.
	fetchTimeout = 10 * time.Second
	// maxFeedBytes is the largest answer read; GitHub's first page of
	// releases is far below it.
	maxFeedBytes = 2 << 20
	// maxRedirects matches net/http's own limit.
	maxRedirects = 10
	// schemeHTTPS is the only scheme fetched, followed or opened.
	schemeHTTPS = "https"
	// feedAccept asks for GitHub's JSON media type; static hosts ignore it.
	feedAccept = "application/vnd.github+json"
	// userAgentFormat is filled with the app version.
	userAgentFormat = "OpenPrintCast/%s (update check)"
)

// Headers the feed exchange uses; rateLimitExhausted is the value of
// headerRateLimitRemain when the quota is used up.
const (
	headerRetryAfter      = "Retry-After"
	headerRateLimitRemain = "X-RateLimit-Remaining"
	headerRateLimitReset  = "X-RateLimit-Reset"
	rateLimitExhausted    = "0"
	headerIfNoneMatch     = "If-None-Match"
	headerETag            = "ETag"
)

// Options configure a Feed.
type Options struct {
	// URL is the feed address; empty means not set up (ErrNotSetUp).
	URL string
	// Version is the running app version, sent in the User-Agent.
	Version string
	// Client does the requests; nil uses a default client. It is copied,
	// never changed, before its redirect policy is tightened.
	Client *http.Client
	// Now is the clock for Retry-After dates; nil means time.Now.
	Now func() time.Time
}

// Feed fetches the release list of one feed URL. It remembers the last
// answer and its ETag in memory, so an unchanged feed costs one 304. It is
// safe for concurrent use.
type Feed struct {
	url     string
	agent   string
	client  *http.Client
	now     func() time.Time
	timeout time.Duration
	mu      sync.Mutex
	etag    string
	release []Release
}

// NewFeed builds a Feed from o.
func NewFeed(o Options) *Feed {
	f := &Feed{url: o.URL, agent: fmt.Sprintf(userAgentFormat, o.Version), now: o.Now, timeout: fetchTimeout}
	if f.now == nil {
		f.now = time.Now
	}
	client := http.Client{}
	if o.Client != nil {
		client = *o.Client
	}
	client.CheckRedirect = httpsOnlyRedirect
	f.client = &client
	return f
}

// httpsOnlyRedirect refuses a redirect to anything but https, and a chain
// longer than maxRedirects.
func httpsOnlyRedirect(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != schemeHTTPS || len(via) >= maxRedirects {
		return ErrInsecureRedirect
	}
	return nil
}

// Fetch downloads the feed and returns its releases in the feed's order,
// drafts included (Newest skips them). Release pages whose address is not
// safe to open come back with an empty URL. ctx cancels the request; the
// fetch also gives up after fetchTimeout.
func (f *Feed) Fetch(ctx context.Context) ([]Release, error) {
	target, err := ParseFeedURL(f.url)
	if err != nil {
		return nil, err
	}
	caller := ctx
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	resp, err := f.get(ctx, target)
	if err != nil {
		return nil, transportError(err)
	}
	defer resp.Body.Close()
	releases, err := f.answer(resp, target)
	if err != nil && errors.Is(caller.Err(), context.Canceled) {
		return nil, caller.Err()
	}
	return releases, err
}

// get sends the request, with the ETag of the answer it remembers.
func (f *Feed) get(ctx context.Context, target *url.URL) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.agent)
	req.Header.Set("Accept", feedAccept)
	if etag := f.knownETag(); etag != "" {
		req.Header.Set(headerIfNoneMatch, etag)
	}
	return f.client.Do(req)
}

// transportError wraps a failure of the exchange as ErrNetwork, leaving
// the caller's own cancellation and a refused redirect as they are.
func transportError(err error) error {
	if errors.Is(err, ErrInsecureRedirect) || errors.Is(err, context.Canceled) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrNetwork, err)
}

// answer turns an HTTP answer into releases or a typed error.
func (f *Feed) answer(resp *http.Response, target *url.URL) ([]Release, error) {
	switch {
	case resp.StatusCode == http.StatusNotModified:
		return f.remembered(resp.StatusCode)
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrFeedNotFound
	case f.rateLimited(resp):
		return nil, &RateLimitError{RetryAfter: f.retryAfter(resp.Header)}
	case resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices:
		return nil, &StatusError{Code: resp.StatusCode}
	}
	body, err := readLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	releases, err := parseFeed(body, target.Host)
	if err != nil {
		return nil, err
	}
	f.remember(resp.Header.Get(headerETag), releases)
	return slices.Clone(releases), nil
}

// readLimited reads the body up to maxFeedBytes.
func readLimited(body io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxFeedBytes+1))
	if err != nil {
		return nil, transportError(err)
	}
	if len(data) > maxFeedBytes {
		return nil, ErrFeedTooLarge
	}
	return data, nil
}

// rateLimited reports whether the answer is the host limiting requests: a
// 429, or a 403 that says the quota is used up or says when to come back.
func (f *Feed) rateLimited(resp *http.Response) bool {
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return true
	case http.StatusForbidden:
		h := resp.Header
		return h.Get(headerRateLimitRemain) == rateLimitExhausted || h.Get(headerRetryAfter) != ""
	}
	return false
}

// retryAfter is the wait a rate-limited answer asks for: Retry-After in
// seconds or as a date, else the time until X-RateLimit-Reset; 0 if none.
func (f *Feed) retryAfter(h http.Header) time.Duration {
	if v := h.Get(headerRetryAfter); v != "" {
		if seconds, err := strconv.Atoi(v); err == nil {
			return max(time.Duration(seconds)*time.Second, 0)
		}
		if at, err := http.ParseTime(v); err == nil {
			return max(at.Sub(f.now()), 0)
		}
	}
	if seconds, err := strconv.ParseInt(h.Get(headerRateLimitReset), 10, 64); err == nil {
		return max(time.Unix(seconds, 0).Sub(f.now()), 0)
	}
	return 0
}

// knownETag is the ETag to send, only while there is an answer to reuse.
func (f *Feed) knownETag() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.release == nil {
		return ""
	}
	return f.etag
}

// remember keeps an answer and its ETag for the next conditional request.
func (f *Feed) remember(etag string, releases []Release) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.etag, f.release = etag, releases
}

// remembered is the answer a 304 stands for; a 304 without one is odd
// enough to be an error.
func (f *Feed) remembered(status int) ([]Release, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.release == nil {
		return nil, &StatusError{Code: status}
	}
	return slices.Clone(f.release), nil
}
