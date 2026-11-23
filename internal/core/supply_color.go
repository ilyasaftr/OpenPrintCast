package core

import "strings"

// colorantHex maps the standard colorant names of PWG 5100.13 (Table 22,
// "printer-supply" Standard Colorant Names) to their sRGB values, in the
// "#RRGGBB" form CUPS' marker-colors use.
var colorantHex = map[string]string{
	"black":         "#000000",
	"light-black":   "#808080",
	"blue":          "#0000FF",
	"cyan":          "#00FFFF",
	"light-cyan":    "#E0FFFF",
	"gold":          "#FFD700",
	"gray":          "#808080",
	"dark-gray":     "#404040",
	"light-gray":    "#D3D3D3",
	"green":         "#008000",
	"magenta":       "#FF00FF",
	"light-magenta": "#FF77FF",
	"orange":        "#FFA500",
	"red":           "#FF0000",
	"silver":        "#C0C0C0",
	"white":         "#FFFFFF",
	"yellow":        "#FFFF00",
	"dark-yellow":   "#FFCC00",
	"violet":        "#EE82EE",
}

// ColorantHex is the sRGB value of a standard colorant name, or "" for a
// name outside the standard (its colour is not guessed).
func ColorantHex(name string) string { return colorantHex[name] }

// noColorantNames are the values prtMarkerColorantValue (RFC 3805) and the
// PWG use for "no single colour".
var noColorantNames = []string{"", "other", "unknown", "no-color", "none"}

// normalizeColorant lower-cases a colorant name, joins its words with
// hyphens and keeps only characters a PWG colorant name allows
// ("Light Cyan" -> "light-cyan"). Names that mean "no colour" give "".
func normalizeColorant(name string) string {
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 'a' - 'A'
		case r == ' ', r == '_':
			return '-'
		}
		return -1
	}, name)
	name = strings.Trim(name, "-")
	for _, none := range noColorantNames {
		if name == none {
			return ""
		}
	}
	return name
}
