package core

import (
	"slices"
	"testing"
)

func TestSidesKeywordsRoundTrip(t *testing.T) {
	for sides, keyword := range map[Sides]string{
		OneSided:          "one-sided",
		TwoSidedLongEdge:  "two-sided-long-edge",
		TwoSidedShortEdge: "two-sided-short-edge",
	} {
		if got := sides.Keyword(); got != keyword {
			t.Errorf("%d.Keyword() = %q, want %q", sides, got, keyword)
		}
		if parsed, ok := ParseSides(keyword); !ok || parsed != sides {
			t.Errorf("ParseSides(%q) = %v, %t, want %v", keyword, parsed, ok, sides)
		}
	}
}

func TestParseSidesRejectsOtherKeywords(t *testing.T) {
	for _, keyword := range []string{"", "ONE-SIDED", "duplex", "two-sided", "two-sided-long-edge "} {
		if sides, ok := ParseSides(keyword); ok || sides != OneSided {
			t.Errorf("ParseSides(%q) = %v, %t, want one-sided, false", keyword, sides, ok)
		}
	}
}

func TestSidesZeroValueIsOneSided(t *testing.T) {
	var sides Sides
	if sides != OneSided || sides.Keyword() != "one-sided" {
		t.Fatalf("zero Sides = %v (%q)", sides, sides.Keyword())
	}
}

func TestSidesValidity(t *testing.T) {
	for _, sides := range []Sides{OneSided, TwoSidedLongEdge, TwoSidedShortEdge} {
		if !sides.Valid() {
			t.Errorf("%v must be valid", sides)
		}
	}
	for _, sides := range []Sides{-1, 3, 99} {
		if sides.Valid() || sides.Keyword() != "" {
			t.Errorf("%d must be invalid with no keyword", sides)
		}
	}
}

func TestSidesKeywordsListsOneSidedFirstAndIsACopy(t *testing.T) {
	want := []string{"one-sided", "two-sided-long-edge", "two-sided-short-edge"}
	got := SidesKeywords()
	if !slices.Equal(got, want) {
		t.Fatalf("SidesKeywords() = %v, want %v", got, want)
	}
	got[0] = "changed"
	if SidesKeywords()[0] != "one-sided" {
		t.Fatal("SidesKeywords must return a copy")
	}
}

func TestPrintOptionsSides(t *testing.T) {
	if !(PrintOptions{}).IsDefault() || !(PrintOptions{Sides: OneSided}).IsDefault() {
		t.Error("one-sided options must stay the default")
	}
	for _, sides := range []Sides{TwoSidedLongEdge, TwoSidedShortEdge} {
		options := PrintOptions{Sides: sides}
		if options.IsDefault() || !options.Valid() {
			t.Errorf("%v: IsDefault = %t, Valid = %t", sides, options.IsDefault(), options.Valid())
		}
	}
	if (PrintOptions{Sides: 3}).Valid() || (PrintOptions{Sides: -1}).Valid() {
		t.Error("an undefined Sides value must be invalid")
	}
}
