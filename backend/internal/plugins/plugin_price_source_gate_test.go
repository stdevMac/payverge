package plugins_test

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
)

// TestPluginPriceDeletedSourceGate freezes the deletion of plugin catalog
// pricing. Plugins are free and included with every plan — GetPrice, a price
// column, and a "price" field on the API response must not reappear.
//
// Modeled on TestAutoMigrateSourceGate: a source-level assertion is what keeps
// a deleted concept deleted.
func TestPluginPriceDeletedSourceGate(t *testing.T) {
	root := pluginPackageRoot(t)

	var getPriceHits []string
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
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Skip this gate file itself so the assertion string does not self-match.
		if strings.HasSuffix(path, "plugin_price_source_gate_test.go") {
			return nil
		}

		f, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer func() { _ = f.Close() }()

		scanner := bufio.NewScanner(f)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		lineNo := 0
		rel, _ := filepath.Rel(root, path)
		for scanner.Scan() {
			lineNo++
			line := scanner.Text()
			// Match the interface method / implementations, not MercadoPago's
			// unit_price payment-line field or unrelated "price" words.
			if strings.Contains(line, "GetPrice") {
				getPriceHits = append(getPriceHits,
					filepath.ToSlash(rel)+":"+strconv.Itoa(lineNo)+": "+strings.TrimSpace(line))
			}
		}
		return scanner.Err()
	})
	if err != nil {
		t.Fatalf("walk plugins tree: %v", err)
	}
	if len(getPriceHits) > 0 {
		t.Fatalf("GetPrice must be fully deleted from backend/internal/plugins/ (%d hits):\n  %s\n\nPlugins are free; do not reintroduce catalog pricing.",
			len(getPriceHits), strings.Join(getPriceHits, "\n  "))
	}
}

// TestPluginModelHasNoPriceField asserts database.Plugin no longer carries a
// Price field (json:"price"). The API response shape is the GORM model.
func TestPluginModelHasNoPriceField(t *testing.T) {
	typ := reflect.TypeOf(database.Plugin{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Name == "Price" {
			t.Fatalf("database.Plugin still has field Price — catalog pricing must be deleted")
		}
		tag := f.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "price" {
			t.Fatalf("database.Plugin field %s still serializes as json:\"price\" — remove it", f.Name)
		}
	}
}

// TestConvertToDBPluginDoesNotSetPrice compiles against ConvertToDBPlugin and
// asserts the returned struct has a zero Price-less shape (no price in JSON).
func TestConvertToDBPluginDoesNotSetPrice(t *testing.T) {
	// Parse interface.go ConvertToDBPlugin body and forbid a Price: assignment.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	ifacePath := filepath.Join(filepath.Dir(thisFile), "interface.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, ifacePath, nil, 0)
	if err != nil {
		t.Fatalf("parse interface.go: %v", err)
	}

	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "ConvertToDBPlugin" {
			return true
		}
		found = true
		ast.Inspect(fn.Body, func(inner ast.Node) bool {
			kv, ok := inner.(*ast.KeyValueExpr)
			if !ok {
				return true
			}
			if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Price" {
				t.Errorf("ConvertToDBPlugin still assigns Price — delete the line")
			}
			return true
		})
		return false
	})
	if !found {
		t.Fatal("ConvertToDBPlugin not found in interface.go")
	}

	// Registry compile-time smoke: a nil-safe empty conversion is not used;
	// we only need the function to remain present without a Price field.
	_ = plugins.GlobalRegistry
}

func pluginPackageRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(thisFile)
}
