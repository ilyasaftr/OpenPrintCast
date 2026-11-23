package net

import "testing"

func TestCancelGateFirstClaimWins(t *testing.T) {
	var cancelFirst cancelGate
	if !cancelFirst.cancel() || cancelFirst.cancel() || !cancelFirst.finish() || !cancelFirst.finish() {
		t.Fatal("a cancel that won must stay won, once")
	}
	var finishFirst cancelGate
	if finishFirst.finish() || finishFirst.cancel() || finishFirst.finish() {
		t.Fatal("a finished job must not be cancellable")
	}
}
