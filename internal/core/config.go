package core

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// Render DPI bounds, matching the Python/Rust bridge behavior.
const (
	MinRenderDPI     = 72
	MaxRenderDPI     = 1200
	DefaultRenderDPI = 300
)

// ClampRenderDPI constrains dpi to the supported 72..1200 range.
func ClampRenderDPI(dpi int) int {
	if dpi < MinRenderDPI {
		return MinRenderDPI
	}
	if dpi > MaxRenderDPI {
		return MaxRenderDPI
	}
	return dpi
}

// AppConfig mirrors config.json. Field names match the JSON keys so the
// file stays compatible with the Python implementation.
type AppConfig struct {
	// Upper bound for rasterization DPI, clamped to 72..1200.
	MaxRenderDPI int `json:"max_render_dpi"`
	// Print pages last-to-first (for printers stacking face-up).
	ReversePageOrder bool `json:"reverse_page_order"`
	// Rotate pages automatically so they fit the paper orientation.
	AutoRotateToFit bool `json:"auto_rotate_to_fit"`
	// Per-printer customisation (names, sharing toggles), keyed by
	// WindowsName; absent entries mean "not customised yet".
	Printers            []PrinterEntry `json:"printers,omitempty"`
	ShowVirtualPrinters bool           `json:"show_virtual_printers,omitempty"`
	FolderEnabled       bool           `json:"folder_enabled,omitempty"`
	FolderPath          string         `json:"folder_path,omitempty"`
	// Show a notification when a print job fails.
	NotifyOnFailure bool `json:"notify_on_failure"`
	// Write verbose diagnostics to the application log.
	VerboseLogging bool `json:"verbose_logging"`
	// Remember finished print jobs across restarts (on by default).
	KeepJobHistory bool `json:"keep_job_history"`
	// Ask network printers for their ink and toner levels over SNMP (on by
	// default; off sends no SNMP traffic and reports no levels).
	ReadSupplyLevels bool `json:"read_supply_levels"`
	// Tab the window was last left on ("printers", "jobs" or "settings");
	// the UI falls back to the first tab for anything else.
	LastTab string `json:"last_tab,omitempty"`
	// Section of Settings the page was last left on; the UI falls back to
	// the first section for anything it does not know.
	LastSettingsSection string `json:"last_settings_section,omitempty"`
	// Look for a new version in the background (on by default).
	AutoUpdateCheck bool `json:"auto_update_check"`
	// Offer pre-release versions (such as 1.0.0-beta.1) too; off by default.
	UpdateBeta bool `json:"update_beta"`
	// Tag of the version the user chose to skip; a newer one is offered again.
	SkippedUpdate string `json:"skipped_update,omitempty"`
	// When the last update check finished, in unix seconds; 0 if never. It
	// keeps checks from running more often than every few hours across
	// restarts.
	LastUpdateCheck int64 `json:"last_update_check,omitempty"`
}

// DefaultConfig returns the Python-compatible defaults.
func DefaultConfig() AppConfig {
	return AppConfig{
		MaxRenderDPI: DefaultRenderDPI, NotifyOnFailure: true, KeepJobHistory: true, ReadSupplyLevels: true,
		AutoUpdateCheck: true,
	}
}

// Normalize clamps MaxRenderDPI into range; call after decoding.
func (c *AppConfig) Normalize() { c.MaxRenderDPI = ClampRenderDPI(c.MaxRenderDPI) }

// LoadConfig reads path, merging over defaults. A missing file yields the
// defaults; unknown keys are ignored so future fields never break loading.
func LoadConfig(path string) (AppConfig, error) {
	cfg, err := readConfigFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultConfig(), nil
	}
	return cfg, err
}

// readConfigFile reads and parses path; unlike LoadConfig it reports a
// missing file as an error wrapping fs.ErrNotExist. On error it returns the
// defaults.
func readConfigFile(path string) (AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultConfig(), err
	}
	return parseConfig(data)
}

// parseConfig decodes config JSON over the defaults with a per-key tolerant
// merge (a malformed value keeps its default). Only input that is not a JSON
// object at all is an error, in which case the defaults are returned.
func parseConfig(data []byte) (AppConfig, error) {
	cfg := DefaultConfig()
	var user map[string]json.RawMessage
	if err := json.Unmarshal(data, &user); err != nil {
		return cfg, err
	}
	for key, dst := range map[string]any{
		"reverse_page_order":    &cfg.ReversePageOrder,
		"auto_rotate_to_fit":    &cfg.AutoRotateToFit,
		"printers":              &cfg.Printers,
		"show_virtual_printers": &cfg.ShowVirtualPrinters,
		"folder_enabled":        &cfg.FolderEnabled,
		"folder_path":           &cfg.FolderPath,
		"notify_on_failure":     &cfg.NotifyOnFailure,
		"verbose_logging":       &cfg.VerboseLogging,
		"keep_job_history":      &cfg.KeepJobHistory,
		"read_supply_levels":    &cfg.ReadSupplyLevels,
		"last_tab":              &cfg.LastTab,
		"last_settings_section": &cfg.LastSettingsSection,
		"auto_update_check":     &cfg.AutoUpdateCheck,
		"update_beta":           &cfg.UpdateBeta,
		"skipped_update":        &cfg.SkippedUpdate,
		"last_update_check":     &cfg.LastUpdateCheck,
	} {
		if raw, ok := user[key]; ok {
			_ = json.Unmarshal(raw, dst)
		}
	}
	if raw, ok := user["max_render_dpi"]; ok {
		var dpi int
		if json.Unmarshal(raw, &dpi) == nil {
			cfg.MaxRenderDPI = dpi
		}
	}
	cfg.Normalize()
	return cfg, nil
}
