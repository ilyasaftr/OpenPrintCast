package winspool

import (
	"context"

	"github.com/ilyasaftr/OpenPrintCast/internal/printerdev"
)

// probeUSB reads a USB-connected printer straight from the device through
// the vendor probes of printerdev.
//
// Probing writes the packet-mode switch into the printer's data stream,
// so it only runs while the queue is idle: mixed into the raster data of
// a job in flight, those bytes would corrupt the print. A job that starts
// after the check still races the probe — the window is the second a
// probe takes, the same one Epson's own Status Monitor lives with — but
// the idle check keeps the common case safe.
func probeUSB(ctx context.Context, printer string) (supplyReading, error) {
	if bits, ok := platformStatusBits(printer); ok && bits&activeBits != 0 {
		return supplyReading{}, errNoSupplySource
	}
	facts, err := printerdev.Probe(ctx, printer)
	if err != nil {
		return supplyReading{}, err
	}
	return supplyReading{
		Supplies:     facts.Supplies,
		Serial:       facts.Serial,
		Firmware:     facts.Firmware,
		StateReasons: facts.StateReasons,
	}, nil
}
