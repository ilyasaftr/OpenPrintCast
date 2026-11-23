package net

import (
	"strings"
	"testing"
)

func TestSplitManufacturerRules(t *testing.T) {
	for _, tc := range []struct{ name, manufacturer, model string }{
		// Brand first: the brand words leave the model.
		{"EPSON L3210 Series", "EPSON", "L3210 Series"},
		{"Seiko Epson ET-2850", "EPSON", "ET-2850"},
		{"Hewlett-Packard LaserJet 1020", "HP", "LaserJet 1020"},
		{"Hewlett Packard DeskJet 2700", "HP", "DeskJet 2700"},
		{"Konica Minolta bizhub C250i", "KONICA MINOLTA", "bizhub C250i"},
		{"Fuji Xerox ApeosPort C3370", "Fuji Xerox", "ApeosPort C3370"},
		{"FUJIFILM Apeos C325", "FUJIFILM", "Apeos C325"},
		{"Lexmark International MS810", "Lexmark", "MS810"},
		{"Lexmark MS810", "Lexmark", "MS810"},
		{"Pantum M6500W series", "Pantum", "M6500W series"},
		{"Toshiba e-STUDIO2505AC", "TOSHIBA", "e-STUDIO2505AC"},
		{"Panasonic KX-MB2230", "Panasonic", "KX-MB2230"},
		{"Zebra ZD420", "Zebra", "ZD420"},
		{"DYMO LabelWriter 450", "DYMO", "LabelWriter 450"},
		{"SATO CL4NX", "SATO", "CL4NX"},
		{"TSC TE210", "TSC", "TE210"},
		{"Citizen CT-S310II", "Citizen", "CT-S310II"},
		{"Ricoh SP 330SFN", "RICOH", "SP 330SFN"},
		{"Lanier MP C2503", "Lanier", "MP C2503"},
		{"Samsung M2020 Series", "Samsung", "M2020 Series"},
		{"Sindoh D330", "SINDOH", "D330"},
		{"BIXOLON SRP-350III", "BIXOLON", "SRP-350III"},
		{"Honeywell PC42t", "Honeywell", "PC42t"},
		{"Kodak ESP 7250", "Kodak", "ESP 7250"},
		{"Dell 1130n", "Dell", "1130n"},
		{"OKI C332", "OKI", "C332"},
		{"Olivetti d-Color MF222", "Olivetti", "d-Color MF222"},
		{"Muratec MFX-C3290", "Muratec", "MFX-C3290"},
		// Product line first: the manufacturer is implied, the words stay.
		{"LaserJet Pro M404dn", "HP", "LaserJet Pro M404dn"},
		{"OfficeJet 8010", "HP", "OfficeJet 8010"},
		{"Smart Tank 7005", "HP", "Smart Tank 7005"},
		{"Stylus Photo R2880", "EPSON", "Stylus Photo R2880"},
		{"EcoTank ET-2720", "EPSON", "EcoTank ET-2720"},
		{"PIXMA TS3350", "Canon", "PIXMA TS3350"},
		{"imageRUNNER ADVANCE C5535", "Canon", "imageRUNNER ADVANCE C5535"},
		{"Phaser 3020", "Xerox", "Phaser 3020"},
		{"WorkCentre 6515", "Xerox", "WorkCentre 6515"},
		{"ECOSYS P2040dw", "KYOCERA", "ECOSYS P2040dw"},
		{"TASKalfa 3253ci", "KYOCERA", "TASKalfa 3253ci"},
		{"bizhub 227", "KONICA MINOLTA", "bizhub 227"},
		// Whole words only, and unknown names stay Generic.
		{"Hpglobal Office 5", "Generic", "Hpglobal Office 5"},
		{"Canonical Printer", "Generic", "Canonical Printer"},
		{"Okidoki Label", "Generic", "Okidoki Label"},
		{"Smart Printer 3", "Generic", "Smart Printer 3"},
		{"Acme Printer 5", "Generic", "Acme Printer 5"},
		// A bare brand has no model to split off.
		{"Brother", "Generic", "Brother"},
		{"Fuji Xerox", "Generic", "Fuji Xerox"},
		{"", "Generic", "Printer"},
	} {
		manufacturer, model := splitManufacturer(deviceIDValue(tc.name))
		if manufacturer != tc.manufacturer || model != tc.model {
			t.Errorf("splitManufacturer(%q) = (%q, %q), want (%q, %q)", tc.name, manufacturer, model, tc.manufacturer, tc.model)
		}
	}
}

func TestMatchManufacturerPrefersLongestPrefix(t *testing.T) {
	rule, words, ok := matchManufacturer("Seiko Epson ET-2850")
	if !ok || rule.manufacturer != "EPSON" || words != 2 {
		t.Fatalf("matchManufacturer = (%+v, %d, %v), want the two-word Seiko Epson rule", rule, words, ok)
	}
	if _, _, ok := matchManufacturer("Seiko"); ok {
		t.Error("a lone first word of a multi-word prefix must not match")
	}
}

func TestManufacturerRulesAreWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, rule := range manufacturerRules {
		switch {
		case rule.prefix == "" || rule.manufacturer == "":
			t.Errorf("rule %+v has an empty field", rule)
		case rule.prefix != strings.ToLower(rule.prefix):
			t.Errorf("prefix %q must be lower case", rule.prefix)
		case strings.ContainsAny(rule.prefix+rule.manufacturer, ";:"):
			t.Errorf("rule %+v contains an IEEE 1284 delimiter", rule)
		case strings.Join(strings.Fields(rule.prefix), " ") != rule.prefix:
			t.Errorf("prefix %q has irregular spacing", rule.prefix)
		case seen[rule.prefix]:
			t.Errorf("duplicate prefix %q", rule.prefix)
		}
		seen[rule.prefix] = true
	}
}
