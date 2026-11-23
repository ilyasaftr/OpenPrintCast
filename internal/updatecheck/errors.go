package updatecheck

import (
	"errors"
	"fmt"
	"time"
)

// Failures a check can end in. Callers tell them apart with errors.Is.
var (
	// ErrNotSetUp means no feed URL is configured.
	ErrNotSetUp = errors.New("update feed is not set up")
	// ErrInvalidFeedURL means the feed URL is not an absolute https URL.
	ErrInvalidFeedURL = errors.New("update feed URL must be an https URL")
	// ErrInsecureRedirect means the feed redirected to something that is
	// not https, or redirected too often.
	ErrInsecureRedirect = errors.New("update feed redirected to an unsafe place")
	// ErrFeedNotFound is an HTTP 404 from the feed.
	ErrFeedNotFound = errors.New("update feed not found")
	// ErrRateLimited means the feed host is limiting requests.
	ErrRateLimited = errors.New("update feed is rate limited")
	// ErrMalformedFeed means the answer was not a JSON array of releases.
	ErrMalformedFeed = errors.New("update feed is malformed")
	// ErrFeedTooLarge means the answer was longer than maxFeedBytes.
	ErrFeedTooLarge = errors.New("update feed is too large")
	// ErrNetwork wraps a transport failure, a timeout included.
	ErrNetwork = errors.New("update feed could not be reached")
	// ErrUnknownCurrentVersion means the running version is not a semantic
	// version (a development build), so there is nothing to compare with.
	ErrUnknownCurrentVersion = errors.New("current version is not a release version")
)

// StatusError is a non-2xx answer other than the ones with their own error.
type StatusError struct{ Code int }

// Error describes the status code.
func (e *StatusError) Error() string { return fmt.Sprintf("update feed answered HTTP %d", e.Code) }

// RateLimitError is ErrRateLimited with the wait the host asked for.
type RateLimitError struct {
	// RetryAfter is how long the host asked to wait; 0 when it did not say.
	RetryAfter time.Duration
}

// Error describes the limit.
func (e *RateLimitError) Error() string {
	if e.RetryAfter <= 0 {
		return ErrRateLimited.Error()
	}
	return fmt.Sprintf("%s; retry after %s", ErrRateLimited, e.RetryAfter)
}

// Is makes errors.Is(err, ErrRateLimited) true.
func (e *RateLimitError) Is(target error) bool { return target == ErrRateLimited }
