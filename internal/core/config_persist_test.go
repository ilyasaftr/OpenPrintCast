package core

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

const goodJSON = `{"folder_path":"Lab","max_render_dpi":600,"verbose_logging":true}`

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSaveLoadRoundTripIncludesVerboseLogging(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	want := DefaultConfig()
	want.FolderPath, want.VerboseLogging = "Lab", true
	if err := want.SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(path)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v err %v, want %+v", got, err, want)
	}
}

func TestVerboseLoggingDefaultsOffAndIsTolerant(t *testing.T) {
	tests := []struct {
		name, content string
		want          bool
	}{
		{"absent", `{}`, false},
		{"true", `{"verbose_logging":true}`, true},
		{"malformed value keeps default", `{"verbose_logging":"yes"}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := loadConfigJSON(t, tc.content).VerboseLogging; got != tc.want {
				t.Fatalf("VerboseLogging = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSaveConfigBackup(t *testing.T) {
	tests := []struct {
		name     string
		existing string
		wantBak  string // empty means no .bak expected
	}{
		{"valid existing content is backed up", goodJSON, goodJSON},
		{"corrupt existing content is not backed up", `{"printer_na`, ""},
		{"no existing file", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if tc.existing != "" {
				writeTestFile(t, path, tc.existing)
			}
			if err := DefaultConfig().SaveConfig(path); err != nil {
				t.Fatal(err)
			}
			_, err := os.Stat(path + ".bak")
			if tc.wantBak == "" {
				if !os.IsNotExist(err) {
					t.Fatalf("unexpected .bak (err %v)", err)
				}
				return
			}
			if got := mustReadFile(t, path+".bak"); got != tc.wantBak {
				t.Fatalf(".bak = %q, want %q", got, tc.wantBak)
			}
		})
	}
}

func TestSaveConfigKeepsGoodBackupWhenMainIsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path, "garbage")
	writeTestFile(t, path+".bak", goodJSON)
	if err := DefaultConfig().SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	if got := mustReadFile(t, path+".bak"); got != goodJSON {
		t.Fatalf("good .bak overwritten: %q", got)
	}
}

func TestLoadConfigRecovering(t *testing.T) {
	tests := []struct {
		name          string
		main, bak     string // empty means the file does not exist
		wantRecovered bool
		wantErr       bool
		wantPrinter   string
	}{
		{"missing file gives defaults", "", "", false, false, ""},
		{"valid main", goodJSON, "", false, false, "Lab"},
		{"corrupt main, good bak", "{broken", `{"folder_path":"Backup"}`, true, false, "Backup"},
		{"corrupt main, no bak", "{broken", "", false, true, ""},
		{"corrupt both", "{broken", "{also broken", false, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if tc.main != "" {
				writeTestFile(t, path, tc.main)
			}
			if tc.bak != "" {
				writeTestFile(t, path+".bak", tc.bak)
			}
			cfg, recovered, err := LoadConfigRecovering(path)
			if recovered != tc.wantRecovered || (err != nil) != tc.wantErr {
				t.Fatalf("recovered=%v err=%v", recovered, err)
			}
			if cfg.FolderPath != tc.wantPrinter || cfg.MaxRenderDPI == 0 {
				t.Fatalf("cfg = %+v", cfg)
			}
			if tc.wantErr && !reflect.DeepEqual(cfg, DefaultConfig()) {
				t.Fatalf("want defaults on failure, got %+v", cfg)
			}
		})
	}
}

func TestLoadConfigRecoveringMovesCorruptFileAside(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path+".corrupt", "older corrupt")
	writeTestFile(t, path, "{broken")
	writeTestFile(t, path+".bak", goodJSON)
	if _, recovered, err := LoadConfigRecovering(path); !recovered || err != nil {
		t.Fatalf("recovered=%v err=%v", recovered, err)
	}
	if got := mustReadFile(t, path+".corrupt"); got != "{broken" {
		t.Fatalf(".corrupt = %q", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("corrupt main should be moved away, stat err %v", err)
	}
}

func TestLoadConfigRecoveringLeavesFailedFilesInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeTestFile(t, path, "{broken")
	if _, _, err := LoadConfigRecovering(path); err == nil {
		t.Fatal("want error")
	}
	if got := mustReadFile(t, path); got != "{broken" {
		t.Fatalf("main modified: %q", got)
	}
}

func TestConcurrentSaveConfigLeavesValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := DefaultConfig()
			cfg.FolderPath = fmt.Sprintf("printer-%d", i)
			if err := cfg.SaveConfig(path); err != nil {
				t.Errorf("save %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if _, recovered, err := LoadConfigRecovering(path); err != nil || recovered {
		t.Fatalf("recovered=%v err=%v", recovered, err)
	}
	for _, name := range dirEntries(t, dir) {
		if name != "config.json" && name != "config.json.bak" {
			t.Fatalf("stray file %q", name)
		}
	}
}
