package core

// Sides is the IPP `sides` job option (RFC 8011 section 5.2.8). The zero
// value is one-sided, so unset options keep the single-sided behavior.
type Sides int

const (
	OneSided Sides = iota
	TwoSidedLongEdge
	TwoSidedShortEdge
)

// IPP keyword for each Sides value, indexed by the value.
var sidesKeywords = [...]string{
	OneSided:          "one-sided",
	TwoSidedLongEdge:  "two-sided-long-edge",
	TwoSidedShortEdge: "two-sided-short-edge",
}

// ParseSides maps an IPP `sides` keyword to its value.
func ParseSides(keyword string) (Sides, bool) {
	for sides, name := range sidesKeywords {
		if name == keyword {
			return Sides(sides), true
		}
	}
	return OneSided, false
}

// Valid reports whether s is one of the defined values.
func (s Sides) Valid() bool {
	return s >= 0 && int(s) < len(sidesKeywords)
}

// Keyword is the IPP `sides` keyword for s, or "" when s is not valid.
func (s Sides) Keyword() string {
	if !s.Valid() {
		return ""
	}
	return sidesKeywords[s]
}

// SidesKeywords lists every IPP `sides` keyword in value order, one-sided
// first.
func SidesKeywords() []string {
	return append([]string(nil), sidesKeywords[:]...)
}
