package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func writeConfigFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfigRestoresFromBackupAndRewritesMain(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)
	writeConfigFile(t, path, "{corrupt")
	writeConfigFile(t, path+".bak", `{"folder_path":"Lab","max_render_dpi":600}`)

	cfg := loadConfig(path)
	if cfg.FolderPath != "Lab" || cfg.MaxRenderDPI != 600 {
		t.Fatalf("restored cfg = %+v", cfg)
	}
	reloaded, err := core.LoadConfig(path)
	if err != nil || reloaded.FolderPath != "Lab" {
		t.Fatalf("main not rewritten: %+v err %v", reloaded, err)
	}
	if _, err := os.Stat(path + ".corrupt"); err != nil {
		t.Fatalf("corrupt file not kept aside: %v", err)
	}
}

func TestLoadConfigFallsBackToDefaultsWhenUnrecoverable(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)
	writeConfigFile(t, path, "{corrupt")

	if cfg := loadConfig(path); cfg.FolderPath != "" || cfg.MaxRenderDPI != core.DefaultRenderDPI {
		t.Fatalf("want defaults, got %+v", cfg)
	}
}
