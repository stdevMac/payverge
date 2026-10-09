package database

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestPluginStructOmitsPrice keeps the GORM model aligned with the dropped column.
func TestPluginStructOmitsPrice(t *testing.T) {
	typ := reflect.TypeOf(Plugin{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Name == "Price" {
			t.Fatal("Plugin.Price must be deleted — plugins are free")
		}
		jsonName := strings.Split(f.Tag.Get("json"), ",")[0]
		if jsonName == "price" {
			t.Fatalf("Plugin field %s still has json:\"price\"", f.Name)
		}
		gormTag := f.Tag.Get("gorm")
		if strings.Contains(strings.ToLower(gormTag), "column:price") {
			t.Fatalf("Plugin field %s still maps to column price", f.Name)
		}
	}
}

// TestUpdatePluginForSyncDoesNotWritePrice is a source guard on the sync column
// list so registry upserts cannot reintroduce price after the column is gone.
func TestUpdatePluginForSyncDoesNotWritePrice(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "plugins.go"))
	if err != nil {
		t.Fatalf("read plugins.go: %v", err)
	}
	// Look for the Select cols slice used by UpdatePluginForSync.
	text := string(src)
	idx := strings.Index(text, "func UpdatePluginForSync")
	if idx < 0 {
		t.Fatal("UpdatePluginForSync not found")
	}
	// Limit to the function body window.
	window := text[idx:]
	if end := strings.Index(window[1:], "\nfunc "); end > 0 {
		window = window[:end+1]
	}
	if strings.Contains(window, `"price"`) {
		t.Fatal(`UpdatePluginForSync still lists "price" in its Select columns — remove it`)
	}
	if strings.Contains(window, "p.price") || strings.Contains(window, "p.Price") {
		t.Fatal("UpdatePluginForSync / related SQL still references p.price")
	}
}

// TestGetBusinessPluginsSQLOmitsPrice guards the raw SQL projection that feeds
// guest and operator plugin listings.
func TestGetBusinessPluginsSQLOmitsPrice(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "plugins.go"))
	if err != nil {
		t.Fatalf("read plugins.go: %v", err)
	}
	if strings.Contains(string(src), "p.price") {
		t.Fatal(`plugins.go still selects p.price — remove from GetBusinessPlugins projection`)
	}
}
