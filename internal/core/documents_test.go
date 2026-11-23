package core

import (
	"slices"
	"strings"
	"testing"
)

func TestLookupDocumentFormatIsAliasAndCaseAware(t *testing.T) {
	cases := map[string]string{
		"application/pdf":                  MIMEPDF,
		"Application/PDF":                  MIMEPDF,
		" application/pdf; charset=utf-8 ": MIMEPDF,
		"application/x-pdf":                MIMEPDF,
		"application/vnd.cups-pdf":         MIMEPDF,
		"application/vnd.cups-pdf-banner":  MIMEPDF,
		"image/jpg":                        MIMEJPEG,
		"image/x-png":                      MIMEPNG,
		"image/urf":                        MIMEURF,
		"image/pwg-raster":                 MIMEPWG,
		"application/postscript":           "",
		"application/pclm":                 "",
		"application/octet-stream":         "",
		"":                                 "",
	}
	for declared, want := range cases {
		got, ok := LookupDocumentFormat(declared)
		if got.MIME != want || ok != (want != "") {
			t.Errorf("LookupDocumentFormat(%q) = %q, %v, want %q", declared, got.MIME, ok, want)
		}
	}
}

func TestRegistryIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, f := range DocumentFormats() {
		names := append([]string{f.MIME}, f.Aliases...)
		for _, name := range names {
			if name != NormalizeMIME(name) || seen[name] {
				t.Errorf("%q is not normalized or listed twice", name)
			}
			seen[name] = true
		}
		if f.Sniff == nil || f.MIME == MIMEOctetStream {
			t.Errorf("%q has no detector or is the auto-sense type", f.MIME)
		}
	}
	if got := strings.Split(AdvertisedPDL, ","); !slices.Equal(got, AdvertisedMIMEs()) {
		t.Errorf("AdvertisedPDL = %v, want the registry's %v", got, AdvertisedMIMEs())
	}
}
