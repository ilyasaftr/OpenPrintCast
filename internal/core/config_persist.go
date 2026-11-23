package core

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Config persistence layout and permissions.
const (
	configDirPerm  fs.FileMode = 0o755
	configFilePerm fs.FileMode = 0o644
	// backupSuffix names the last known-good copy beside the config file.
	backupSuffix = ".bak"
	// corruptSuffix names where an unreadable config is moved for inspection.
	corruptSuffix = ".corrupt"
)

// SaveConfig writes cfg as pretty JSON, creating parent dirs as needed. The
// write is atomic (temp file + replace), so a crash never leaves a truncated
// config; the previous valid content is kept as <path>.bak.
func (c AppConfig) SaveConfig(path string) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, configDirPerm); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	backupValidConfig(path)
	return writeFileAtomic(path, append(data, '\n'), configFilePerm)
}

// backupValidConfig copies the current config to <path>.bak, but only when
// it exists and is valid JSON, so a good backup is never replaced by a
// corrupt file. Best-effort: a failed backup must not block saving.
func backupValidConfig(path string) {
	current, err := os.ReadFile(path)
	if err != nil || !json.Valid(current) {
		return
	}
	_ = writeFileAtomic(path+backupSuffix, current, configFilePerm)
}

// LoadConfigRecovering is LoadConfig with a safety net: when the config file
// exists but is unreadable or unparsable, it falls back to <path>.bak and
// reports recovered=true (moving the bad file to <path>.corrupt). If the
// backup is unusable too it returns the defaults and the original error.
func LoadConfigRecovering(path string) (cfg AppConfig, recovered bool, err error) {
	cfg, err = readConfigFile(path)
	if err == nil {
		return cfg, false, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultConfig(), false, nil
	}
	backup, backupErr := readConfigFile(path + backupSuffix)
	if backupErr != nil {
		return DefaultConfig(), false, err
	}
	setCorruptConfigAside(path)
	return backup, true, nil
}

// setCorruptConfigAside moves the unreadable config to <path>.corrupt,
// overwriting any earlier one. Best-effort: errors are ignored.
func setCorruptConfigAside(path string) {
	_ = os.Rename(path, path+corruptSuffix)
}
