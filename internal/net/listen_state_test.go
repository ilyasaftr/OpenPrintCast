package net

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestDescribeListenError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"none", nil, ""},
		{"address in use (unix)", &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", syscall.EADDRINUSE)}, addrInUseText},
		{"address in use (winsock number)", &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", wsaEADDRINUSE)}, addrInUseText},
		{"address in use (Windows text)", errors.New("listen tcp :631: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted."), addrInUseText},
		{"permission (unix)", &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", syscall.EACCES)}, permissionText},
		{"permission (winsock number)", &net.OpError{Op: "listen", Err: os.NewSyscallError("bind", wsaEACCES)}, permissionText},
		{"permission (Windows text)", errors.New("bind: An attempt was made to access a socket in a way forbidden by its access permissions."), permissionText},
		{"anything else", errors.New("listen tcp: lookup failed\n"), otherFailedText + "listen tcp: lookup failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DescribeListenError(tc.err); got != tc.want {
				t.Fatalf("DescribeListenError = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDescribeListenErrorOfARealSecondListener(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback: %v", err)
	}
	defer first.Close()
	_, err = net.Listen("tcp", first.Addr().String())
	if err == nil {
		t.Skip("this system lets two listeners share a port")
	}
	if got := DescribeListenError(err); got != addrInUseText {
		t.Fatalf("DescribeListenError(%v) = %q, want %q", err, got, addrInUseText)
	}
}

func TestSharedStateRecordsListeningAndStopping(t *testing.T) {
	shared := NewSharedState(nil, nil)
	if shared.ListenStatus().Attempted() {
		t.Fatal("a fresh state has already tried")
	}
	var seen atomic.Int32
	shared.ObserveListen(func(ListenStatus) { seen.Add(1) })
	srv := &http.Server{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second}
	done := make(chan error, 1)
	go func() { done <- listenAndServe(shared, srv) }()
	waitFor(t, func() bool { return shared.ListenStatus().Listening })
	status := shared.ListenStatus()
	if status.Addr == "" || status.Err != "" || !status.Attempted() {
		t.Fatalf("status = %+v", status)
	}
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-done
	stopped := shared.ListenStatus()
	if stopped.Listening || stopped.Err != "" || stopped.Addr != status.Addr {
		t.Fatalf("stopped status = %+v", stopped)
	}
	if seen.Load() != 2 {
		t.Fatalf("observer saw %d changes, want 2", seen.Load())
	}
}

func TestSharedStateRecordsABindFailure(t *testing.T) {
	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("no loopback: %v", err)
	}
	defer taken.Close()
	shared := NewSharedState(nil, nil)
	srv := &http.Server{Addr: taken.Addr().String()}
	if err := listenAndServe(shared, srv); err == nil {
		t.Skip("this system lets two listeners share a port")
	}
	status := shared.ListenStatus()
	if status.Listening || status.Err != addrInUseText || status.Addr != srv.Addr || !status.Attempted() {
		t.Fatalf("status = %+v", status)
	}
	if got := status.String(); got != "not listening: "+addrInUseText {
		t.Fatalf("String = %q", got)
	}
}

func TestListenStatusRecoversAfterARetry(t *testing.T) {
	shared := NewSharedState(nil, nil)
	shared.listen.setListenFailed(":631", errors.New("boom"))
	shared.listen.setListening("[::]:631")
	if status := shared.ListenStatus(); !status.Listening || status.Err != "" {
		t.Fatalf("the old failure survived the retry: %+v", status)
	}
	if got := shared.ListenStatus().String(); got != fmt.Sprintf("listening on %s", "[::]:631") {
		t.Fatalf("String = %q", got)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
