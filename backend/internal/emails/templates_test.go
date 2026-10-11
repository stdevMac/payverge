package emails

import (
	"os"
	"path/filepath"
	"testing"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"
)

func TestTemplateManagerDebugUsesProductionOverride(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "development")
	appconfig.SetProductionModeOverride(true)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })

	templatesDir := filepath.Join(t.TempDir(), "templates")
	layoutDir := filepath.Join(templatesDir, "..", "layout")
	if err := os.MkdirAll(filepath.Join(templatesDir, "eng"), 0o755); err != nil {
		t.Fatalf("create templates dir: %v", err)
	}
	if err := os.MkdirAll(layoutDir, 0o755); err != nil {
		t.Fatalf("create layout dir: %v", err)
	}
	for _, lang := range templateLanguageFamilies() {
		if err := os.WriteFile(filepath.Join(layoutDir, "base_"+lang+".html"), []byte("{{block \"content\" .}}{{end}}"), 0o644); err != nil {
			t.Fatalf("write layout: %v", err)
		}
	}

	tm, err := NewTemplateManager(templatesDir)
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}

	if tm.debug {
		t.Fatal("expected debug reload to be disabled when production override is enabled")
	}
}
