package main

import (
	"time"

	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// ipRefreshInterval bounds how often the LAN address is re-read.
const ipRefreshInterval = 15 * time.Second

// lanIP returns the address devices reach this PC on ("" when offline),
// re-read at most every ipRefreshInterval so a network change shows up
// without opening a socket on every frame.
func (a *bridgeApp) lanIP() string {
	if a.ipAt.IsZero() || time.Since(a.ipAt) > ipRefreshInterval {
		a.ip = bridgenet.OutboundIP()
		a.ipAt = time.Now()
	}
	return a.ip
}

// status is the current one-line system status (header pill, tray tip).
func (a *bridgeApp) status() (string, level) {
	text, lvl := summarize(a.paused, a.sharedPrinterCount(), a.destinationCount(), a.lanIP() != "")
	if lvl == levelOK && a.netProblem() {
		return text + netAttentionSuffix, levelWarn
	}
	return text, lvl
}
