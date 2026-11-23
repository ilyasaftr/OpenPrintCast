package main

import (
	"context"
	"time"

	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// serverRetryDelay is the pause before re-serving after the IPP server
// exits (for example a busy port 631 left by a previous instance),
// mirroring the Rust shell's 10 s backoff loop.
const serverRetryDelay = 10 * time.Second

// runServerForever serves IPP + mDNS until ctx is cancelled, retrying with
// backoff instead of exiting the goroutine when RunForever returns an
// error. It returns only after ctx is done.
func runServerForever(ctx context.Context, shared *bridgenet.SharedState, port int) {
	for {
		if err := bridgenet.RunForever(ctx, shared, port); err != nil {
			if ctx.Err() != nil {
				return
			}
			componentLogger(componentIPP).Error("ipp server exited; retrying", "err", err, "retry_in", serverRetryDelay)
		}
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(serverRetryDelay):
		}
	}
}
