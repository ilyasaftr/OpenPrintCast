package net

import (
	"slices"

	"github.com/OpenPrinting/goipp"
)

// stringsAttr builds an attribute whose values are all strings of one tag.
func stringsAttr(name string, tag goipp.Tag, values ...string) goipp.Attribute {
	attr := goipp.Attribute{Name: name}
	for _, value := range values {
		attr.Values.Add(tag, goipp.String(value))
	}
	return attr
}

func keywordAttr(name string, keywords ...string) goipp.Attribute {
	return stringsAttr(name, goipp.TagKeyword, keywords...)
}

func intAttr(name string, value int) goipp.Attribute {
	return goipp.MakeAttr(name, goipp.TagInteger, goipp.Integer(value))
}

func enumAttr(name string, values ...int) goipp.Attribute {
	attr := goipp.Attribute{Name: name}
	for _, value := range values {
		attr.Values.Add(goipp.TagEnum, goipp.Integer(value))
	}
	return attr
}

func boolAttr(name string, value bool) goipp.Attribute {
	return goipp.MakeAttr(name, goipp.TagBoolean, goipp.Boolean(value))
}

func rangeAttr(name string, lower, upper int) goipp.Attribute {
	return goipp.MakeAttr(name, goipp.TagRange, goipp.Range{Lower: lower, Upper: upper})
}

func resolutionAttr(name string, resolution goipp.Resolution) goipp.Attribute {
	return goipp.MakeAttr(name, goipp.TagResolution, resolution)
}

// voidAttr builds an attribute that carries a value-less tag such as
// unknown or no-value.
func voidAttr(name string, tag goipp.Tag) goipp.Attribute {
	return goipp.MakeAttr(name, tag, goipp.Void{})
}

// collectionAttr copies col so attributes never share a backing array.
func collectionAttr(name string, col goipp.Collection) goipp.Attribute {
	return goipp.MakeAttr(name, goipp.TagBeginCollection, slices.Clone(col))
}

func isNameOrKeyword(tag goipp.Tag) bool {
	return tag == goipp.TagKeyword || tag == goipp.TagName
}

// hasSingleValue reports whether attr carries exactly one value of tag.
func hasSingleValue(attr goipp.Attribute, tag goipp.Tag) bool {
	return len(attr.Values) == 1 && attr.Values[0].T == tag
}

// singleValue reports whether attr carries exactly one value of the given
// tag whose text form is want.
func singleValue(attr goipp.Attribute, tag goipp.Tag, want string) bool {
	return len(attr.Values) == 1 && attr.Values[0].T == tag && attr.Values[0].V.String() == want
}

// firstValue is the text of attr's first value, or "" when it has none.
func firstValue(attr goipp.Attribute) string {
	if len(attr.Values) == 0 {
		return ""
	}
	return attr.Values[0].V.String()
}
