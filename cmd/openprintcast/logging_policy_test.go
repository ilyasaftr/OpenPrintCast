package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Directories (slash-separated, relative to the module root) that may keep
// using the stdlib log package: the vendored zeroconf copy, and the logging
// infrastructure itself.
var stdlogExempt = []string{
	"internal/zeroconf",
	"internal/applog",
	"internal/logfile",
}

// TestNoStdlibLogCalls guards the slog migration: application code logs
// through log/slog with levels and attributes, never through log.Print*,
// log.Fatal* or log.Panic*.
func TestNoStdlibLogCalls(t *testing.T) {
	root := moduleRoot(t)
	var offenders []string
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				if isStdlogExempt(rel) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
				found, err := stdlogCalls(path, rel)
				offenders = append(offenders, found...)
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Fatalf("stdlib log calls found; use log/slog with a level and attributes instead:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

func isStdlogExempt(rel string) bool {
	for _, dir := range stdlogExempt {
		if rel == dir {
			return true
		}
	}
	return false
}

// moduleRoot walks up from the working directory to the directory of go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found in any parent directory")
		}
		dir = parent
	}
}

// stdlogCalls lists the "file:line: log.Func" calls of the stdlib log
// package in the Go file at path, resolving the name the package is
// imported under.
func stdlogCalls(path, rel string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	name := stdlogImportName(file)
	if name == "" {
		return nil, nil
	}
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := stdlogFunc(call, name); ok {
			found = append(found, fmt.Sprintf("%s:%d: %s.%s", rel, fset.Position(call.Pos()).Line, name, fn))
		}
		return true
	})
	return found, nil
}

// stdlogImportName is the identifier the file uses for the stdlib "log"
// package, or "" when it does not import it (or imports it as _ or .).
func stdlogImportName(file *ast.File) string {
	for _, spec := range file.Imports {
		if path, err := strconv.Unquote(spec.Path.Value); err != nil || path != "log" {
			continue
		}
		switch {
		case spec.Name == nil:
			return "log"
		case spec.Name.Name == "_" || spec.Name.Name == ".":
			return ""
		default:
			return spec.Name.Name
		}
	}
	return ""
}

// stdlogFunc reports the forbidden function a call to <pkg>.<Func> names.
func stdlogFunc(call *ast.CallExpr, pkg string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != pkg {
		return "", false
	}
	for _, prefix := range []string{"Print", "Fatal", "Panic"} {
		if strings.HasPrefix(sel.Sel.Name, prefix) {
			return sel.Sel.Name, true
		}
	}
	return "", false
}
