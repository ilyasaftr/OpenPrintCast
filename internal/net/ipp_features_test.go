package net

import (
	"slices"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestIPPEverywhereIsClaimedOnlyWithSupplyData(t *testing.T) {
	attrs := printerAttributes(t, newSupplySpooler(cmykSupplies(), "192.168.1.50"))
	if got := attrValues(t, attrs, "ipp-features-supported"); !slices.Equal(got, []string{featureIPPEverywhere}) {
		t.Fatalf("ipp-features-supported = %v, want [%s]", got, featureIPPEverywhere)
	}
}

func TestIPPEverywhereIsNotClaimedWithoutSupplyData(t *testing.T) {
	for name, spooler := range map[string]Spooler{
		"backend without supplies":     &mediaSpooler{&fakeSpooler{}},
		"printer that reports nothing": newSupplySpooler(nil, "192.168.1.50"),
		"only unpublishable kinds":     newSupplySpooler([]core.Supply{{Name: "Transfer Kit", Kind: core.SupplyOther, Level: 50}}, "192.168.1.50"),
	} {
		t.Run(name, func(t *testing.T) {
			if hasAttr(printerAttributes(t, spooler), "ipp-features-supported") {
				t.Error("ipp-features-supported present although the printer reports no supplies")
			}
		})
	}
}
