package net

import (
	"context"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestUploadLimitAndCanceledWaiter(t *testing.T) {
	shared := testShared()
	for slot := 1; slot <= maxConcurrentUploads; slot++ {
		if !shared.beginUpload(context.Background()) {
			t.Fatalf("could not reserve upload slot %d of %d", slot, maxConcurrentUploads)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if shared.beginUpload(ctx) {
		t.Fatal("canceled waiter acquired an upload slot")
	}
	for slot := 0; slot < maxConcurrentUploads; slot++ {
		shared.endUpload()
	}
}

func TestObservedPrinterChanges(t *testing.T) {
	shared := testShared()
	first := shared.observePrinter("hp", "A4", 3, "none")
	unchanged := shared.observePrinter("hp", "A4", 3, "none")
	if first != unchanged {
		t.Fatalf("unchanged observation changed timestamps: %+v vs %+v", first, unchanged)
	}
	config := shared.observePrinter("hp", "Letter", 3, "none")
	if config.configChange.Before(first.configChange) || config.stateChange != first.stateChange {
		t.Fatalf("incorrect config change observation: %+v", config)
	}
	stopped := shared.observePrinter("hp", "Letter", 5, "media-needed")
	if stopped.configChange != config.configChange || stopped.stateChange.Before(config.stateChange) {
		t.Fatalf("incorrect state change observation: %+v", stopped)
	}
}

func TestSetPrintersSignalsChangeAndCoalesces(t *testing.T) {
	s := NewSharedState(nil, nil)
	select {
	case <-s.Changed():
		t.Fatal("no signal expected before any change")
	default:
	}
	s.SetPrinters([]core.PrinterInfo{testPrinter()})
	s.SetPrinters([]core.PrinterInfo{testPrinter()})
	<-s.Changed()
	select {
	case <-s.Changed():
		t.Fatal("bursts must coalesce into one signal")
	default:
	}
}
