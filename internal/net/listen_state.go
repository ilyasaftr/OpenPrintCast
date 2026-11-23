package net

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ListenStatus says whether the IPP server is accepting connections right
// now. Addr is the bound address while listening; Err is the plain-language
// reason of the last failed attempt, kept until the next success. At is
// when the status last changed; it is zero until the server first tried.
type ListenStatus struct {
	Listening bool
	Addr      string
	Err       string
	At        time.Time
}

// Attempted reports whether the server has tried to listen at all.
func (s ListenStatus) Attempted() bool { return !s.At.IsZero() }

// ListenStatusReader is the read side of the server's listening state, for
// code that only reports on it (the reachability check).
type ListenStatusReader interface {
	ListenStatus() ListenStatus
}

// listenState is the goroutine-safe holder of ListenStatus; the server
// goroutine writes it and the UI reads it.
type listenState struct {
	mu       sync.Mutex
	status   ListenStatus
	observer func(ListenStatus)
}

// current returns the latest status.
func (l *listenState) current() ListenStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status
}

// observe registers fn to be called after every change of the status. It
// replaces an earlier observer.
func (l *listenState) observe(fn func(ListenStatus)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.observer = fn
}

func (l *listenState) set(status ListenStatus) {
	l.mu.Lock()
	status.At = time.Now()
	l.status = status
	observer := l.observer
	l.mu.Unlock()
	if observer != nil {
		observer(status)
	}
}

// setListening records that the server is bound to addr.
func (l *listenState) setListening(addr string) { l.set(ListenStatus{Listening: true, Addr: addr}) }

// setListenFailed records that the server could not listen on addr.
func (l *listenState) setListenFailed(addr string, err error) {
	l.set(ListenStatus{Addr: addr, Err: DescribeListenError(err)})
}

// setStopped records that a server that was listening has stopped, keeping
// the reason when it stopped on an error.
func (l *listenState) setStopped(addr string, err error) {
	status := ListenStatus{Addr: addr}
	if err != nil {
		status.Err = DescribeListenError(err)
	}
	l.set(status)
}

// Windows socket error numbers that syscall does not map to its own
// Errno constants.
const (
	wsaEACCES     syscall.Errno = 10013
	wsaEADDRINUSE syscall.Errno = 10048
)

// Wording of a failed listen.
const (
	addrInUseText   = "the port is already in use by another program"
	permissionText  = "Windows did not allow the program to listen on the port"
	otherFailedText = "listening failed: "
)

// DescribeListenError turns a listen failure into one short phrase that
// tells address in use, permission and everything else apart.
func DescribeListenError(err error) string {
	switch {
	case err == nil:
		return ""
	case isAddrInUse(err):
		return addrInUseText
	case isPermissionDenied(err):
		return permissionText
	}
	return otherFailedText + strings.TrimSpace(err.Error())
}

func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, wsaEADDRINUSE) ||
		containsFold(err, "address already in use", "only one usage of each socket address")
}

func isPermissionDenied(err error) bool {
	return errors.Is(err, syscall.EACCES) || errors.Is(err, wsaEACCES) ||
		containsFold(err, "permission denied", "forbidden by its access permissions")
}

func containsFold(err error, phrases ...string) bool {
	text := strings.ToLower(err.Error())
	for _, p := range phrases {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

// String is the status in one line for logs and diagnostics.
func (s ListenStatus) String() string {
	if s.Listening {
		return fmt.Sprintf("listening on %s", s.Addr)
	}
	if s.Err != "" {
		return "not listening: " + s.Err
	}
	return "not listening"
}
