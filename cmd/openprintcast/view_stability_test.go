package main

import (
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// diffCount reports how many pixels differ between a and b.
func diffCount(a, b *image.RGBA) int {
	bounds := a.Bounds()
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			ao := a.PixOffset(x, y)
			bo := b.PixOffset(x, y)
			if a.Pix[ao] != b.Pix[bo] || a.Pix[ao+1] != b.Pix[bo+1] || a.Pix[ao+2] != b.Pix[bo+2] {
				count++
			}
		}
	}
	return count
}

// copyFrame deep-copies the frame since the tester reuses its buffer.
func copyFrame(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

// movementAfter advances the clock past a transition and totals how many
// pixels change between the consecutive frames an animation would move.
func movementAfter(tt *ui.Tester) int {
	prev := copyFrame(tt.Image())
	total := 0
	for i := 0; i < 5; i++ {
		tt.Advance(40 * time.Millisecond)
		tt.Frame()
		cur := copyFrame(tt.Image())
		total += diffCount(prev, cur)
		prev = cur
	}
	return total
}

// settleView renders out any pending work and lets animations finish.
func settleView(tt *ui.Tester) {
	for i := 0; i < 4; i++ {
		tt.Frame()
	}
	tt.Advance(300 * time.Millisecond)
	tt.Frame()
}

// TestSettingsSectionSwitchingIsStable walks every ordered pair of
// settings sections and both tab entries into Settings, and nothing may
// move between the frames after the switch: the controls of the section
// that left must not pass their animation state to the ones arriving.
// Element identity is positional unless keyed, which is exactly what the
// keys of the settings rows and the pane prevent.
func TestSettingsSectionSwitchingIsStable(t *testing.T) {
	sections := []string{"General", "Printing", "Network", "Updates", "About"}
	app := newTestApp(t)
	// Sections must differ in switch values, or an inherited animation
	// channel would have nothing to travel to.
	app.reversePageOrder = true
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, from := range sections {
		for _, to := range sections {
			if from == to {
				continue
			}
			if err := tt.Click(from); err != nil {
				t.Fatalf("click %s: %v", from, err)
			}
			settleView(tt)
			if err := tt.Click(to); err != nil {
				t.Fatalf("click %s: %v", to, err)
			}
			if n := movementAfter(tt); n > 0 {
				t.Errorf("%s -> %s: %d pixels moved after the section switch", from, to, n)
			}
		}
	}
	for _, tab := range []string{"Printers", "Jobs"} {
		if err := tt.Click(tab); err != nil {
			t.Fatal(err)
		}
		settleView(tt)
		if err := tt.Click("Settings"); err != nil {
			t.Fatal(err)
		}
		if n := movementAfter(tt); n > 0 {
			t.Errorf("%s -> Settings: %d pixels moved after the tab switch", tab, n)
		}
	}
}

// TestSettingsSwitchAnimatesOnToggle guards the keying against freezing
// the control itself: a switch the user toggles must still slide its knob.
func TestSettingsSwitchAnimatesOnToggle(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	settleView(tt)
	sw, ok := tt.AccessByRole(ui.RoleSwitch, "Start with Windows")
	if !ok {
		t.Fatal("switch not found in the access tree")
	}
	tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	if n := movementAfter(tt); n == 0 {
		t.Error("the knob did not move after the switch was toggled")
	}
	if !app.openAtLogin {
		t.Error("the click did not toggle the setting")
	}
}

// TestSettingsContentCentersWhenWide checks the wide-window layout: at a
// maximized width the settings column sits centered in the pane beside the
// sidebar rather than clinging to it, and at the default width it fills
// the pane as before.
func TestSettingsContentCentersWhenWide(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 1920, 1040)
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	settleView(tt)
	r, ok := tt.Find("Start with Windows")
	if !ok {
		t.Fatal("setting row not found")
	}
	// Centered in the pane: the pane starts past the sidebar (16 + 176 +
	// 12 = 204) and runs 1700 wide, so a 640-wide column begins near 734;
	// anything far left of that is the old cling-to-the-sidebar layout.
	if r.X < 500 {
		t.Errorf("wide window: settings content starts at x %.0f, not centered in the pane", r.X)
	}
	tt.SetSize(860, 620)
	settleView(tt)
	r, ok = tt.Find("Start with Windows")
	if !ok {
		t.Fatal("setting row not found after resize")
	}
	if r.X > 300 {
		t.Errorf("default window: settings content starts at x %.0f, no longer filling the pane", r.X)
	}
}
