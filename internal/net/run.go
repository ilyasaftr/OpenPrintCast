package net

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	// resyncDelay debounces edits (a name field fires per keystroke) before
	// the advertisement is rebuilt.
	resyncDelay = 1500 * time.Millisecond
	// resyncInterval re-checks the network even when nothing was edited.
	resyncInterval = 5 * time.Second

	serverReadHeaderTimeout = 10 * time.Second
	serverReadTimeout       = 10 * time.Minute
	shutdownTimeout         = 5 * time.Second
)

// resyncLoop tracks both printer selection and network changes.
func resyncLoop(ctx context.Context, shared *SharedState, adv *Advertiser, delay time.Duration) {
	var fire <-chan time.Time
	ticker := time.NewTicker(resyncInterval)
	defer ticker.Stop()
	syncAdvertisement := func() {
		printers := shared.EnabledPrinters()
		changed, err := adv.Resync(printers)
		switch {
		case err != nil:
			componentLogger(componentAdvertise).Warn("advertise failed", "err", err)
		case changed:
			componentLogger(componentAdvertise).Info("re-advertised printers after a printer or network change", "printers", len(printers))
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-shared.Changed():
			fire = time.After(delay)
		case <-fire:
			fire = nil
			syncAdvertisement()
		case <-ticker.C:
			syncAdvertisement()
		case notice := <-adv.conflicts:
			if adv.markConflict(notice) {
				componentLogger(componentAdvertise).Info("resolving name conflict", "printer", notice.id)
				syncAdvertisement()
			}
		}
	}
}

// RunForever advertises the enabled printers, then serves IPP until ctx
// is cancelled. It blocks; shutdown withdraws the advertisement.
func RunForever(ctx context.Context, shared *SharedState, port int) error {
	printers := shared.EnabledPrinters()
	host := HostLabel()
	ip := OutboundIP()
	componentLogger(componentIPP).Info("starting ipp server",
		"port", port, "printers", len(printers), "host", host, "ip", ip)
	adv := NewAdvertiser(port)
	shared.setIdentityHost(host)
	shared.setInstanceName(adv.advertisedName)
	done := make(chan error, 1)
	if err := adv.RegisterAll(printers); err != nil {
		// mDNS may be unavailable (no multicast route in a container);
		// IPP serving is still useful, so warn and continue.
		componentLogger(componentAdvertise).Warn("advertise failed", "err", err)
	}
	stopResync := startResync(ctx, shared, adv)
	defer stopResync()
	srv := newPlainServer(shared, adv, port)
	go reportServe(done, func() error { return listenAndServe(shared, srv) })
	return awaitShutdown(ctx, done, srv)
}

// startResync runs resyncLoop in the background and returns a function that
// stops it, waits for it to exit, and withdraws the advertisement.
func startResync(ctx context.Context, shared *SharedState, adv *Advertiser) (stop func()) {
	loopCtx, stopLoop := context.WithCancel(ctx)
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		resyncLoop(loopCtx, shared, adv, resyncDelay)
	}()
	return func() {
		stopLoop()
		<-loopDone
		adv.Withdraw()
	}
}

// newPlainServer builds the IPP-over-HTTP server.
func newPlainServer(shared *SharedState, adv *Advertiser, port int) *http.Server {
	mux := newMux(shared, adv.Host, port)
	logged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// One line per request: distinguishes "iOS never arrived"
		// (firewall/mDNS) from "arrived but rejected" (IPP semantics).
		componentLogger(componentHTTP).Info("http request",
			"method", r.Method, "path", r.URL.Path, "remote", r.RemoteAddr, "ua", r.UserAgent())
		mux.ServeHTTP(w, r)
	})
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           logged,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
	}
}

// listenAndServe binds the server's address, records the outcome in shared
// for the reachability check, and serves until the server stops.
func listenAndServe(shared *SharedState, srv *http.Server) error {
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		shared.listen.setListenFailed(srv.Addr, err)
		return err
	}
	addr := ln.Addr().String()
	shared.listen.setListening(addr)
	err = srv.Serve(ln)
	shared.listen.setStopped(addr, ignoreServerClosed(err))
	return err
}

// ignoreServerClosed reports a clean shutdown as no error.
func ignoreServerClosed(err error) error {
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// reportServe runs serve and sends its error, with a clean shutdown
// reported as nil, to done.
func reportServe(done chan<- error, serve func() error) {
	done <- ignoreServerClosed(serve())
}

// awaitShutdown blocks until ctx is cancelled (graceful shutdown) or the
// server fails (immediate close), and returns the resulting error.
func awaitShutdown(ctx context.Context, done <-chan error, srv *http.Server) error {
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return srv.Shutdown(shutCtx)
	case err := <-done:
		_ = srv.Close()
		return err
	}
}
