package net

import "strings"

// manufacturerRule recognizes a manufacturer from the leading words of a
// Windows printer name. prefix is lower case and may hold several words.
// keepPrefix marks a product line (for example "LaserJet"): the manufacturer
// is implied, but the matched words stay part of the model.
type manufacturerRule struct {
	prefix       string
	manufacturer string
	keepPrefix   bool
}

// manufacturerRules lists the manufacturers the bridge recognizes. Only
// distinctive brand names and trademarked product lines belong here: there is
// no canonical list of manufacturer names, and a wrong guess is worse than
// "Generic". The spellings follow what the devices report in IEEE 1284 IDs.
var manufacturerRules = []manufacturerRule{
	// Office and home printer brands.
	{prefix: "hp", manufacturer: "HP"},
	{prefix: "hewlett-packard", manufacturer: "HP"},
	{prefix: "hewlett packard", manufacturer: "HP"},
	{prefix: "brother", manufacturer: "Brother"},
	{prefix: "canon", manufacturer: "Canon"},
	{prefix: "dell", manufacturer: "Dell"},
	{prefix: "epson", manufacturer: "EPSON"},
	{prefix: "seiko epson", manufacturer: "EPSON"},
	{prefix: "fujifilm", manufacturer: "FUJIFILM"},
	{prefix: "fuji xerox", manufacturer: "Fuji Xerox"},
	{prefix: "fujitsu", manufacturer: "Fujitsu"},
	{prefix: "konica minolta", manufacturer: "KONICA MINOLTA"},
	{prefix: "kodak", manufacturer: "Kodak"},
	{prefix: "kyocera", manufacturer: "KYOCERA"},
	{prefix: "lenovo", manufacturer: "Lenovo"},
	{prefix: "lexmark international", manufacturer: "Lexmark"},
	{prefix: "lexmark", manufacturer: "Lexmark"},
	{prefix: "muratec", manufacturer: "Muratec"},
	{prefix: "oki", manufacturer: "OKI"},
	{prefix: "olivetti", manufacturer: "Olivetti"},
	{prefix: "panasonic", manufacturer: "Panasonic"},
	{prefix: "pantum", manufacturer: "Pantum"},
	{prefix: "ricoh", manufacturer: "RICOH"},
	{prefix: "lanier", manufacturer: "Lanier"},
	{prefix: "savin", manufacturer: "Savin"},
	{prefix: "gestetner", manufacturer: "Gestetner"},
	{prefix: "riso", manufacturer: "RISO"},
	{prefix: "samsung", manufacturer: "Samsung"},
	{prefix: "sharp", manufacturer: "SHARP"},
	{prefix: "sindoh", manufacturer: "SINDOH"},
	{prefix: "toshiba", manufacturer: "TOSHIBA"},
	{prefix: "triumph-adler", manufacturer: "Triumph-Adler"},
	{prefix: "utax", manufacturer: "UTAX"},
	{prefix: "xerox", manufacturer: "Xerox"},
	{prefix: "microsoft", manufacturer: "Microsoft"},

	// Label, receipt and specialty printer brands.
	{prefix: "bixolon", manufacturer: "BIXOLON"},
	{prefix: "brady", manufacturer: "Brady"},
	{prefix: "citizen", manufacturer: "Citizen"},
	{prefix: "datamax", manufacturer: "Datamax-O'Neil"},
	{prefix: "dymo", manufacturer: "DYMO"},
	{prefix: "godex", manufacturer: "Godex"},
	{prefix: "honeywell", manufacturer: "Honeywell"},
	{prefix: "intermec", manufacturer: "Intermec"},
	{prefix: "primera", manufacturer: "Primera"},
	{prefix: "rollo", manufacturer: "Rollo"},
	{prefix: "sato", manufacturer: "SATO"},
	{prefix: "tallygenicom", manufacturer: "TallyGenicom"},
	{prefix: "tsc", manufacturer: "TSC"},
	{prefix: "zebra", manufacturer: "Zebra"},

	// Trademarked product lines whose names often appear without the brand
	// (CUPS makes the same inference for "Stylus" and "DesignJet").
	{prefix: "laserjet", manufacturer: "HP", keepPrefix: true},
	{prefix: "officejet", manufacturer: "HP", keepPrefix: true},
	{prefix: "deskjet", manufacturer: "HP", keepPrefix: true},
	{prefix: "designjet", manufacturer: "HP", keepPrefix: true},
	{prefix: "photosmart", manufacturer: "HP", keepPrefix: true},
	{prefix: "pagewide", manufacturer: "HP", keepPrefix: true},
	{prefix: "smart tank", manufacturer: "HP", keepPrefix: true},
	{prefix: "stylus", manufacturer: "EPSON", keepPrefix: true},
	{prefix: "ecotank", manufacturer: "EPSON", keepPrefix: true},
	{prefix: "pixma", manufacturer: "Canon", keepPrefix: true},
	{prefix: "imageclass", manufacturer: "Canon", keepPrefix: true},
	{prefix: "imagerunner", manufacturer: "Canon", keepPrefix: true},
	{prefix: "imageprograf", manufacturer: "Canon", keepPrefix: true},
	{prefix: "selphy", manufacturer: "Canon", keepPrefix: true},
	{prefix: "phaser", manufacturer: "Xerox", keepPrefix: true},
	{prefix: "workcentre", manufacturer: "Xerox", keepPrefix: true},
	{prefix: "versalink", manufacturer: "Xerox", keepPrefix: true},
	{prefix: "altalink", manufacturer: "Xerox", keepPrefix: true},
	{prefix: "docuprint", manufacturer: "Xerox", keepPrefix: true},
	{prefix: "docucentre", manufacturer: "Xerox", keepPrefix: true},
	{prefix: "colorqube", manufacturer: "Xerox", keepPrefix: true},
	{prefix: "ecosys", manufacturer: "KYOCERA", keepPrefix: true},
	{prefix: "taskalfa", manufacturer: "KYOCERA", keepPrefix: true},
	{prefix: "bizhub", manufacturer: "KONICA MINOLTA", keepPrefix: true},
}

// matchManufacturer returns the rule whose prefix words start name, and how
// many leading words of name it covers. Whole words only, case-insensitive;
// the longest prefix wins ("seiko epson" over a shorter match).
func matchManufacturer(name string) (rule manufacturerRule, words int, ok bool) {
	fields := strings.Fields(strings.ToLower(name))
	for _, candidate := range manufacturerRules {
		prefix := strings.Fields(candidate.prefix)
		if len(prefix) > len(fields) || len(prefix) <= words {
			continue
		}
		if equalWords(fields[:len(prefix)], prefix) {
			rule, words, ok = candidate, len(prefix), true
		}
	}
	return rule, words, ok
}

func equalWords(a, b []string) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
