package winspool

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAllocatePDFOutput(t *testing.T) {
	root := t.TempDir()
	first, err := allocatePDFOutput(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := allocatePDFOutput(root)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Base(second) != "AirPrint.pdf" {
		t.Fatalf("output paths are not distinct PDF paths: %q, %q", first, second)
	}
	if data, err := os.ReadFile(first); err != nil || string(data) != "existing" {
		t.Fatalf("existing output was modified: %q, %v", data, err)
	}
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatalf("driver output file should not exist yet: %v", err)
	}
}

func TestCompletePDFOutputNeedsHeaderAndTrailer(t *testing.T) {
	dir := t.TempDir()
	path, err := allocatePDFOutputIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	if completePDFOutput(path) {
		t.Fatal("missing file was treated as complete")
	}
	for contents, want := range map[string]bool{
		"%PDF-1.7\npartial":              false,
		"not a pdf\n%%EOF\n":             false,
		"%PDF-1.7\ncomplete\n%%EOF\n":    true,
		"%PDF-1.7\ncomplete\n%%EOF \r\n": true,
	} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if got := completePDFOutput(path); got != want {
			t.Errorf("completePDFOutput(%q) = %t, want %t", contents, got, want)
		}
	}
}

func TestAllocatePDFOutputRequiresDirectory(t *testing.T) {
	if _, err := allocatePDFOutput(""); err == nil {
		t.Fatal("expected an error for an unavailable output directory")
	}
}

func TestPublishFolderPDF(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "1.pdf")
	if err := os.WriteFile(existing, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	const count = 12
	var wg sync.WaitGroup
	paths := make(chan string, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			staged, err := allocatePDFOutputIn(dir)
			if err != nil {
				t.Error(err)
				return
			}
			if err := os.WriteFile(staged, []byte("%PDF-test\n%%EOF\n"), 0600); err != nil {
				t.Error(err)
				return
			}
			path, err := publishFolderPDF(staged, dir, "1.pdf")
			if err != nil {
				t.Error(err)
				return
			}
			paths <- path
		}()
	}
	wg.Wait()
	close(paths)
	seen := make(map[string]bool)
	for path := range paths {
		if filepath.Dir(path) != dir || !strings.HasPrefix(filepath.Base(path), "1-") || filepath.Ext(path) != ".pdf" || seen[path] {
			t.Errorf("invalid or duplicate output path: %q", path)
		}
		seen[path] = true
		if data, err := os.ReadFile(path); err != nil || string(data) != "%PDF-test\n%%EOF\n" {
			t.Errorf("output contents = %q, error = %v", data, err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(seen) != count || len(entries) != count+1 {
		t.Fatalf("outputs = %d, directory entries = %v, error = %v", len(seen), entries, err)
	}
	if data, err := os.ReadFile(existing); err != nil || string(data) != "existing" {
		t.Fatalf("existing file changed: %q, %v", data, err)
	}
}

func TestPublishFolderPDFPreservesStagedFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	staged, err := allocatePDFOutputIn(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staged, []byte("pdf"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := publishFolderPDF(staged, filepath.Join(dir, "missing"), "test.pdf"); err == nil {
		t.Fatal("expected a destination error")
	}
	if data, err := os.ReadFile(staged); err != nil || string(data) != "pdf" {
		t.Fatalf("staged file lost: %q, %v", data, err)
	}
}

func TestPDFNameStem(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"1.pdf", "1"},
		{`C:\private\report.pdf`, "report"},
		{"../../report.pdf", "report"},
		{"bad:*?name.png", "bad___name"},
		{"CON.txt.pdf", "_CON.txt"},
		{"", "AirPrint"},
		{"...", "AirPrint"},
		{strings.Repeat("a", 100) + ".pdf", strings.Repeat("a", 80)},
	} {
		if got := pdfNameStem(tc.name); got != tc.want {
			t.Errorf("pdfNameStem(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}
