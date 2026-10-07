package emails

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	texttemplate "text/template"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/locales"
)

// TemplateManager handles loading and rendering of email templates
type TemplateManager struct {
	templatesDir string
	templates    map[string]*htmltemplate.Template
	// subjects are parsed with text/template so inbox headers are not
	// HTML-escaped (Joe's Bar & Grill must not become Joe&#39;s Bar &amp; Grill).
	subjects map[string]*texttemplate.Template
	mu       sync.RWMutex
	debug    bool
}

// templateLanguageFamilies returns the distinct email template families that
// must ship, derived from the locale registry's "emails" surface contract (the
// single source of truth) rather than a hardcoded list. Today this is
// eng/es/es_ar; promoting a locale to an email locale in registry.json makes its
// family appear here automatically (and TestEmailFamiliesMatchRegistryContract
// then requires its templates + base layout to exist).
func templateLanguageFamilies() []string {
	seen := make(map[string]struct{}, 3)
	families := make([]string, 0, 3)
	for _, loc := range locales.AllLocales() {
		if !locales.IsEmailLocale(loc.Canonical) {
			continue
		}
		if _, ok := seen[loc.EmailFamily]; ok {
			continue
		}
		seen[loc.EmailFamily] = struct{}{}
		families = append(families, loc.EmailFamily)
	}
	sort.Strings(families)
	return families
}

// NewTemplateManager creates a new template manager
func NewTemplateManager(templatesDir string) (*TemplateManager, error) {
	tm := &TemplateManager{
		templatesDir: templatesDir,
		templates:    make(map[string]*htmltemplate.Template),
		subjects:     make(map[string]*texttemplate.Template),
		debug:        !appconfig.IsProductionMode(false),
	}

	if err := tm.loadTemplates(); err != nil {
		return nil, err
	}

	return tm, nil
}

func (tm *TemplateManager) loadTemplates() error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.templates = make(map[string]*htmltemplate.Template)
	tm.subjects = make(map[string]*texttemplate.Template)

	// Supported languages
	for _, lang := range templateLanguageFamilies() {
		// Load base layout for the language
		// templatesDir is like "backend/email/templates"
		// layout is in "backend/email/layout/base_{lang}.html"
		layoutPath := filepath.Join(tm.templatesDir, "..", "layout", fmt.Sprintf("base_%s.html", lang))
		layoutContent, err := os.ReadFile(layoutPath)
		if err != nil {
			return fmt.Errorf("failed to load layout %s: %w", layoutPath, err)
		}

		baseTmpl, err := htmltemplate.New("base").Parse(string(layoutContent))
		if err != nil {
			return fmt.Errorf("failed to parse layout %s: %w", layoutPath, err)
		}

		langDir := filepath.Join(tm.templatesDir, lang)
		entries, err := os.ReadDir(langDir)
		if err != nil {
			if os.IsNotExist(err) {
				if lang == "eng" {
					return fmt.Errorf("english templates directory missing: %w", err)
				}
				continue
			}
			return err
		}

		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".html" {
				continue
			}

			path := filepath.Join(langDir, entry.Name())
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			// Key format: "lang/template_name" (e.g., "eng/welcome")
			name := entry.Name()
			nameWithoutExt := name[:len(name)-len(filepath.Ext(name))]
			key := fmt.Sprintf("%s/%s", lang, nameWithoutExt)

			// Clone base template
			tmpl, err := baseTmpl.Clone()
			if err != nil {
				return err
			}

			// Parse content into the clone
			// The content file MUST define a "content" block: {{ define "content" }}...{{ end }}
			_, err = tmpl.Parse(string(content))
			if err != nil {
				return fmt.Errorf("failed to parse template %s: %w", path, err)
			}

			tm.templates[key] = tmpl

			// Subject lines are MIME headers, not HTML. Parse the same file
			// with text/template so apostrophes and ampersands stay literal.
			textSet, err := texttemplate.New(key).Parse(string(content))
			if err != nil {
				return fmt.Errorf("failed to parse subject template %s: %w", path, err)
			}
			if sub := textSet.Lookup("subject"); sub != nil {
				tm.subjects[key] = sub
			}
		}
	}

	return nil
}

// Render renders a template with the given data
func (tm *TemplateManager) Render(lang, templateName string, data map[string]interface{}) (string, error) {
	// In debug mode, reload templates on every render to allow hot-swapping
	if tm.debug {
		if err := tm.loadTemplates(); err != nil {
			return "", err
		}
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	resolvedLang := normalizeTemplateLanguage(lang)

	// Try requested language
	key := fmt.Sprintf("%s/%s", resolvedLang, templateName)
	tmpl, ok := tm.templates[key]
	if !ok {
		// Fallback to English if requested language not found
		if resolvedLang != "eng" {
			fallbackKey := fmt.Sprintf("eng/%s", templateName)
			if fallbackTmpl, ok := tm.templates[fallbackKey]; ok {
				tmpl = fallbackTmpl
			} else {
				return "", fmt.Errorf("template not found: %s (and no fallback)", key)
			}
		} else {
			return "", fmt.Errorf("template not found: %s", key)
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// renderSubject executes a template's "subject" block in isolation, returning
// the trimmed subject line. Mirrors Render's language resolution (with eng
// fallback). Returns "" if the template has no "subject" block or it renders
// empty — callers substitute a default.
func (tm *TemplateManager) renderSubject(lang, templateName string, data map[string]interface{}) (string, error) {
	if tm.debug {
		if err := tm.loadTemplates(); err != nil {
			return "", err
		}
	}

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	resolvedLang := normalizeTemplateLanguage(lang)
	key := fmt.Sprintf("%s/%s", resolvedLang, templateName)
	_, ok := tm.templates[key]
	if !ok && resolvedLang != "eng" {
		key = fmt.Sprintf("eng/%s", templateName)
		_, ok = tm.templates[key]
	}
	if !ok {
		return "", fmt.Errorf("template not found: %s", key)
	}

	sub := tm.subjects[key]
	if sub == nil {
		return "", nil
	}

	var buf bytes.Buffer
	if err := sub.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func normalizeTemplateLanguage(lang string) string {
	normalized := strings.ToLower(strings.TrimSpace(lang))
	for _, family := range templateLanguageFamilies() {
		if normalized == family {
			return family
		}
	}

	return locales.EmailFamily(lang)
}
