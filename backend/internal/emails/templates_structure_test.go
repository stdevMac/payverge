package emails

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestEmailTemplatesParityAndStructure(t *testing.T) {
	templatesRoot := resolveTemplatesRoot(t)
	engDir := filepath.Join(templatesRoot, "eng")

	engFiles := listTemplateFiles(t, engDir)
	if len(engFiles) == 0 {
		t.Fatalf("no english templates found in %s", engDir)
	}

	engNames := make([]string, 0, len(engFiles))
	for name := range engFiles {
		engNames = append(engNames, name)
	}
	sort.Strings(engNames)

	for _, lang := range templateLanguageFamilies() {
		langDir := filepath.Join(templatesRoot, lang)
		langFiles := listTemplateFiles(t, langDir)
		if len(langFiles) == 0 {
			t.Fatalf("no %s templates found in %s", lang, langDir)
		}

		langNames := make([]string, 0, len(langFiles))
		for name := range langFiles {
			langNames = append(langNames, name)
		}
		sort.Strings(langNames)

		if lang != "eng" && strings.Join(engNames, ",") != strings.Join(langNames, ",") {
			t.Fatalf("template mismatch between eng and %s.\neng: %v\n%s: %v", lang, engNames, lang, langNames)
		}

		for _, filename := range langNames {
			content := readTemplate(t, filepath.Join(langDir, filename))
			assertTemplateStructure(t, lang+"/"+filename, content)
		}
	}
}

func TestManagedEmailTemplatesMatchManifest(t *testing.T) {
	templatesRoot := resolveTemplatesRoot(t)
	engDir := filepath.Join(templatesRoot, "eng")

	files := listTemplateFiles(t, engDir)
	actual := make([]string, 0, len(files))
	for filename := range files {
		actual = append(actual, strings.TrimSuffix(filename, filepath.Ext(filename)))
	}
	sort.Strings(actual)

	expected := managedTemplateNames()
	if strings.Join(actual, ",") != strings.Join(expected, ",") {
		t.Fatalf("managed template manifest mismatch.\nexpected: %v\nactual: %v", expected, actual)
	}
}

func TestAllTemplatesRenderWithTemplateManager(t *testing.T) {
	templatesRoot := resolveTemplatesRoot(t)

	tm, err := NewTemplateManager(templatesRoot)
	if err != nil {
		t.Fatalf("failed to initialize template manager: %v", err)
	}

	for _, lang := range templateLanguageFamilies() {
		files := listTemplateFiles(t, filepath.Join(templatesRoot, lang))
		names := make([]string, 0, len(files))
		for filename := range files {
			names = append(names, filename)
		}
		sort.Strings(names)

		for _, filename := range names {
			templateName := strings.TrimSuffix(filename, filepath.Ext(filename))
			htmlBody, renderErr := tm.Render(lang, templateName, map[string]interface{}{
				"owner_name":      "Test Owner",
				"dashboard_url":   "https://payverge.io/business/1/dashboard?tab=overview",
				"unsubscribe_url": "https://payverge.io/account",
			})
			if renderErr != nil {
				t.Fatalf("failed to render %s/%s: %v", lang, templateName, renderErr)
			}
			if !strings.Contains(strings.ToLower(htmlBody), "<html") {
				t.Fatalf("rendered output for %s/%s does not look like html", lang, templateName)
			}
		}
	}
}

func TestTemplateManagerRenderNormalizesLanguageCodes(t *testing.T) {
	templatesRoot := resolveTemplatesRoot(t)

	tm, err := NewTemplateManager(templatesRoot)
	if err != nil {
		t.Fatalf("failed to initialize template manager: %v", err)
	}

	testData := map[string]interface{}{
		"owner_name":    "Test Owner",
		"dashboard_url": "https://payverge.io/business/test/dashboard",
	}

	englishBody, err := tm.Render("en-US", "welcome", testData)
	if err != nil {
		t.Fatalf("failed to render english alias: %v", err)
	}
	if !strings.Contains(englishBody, "Thanks for joining Payverge") {
		t.Fatalf("expected english alias to render english template, got %q", englishBody)
	}

	if normalized := normalizeTemplateLanguage("es-AR"); normalized != "es_ar" {
		t.Fatalf("expected es-AR to normalize to es_ar, got %q", normalized)
	}
	if normalized := normalizeTemplateLanguage("es_ar"); normalized != "es_ar" {
		t.Fatalf("expected es_ar family to normalize to es_ar, got %q", normalized)
	}

	spanishBody, err := tm.Render("es-AR", "welcome", testData)
	if err != nil {
		t.Fatalf("failed to render spanish alias: %v", err)
	}
	if !strings.Contains(spanishBody, "Gracias por sumarte a Payverge") {
		t.Fatalf("expected spanish alias to render spanish template, got %q", spanishBody)
	}
}

func resolveTemplatesRoot(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve current file path")
	}

	templatesRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "email", "templates")
	if _, err := os.Stat(templatesRoot); err != nil {
		t.Fatalf("templates root does not exist: %s (%v)", templatesRoot, err)
	}
	return templatesRoot
}

func listTemplateFiles(t *testing.T, dir string) map[string]struct{} {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir %s: %v", dir, err)
	}

	result := make(map[string]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".html" {
			continue
		}
		result[name] = struct{}{}
	}
	return result
}

func readTemplate(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(content)
}

func assertTemplateStructure(t *testing.T, name, content string) {
	t.Helper()

	if !strings.Contains(content, `{{ define "content" }}`) {
		t.Fatalf("template %s is missing {{ define \"content\" }}", name)
	}
	if !strings.Contains(content, `{{ define "subject" }}`) {
		t.Fatalf("template %s is missing {{ define \"subject\" }}", name)
	}
	if strings.Contains(content, "<title>") {
		t.Fatalf("template %s must not contain an in-body <title> (subject lives in the {{ define \"subject\" }} block)", name)
	}
}

// Email layouts must not pull stylesheets or fonts from third-party hosts:
// every open would tell that host who read which instance's mail. The font
// stacks fall back to system fonts.
func TestEmailLayoutsLoadNoRemoteStylesheets(t *testing.T) {
	layoutDir := filepath.Join(resolveTemplatesRoot(t), "..", "layout")
	entries, err := os.ReadDir(layoutDir)
	if err != nil {
		t.Fatalf("read %s: %v", layoutDir, err)
	}
	for _, entry := range entries {
		body, err := os.ReadFile(filepath.Join(layoutDir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{"fonts.googleapis.com", "fonts.gstatic.com", `rel="stylesheet"`, "@import"} {
			if strings.Contains(string(body), banned) {
				t.Errorf("%s loads a remote stylesheet (%s)", entry.Name(), banned)
			}
		}
	}
}
