package net

import (
	"strconv"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func mediaChoiceForName(attr goipp.Attribute, shared *SharedState, printer core.PrinterInfo) (mediaChoice, bool) {
	if len(attr.Values) != 1 || !isNameOrKeyword(attr.Values[0].T) {
		return mediaChoice{}, false
	}
	_, choices := mediaChoices(shared, printer)
	for _, choice := range choices {
		if attr.Values[0].V.String() == choice.name {
			return choice, true
		}
	}
	return mediaChoice{}, false
}

func mediaChoiceForCollection(attr goipp.Attribute, shared *SharedState, printer core.PrinterInfo) (mediaChoice, bool) {
	if len(attr.Values) != 1 || attr.Values[0].T != goipp.TagBeginCollection {
		return mediaChoice{}, false
	}
	requested, ok := attr.Values[0].V.(goipp.Collection)
	if !ok {
		return mediaChoice{}, false
	}
	_, choices := mediaChoices(shared, printer)
	for _, choice := range choices {
		if matchesMediaCollection(requested, choice) {
			return choice, true
		}
	}
	return mediaChoice{}, false
}

// matchesMediaCollection accepts a requested media-col only when every
// member agrees with choice and the size is pinned down.
func matchesMediaCollection(requested goipp.Collection, choice mediaChoice) bool {
	matchedSize := false
	seen := make(map[string]bool)
	for _, member := range requested {
		if seen[member.Name] || len(member.Values) != 1 {
			return false
		}
		seen[member.Name] = true
		matches, definesSize := matchesMediaMember(member, choice)
		if !matches {
			return false
		}
		matchedSize = matchedSize || definesSize
	}
	return matchedSize
}

// matchesMediaMember reports whether one single-valued media-col member
// agrees with choice, and whether that member identifies the paper size.
func matchesMediaMember(member goipp.Attribute, choice mediaChoice) (matches, definesSize bool) {
	tag, value := member.Values[0].T, member.Values[0].V
	switch member.Name {
	case "media-size-name", "media-key":
		matches = isNameOrKeyword(tag) && value.String() == choice.name
		return matches, matches
	case "media-size":
		matches = matchesMediaSize(member, choice)
		return matches, matches
	case "media-source", "media-type":
		return tag == goipp.TagKeyword && value.String() == valueAuto, false
	case "media-left-margin", "media-right-margin", "media-top-margin", "media-bottom-margin":
		return matchesMediaMargin(member, choice), false
	default:
		return false, false
	}
}

func matchesMediaSize(member goipp.Attribute, choice mediaChoice) bool {
	if member.Values[0].T != goipp.TagBeginCollection {
		return false
	}
	size, ok := member.Values[0].V.(goipp.Collection)
	if !ok || len(size) != 2 ||
		opAttrString(goipp.Attributes(size), "x-dimension") != strconv.Itoa(choice.width) ||
		opAttrString(goipp.Attributes(size), "y-dimension") != strconv.Itoa(choice.height) {
		return false
	}
	for _, dimension := range size {
		if (dimension.Name != "x-dimension" && dimension.Name != "y-dimension") ||
			len(dimension.Values) != 1 || dimension.Values[0].T != goipp.TagInteger {
			return false
		}
	}
	return true
}

func matchesMediaMargin(member goipp.Attribute, choice mediaChoice) bool {
	if member.Values[0].T != goipp.TagInteger {
		return false
	}
	for _, margin := range mediaMargins(choice.margins) {
		if margin.name == member.Name {
			return member.Values[0].V.String() == strconv.Itoa(margin.value)
		}
	}
	return false
}
