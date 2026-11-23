package core

import (
	"path/filepath"
	"testing"
)

func TestConfigUpdateSwitchesDefaultAndHonorExplicitValues(t *testing.T) {
	defaults := DefaultConfig()
	if !defaults.AutoUpdateCheck || defaults.UpdateBeta {
		t.Fatalf("automatic checks default to on and pre-releases to off: %+v", defaults)
	}
	for _, key := range []string{"auto_update_check", "update_beta"} {
		fallback := key == "auto_update_check"
		read := func(c AppConfig) bool {
			if key == "auto_update_check" {
				return c.AutoUpdateCheck
			}
			return c.UpdateBeta
		}
		for name, tc := range map[string]struct {
			content string
			want    bool
		}{
			"key absent keeps default": {`{"max_render_dpi": 600}`, fallback},
			"explicit true":            {`{"` + key + `": true}`, true},
			"explicit false":           {`{"` + key + `": false}`, false},
			"malformed keeps default":  {`{"` + key + `": "no"}`, fallback},
		} {
			t.Run(key+" "+name, func(t *testing.T) {
				if got := read(loadConfigJSON(t, tc.content)); got != tc.want {
					t.Fatalf("%s = %v, want %v", key, got, tc.want)
				}
			})
		}
	}
}

func TestConfigUpdateBookkeepingKeys(t *testing.T) {
	cfg := loadConfigJSON(t, `{"skipped_update":"v0.2.9-beta.15","last_update_check":1790000000}`)
	if cfg.SkippedUpdate != "v0.2.9-beta.15" || cfg.LastUpdateCheck != 1790000000 {
		t.Fatalf("keys not read: %+v", cfg)
	}
	cfg = loadConfigJSON(t, `{"skipped_update":7,"last_update_check":"yesterday"}`)
	if cfg.SkippedUpdate != "" || cfg.LastUpdateCheck != 0 {
		t.Fatalf("malformed values must keep the defaults: %+v", cfg)
	}
	if cfg = loadConfigJSON(t, `{}`); cfg.SkippedUpdate != "" || cfg.LastUpdateCheck != 0 {
		t.Fatalf("absent keys must be empty: %+v", cfg)
	}
}

func TestConfigUpdateFieldsSurviveSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := DefaultConfig()
	cfg.AutoUpdateCheck, cfg.UpdateBeta = false, false
	cfg.SkippedUpdate, cfg.LastUpdateCheck = "v0.2.9-beta.15", 1790000000
	if err := cfg.SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.AutoUpdateCheck || got.UpdateBeta || got.SkippedUpdate != cfg.SkippedUpdate || got.LastUpdateCheck != cfg.LastUpdateCheck {
		t.Fatalf("update fields lost in a round trip: %+v", got)
	}
}
