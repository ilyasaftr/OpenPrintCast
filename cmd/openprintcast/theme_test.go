package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

// Minimum WCAG 2.x contrast ratios: text, and symbols or graphics.
const (
	minTextContrast   = 4.5
	minSymbolContrast = 3.0
)

// channel is one sRGB channel linearized, as WCAG defines it.
func channel(v uint8) float64 {
	c := float64(v) / 255
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func luminance(c ui.Color) float64 {
	return 0.2126*channel(c.R) + 0.7152*channel(c.G) + 0.0722*channel(c.B)
}

// contrastRatio is the WCAG contrast ratio of two opaque colors.
func contrastRatio(a, b ui.Color) float64 {
	hi, lo := luminance(a), luminance(b)
	if lo > hi {
		hi, lo = lo, hi
	}
	return (hi + 0.05) / (lo + 0.05)
}

func TestContrastRatioMatchesKnownValues(t *testing.T) {
	if got := contrastRatio(ui.Hex("#000000"), ui.Hex("#ffffff")); math.Abs(got-21) > 0.01 {
		t.Fatalf("black on white = %.2f, want 21", got)
	}
	if got := contrastRatio(ui.Hex("#777777"), ui.Hex("#ffffff")); math.Abs(got-4.48) > 0.02 {
		t.Fatalf("#777 on white = %.2f, want 4.48", got)
	}
}

// TestThemesMeetContrast pins the contrast of every text color the views
// use on the two backgrounds they sit on, in both appearances.
func TestThemesMeetContrast(t *testing.T) {
	for _, base := range []*ui.Theme{ui.LightTheme(), ui.DarkTheme()} {
		theme := accessibleTheme(base)
		name := "light"
		if theme.Dark {
			name = "dark"
		}
		for _, bg := range []struct {
			name  string
			color ui.Color
		}{{"window", theme.Background}, {"list row", theme.Surface}} {
			text := map[string]ui.Color{
				"Text": theme.Text, "TextMuted": theme.TextMuted, "Warning": theme.Warning, "Success": theme.Success,
			}
			for color, fg := range text {
				if got := contrastRatio(fg, bg.color); got < minTextContrast {
					t.Errorf("%s %s on %s = %.2f:1, want at least %.1f", name, color, bg.name, got, minTextContrast)
				}
			}
			floor := minTextContrast
			if theme.Dark && bg.name == "list row" {
				floor = minSymbolContrast // dark Danger is only used as a symbol on rows
			}
			if got := contrastRatio(theme.Danger, bg.color); got < floor {
				t.Errorf("%s Danger on %s = %.2f:1, want at least %.1f", name, bg.name, got, floor)
			}
		}
	}
}

func TestAccessibleThemeOnlyFillsTheDarkAccent(t *testing.T) {
	dark := ui.DarkTheme()
	got := accessibleTheme(dark)
	want := *dark
	want.Accent, want.AccentHover, want.AccentPressed = darkFillAccent, darkFillAccentHover, darkFillAccentPressed
	if *got != want {
		t.Fatalf("the dark theme changed beyond its accent fill: %+v", got)
	}
	if again := accessibleTheme(got); *again != *got {
		t.Fatal("applying the theme twice changed it again")
	}
}

// blend is the opaque color c shows as over the opaque color under, which
// is how the translucent focus ring reaches the eye.
func blend(c, under ui.Color) ui.Color {
	a := float64(c.A) / 255
	mix := func(top, bottom uint8) uint8 { return uint8(math.Round(a*float64(top) + (1-a)*float64(bottom))) }
	return ui.RGB(mix(c.R, under.R), mix(c.G, under.G), mix(c.B, under.B))
}

// TestDarkAccentFillsMeetContrast pins the dark accent's contrast: white
// text on every state of a filled control, the fill's own edge against
// both backgrounds it sits on (switch tracks, checked checkboxes), the
// focus ring, and the lighter accent kept for text and icons.
func TestDarkAccentFillsMeetContrast(t *testing.T) {
	theme := accessibleTheme(ui.DarkTheme())
	for name, fill := range map[string]ui.Color{
		"Accent": theme.Accent, "AccentHover": theme.AccentHover, "AccentPressed": theme.AccentPressed,
	} {
		if got := contrastRatio(theme.AccentText, fill); got < minTextContrast {
			t.Errorf("%s text on %s = %.2f:1, want at least %.1f", "AccentText", name, got, minTextContrast)
		}
	}
	if stock := ui.DarkTheme(); contrastRatio(stock.AccentText, stock.Accent) >= minTextContrast {
		t.Fatal("setup: the stock accent already carries white text; the fill is not needed")
	}
	for name, bg := range map[string]ui.Color{"window": theme.Background, "list row": theme.Surface} {
		if got := contrastRatio(theme.Accent, bg); got < minSymbolContrast {
			t.Errorf("fill accent on %s = %.2f:1, want at least %.1f", name, got, minSymbolContrast)
		}
		if got := contrastRatio(blend(theme.Focus, bg), bg); got < minSymbolContrast {
			t.Errorf("focus ring on %s = %.2f:1, want at least %.1f", name, got, minSymbolContrast)
		}
	}
	fg := accentForeground(theme)
	if got := contrastRatio(fg, theme.Background); got < minTextContrast {
		t.Errorf("accent text on the window = %.2f:1, want at least %.1f", got, minTextContrast)
	}
	if got := contrastRatio(fg, theme.Surface); got < minSymbolContrast {
		t.Errorf("accent icon on a list row = %.2f:1, want at least %.1f", got, minSymbolContrast)
	}
	if fg != ui.DarkTheme().Accent {
		t.Fatalf("accent for text = %v, want the original lighter accent", fg)
	}
}

func TestDarkAccentKeepsTheDesktopsAccent(t *testing.T) {
	custom := ui.DarkTheme().WithAccent(ui.Hex("#e11d48"))
	got := accessibleTheme(custom)
	if got.Accent != custom.Accent || got.AccentHover != custom.AccentHover || got.AccentPressed != custom.AccentPressed {
		t.Fatal("an accent the desktop chose was overridden")
	}
	if accentForeground(got) != custom.Accent {
		t.Fatal("accent text must follow the desktop's accent")
	}
}

func TestLightAccentIsLeftAlone(t *testing.T) {
	light := ui.LightTheme()
	if got := accessibleTheme(light); got.Accent != light.Accent || accentForeground(got) != light.Accent {
		t.Fatal("the light theme's accent changed")
	}
}

func TestAccessibleThemeKeepsColorsTheUserChanged(t *testing.T) {
	custom := ui.LightTheme()
	custom.TextMuted = ui.Hex("#222222") // as a high-contrast setting would
	got := accessibleTheme(custom)
	if got.TextMuted != custom.TextMuted {
		t.Fatal("a color changed by the desktop's contrast setting was overridden")
	}
	if got.Warning == ui.LightTheme().Warning {
		t.Fatal("the stock warning color was not darkened")
	}
	if ui.LightTheme().TextMuted == lightTextMuted {
		t.Fatal("setup: the stock muted color is unchanged")
	}
}
