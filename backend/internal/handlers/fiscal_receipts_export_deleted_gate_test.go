package handlers_test

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestFiscalReceiptsServerExportDeletedSourceGate freezes the deletion of the
// server-side fiscal receipts CSV export.
//
// Two export paths for the same data is how a divergence gets shipped, and this
// pair had already diverged three ways before it was removed:
//
//   - the server export ignored the `receipt_type` and `needs_attention`
//     filters entirely, so an operator who filtered the table and hit Export
//     silently downloaded rows they had just filtered out;
//   - it dumped `created_at` in UTC, after the analytics timezone work moved
//     operator-facing windows to business-local time;
//   - it was reachable but unreferenced — no frontend code called it, so the
//     divergence was invisible until someone read both implementations.
//
// The surviving path is the client `exportLocalizedCsv` in
// InvoicesTab.handleExportCsv, which honors every active filter and walks the
// full paginated range rather than the on-screen page.
//
// Modeled on TestAutoMigrateSourceGate / TestPluginPriceDeletedSourceGate: a
// source-level assertion is what keeps a deleted concept deleted.
func TestFiscalReceiptsServerExportDeletedSourceGate(t *testing.T) {
	handlersRoot := handlersPackageRoot(t)

	hits := scanForbidden(t, handlersRoot, "ExportFiscalReceiptsCSV",
		"fiscal_receipts_export_deleted_gate_test.go")
	if len(hits) > 0 {
		t.Fatalf("ExportFiscalReceiptsCSV must stay deleted from backend/internal/handlers/ (%d hits):\n  %s\n\n"+
			"Fiscal receipts export lives in the client exportLocalizedCsv path, which honors the\n"+
			"receipt_type/needs_attention filters the server export ignored. Do not reintroduce a\n"+
			"second export path for the same data.",
			len(hits), strings.Join(hits, "\n  "))
	}
}

// TestFiscalReceiptsExportRouteNotRegistered asserts the route itself is gone
// from main.go. Deleting the handler but leaving a registration behind would
// not compile, but a future re-add could wire a *different* handler to the same
// path and silently restore the divergence — so the path is pinned too.
func TestFiscalReceiptsExportRouteNotRegistered(t *testing.T) {
	_, thisFile, ok := callerDir()
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	mainPath := filepath.Clean(filepath.Join(thisFile, "..", "..", "..", "cmd", "app", "main.go"))

	data, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if strings.Contains(string(data), "fiscal/receipts/export.csv") {
		t.Fatalf("main.go still registers GET fiscal/receipts/export.csv — the server-side receipts\n" +
			"export is deleted; the client exportLocalizedCsv path is the only export.")
	}
}

// scanForbidden walks root for .go files containing needle, skipping the gate
// file itself so the assertion string cannot self-match.
func scanForbidden(t *testing.T, root, needle, selfFile string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", "testdata", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, selfFile) {
			return nil
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer func() { _ = f.Close() }()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		rel, _ := filepath.Rel(root, path)
		for scanner.Scan() {
			lineNo++
			if line := scanner.Text(); strings.Contains(line, needle) {
				hits = append(hits,
					filepath.ToSlash(rel)+":"+strconv.Itoa(lineNo)+": "+strings.TrimSpace(line))
			}
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return hits
}

func handlersPackageRoot(t *testing.T) string {
	t.Helper()
	dir, _, ok := callerDir()
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return dir
}

func callerDir() (string, string, bool) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", "", false
	}
	return filepath.Dir(thisFile), thisFile, true
}
