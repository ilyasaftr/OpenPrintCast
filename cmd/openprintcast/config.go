package main

import (
	"os"
	"path/filepath"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// configFileName is the JSON settings file. It lives in the per-user
// config directory — %APPDATA%\OpenPrintCast on Windows, ~/.config on
// Linux — never beside the executable, which may not be writable
// (Program Files) and should stay clean of state.
const configFileName = "config.json"

// configDirMode is the permission of the created config directory.
const configDirMode = 0o755

// configStore is where the bridge persists its settings. The UI model
// depends on this seam, not on a file path, so storage can be swapped
// (and faked in tests).
type configStore interface {
	// Save persists cfg.
	Save(cfg core.AppConfig) error
	// Location describes where settings live, for display.
	Location() string
}

// fileConfigStore keeps the settings in a JSON file at path.
type fileConfigStore struct{ path string }

// newFileConfigStore stores settings at the per-user config path.
func newFileConfigStore() fileConfigStore { return fileConfigStore{path: configPath()} }

// Load reads the stored config, applying first-run defaults and the legacy
// migration (see loadConfig).
func (s fileConfigStore) Load() core.AppConfig { return loadConfig(s.path) }

func (s fileConfigStore) Save(cfg core.AppConfig) error { return cfg.SaveConfig(s.path) }

func (s fileConfigStore) Location() string { return s.path }

// legacyConfigPath is the pre-0.2.x location beside the executable,
// honoured once for migration and then abandoned.
func legacyConfigPath() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), configFileName)
}

// configPath resolves where settings load from and save to, creating the
// directory. Falls back to the temp dir only when no config home exists.
func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "OpenPrintCast")
	if err := os.MkdirAll(dir, configDirMode); err != nil {
		return filepath.Join(os.TempDir(), configFileName)
	}
	return filepath.Join(dir, configFileName)
}

// loadConfig reads the per-user config, migrating the legacy exe-side file
// once: if the new path is missing but the legacy one exists, its values
// are adopted and immediately re-saved to the new location. A missing file
// is normal on first run, never an error.
func loadConfig(path string) core.AppConfig {
	if _, err := os.Stat(path); err == nil {
		return loadExistingConfig(path)
	}
	return migrateLegacyConfig(path)
}

// loadExistingConfig loads the config at path, restoring it from its backup
// when the file is unreadable (and re-saving so the main file is valid
// again). If nothing is recoverable it falls back to the defaults.
func loadExistingConfig(path string) core.AppConfig {
	cfg, recovered, err := core.LoadConfigRecovering(path)
	if err != nil {
		componentLogger(componentConfig).Warn("unreadable config, using defaults", "path", path, "err", err)
		return core.DefaultConfig()
	}
	if recovered {
		componentLogger(componentConfig).Warn("config was unreadable; restored from backup", "path", path)
		if err := cfg.SaveConfig(path); err != nil {
			componentLogger(componentConfig).Warn("could not rewrite restored config", "path", path, "err", err)
		}
	}
	return cfg
}

// migrateLegacyConfig adopts the legacy exe-side config, if any, saving it
// to path. Without a usable legacy file it returns the defaults.
func migrateLegacyConfig(path string) core.AppConfig {
	if legacy := legacyConfigPath(); legacy != "" && legacy != path {
		if _, err := os.Stat(legacy); err == nil {
			if cfg, err := core.LoadConfig(legacy); err == nil {
				cfg.SaveConfig(path)
				componentLogger(componentConfig).Info("migrated config", "from", legacy, "path", path)
				return cfg
			}
		}
	}
	return core.DefaultConfig()
}
