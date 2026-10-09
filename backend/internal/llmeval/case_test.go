package llmeval

import (
	"path/filepath"
	"testing"
)

func TestLoadCaseYAML(t *testing.T) {
	yaml := `
id: waiter_greeting_en
feature: waiter
request:
  system: "You are a waiter."
  user: "Hi"
  locale: en
assertions:
  - type: contains_all
    values: ["welcome"]
  - type: language_is
    lang: en
`
	dir := t.TempDir()
	path := filepath.Join(dir, "case.yaml")
	if err := writeFile(t, path, yaml); err != nil {
		t.Fatalf("write: %v", err)
	}
	c, err := LoadCase(path)
	if err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	if c.ID != "waiter_greeting_en" || c.Feature != "waiter" {
		t.Fatalf("id/feature = %q/%q", c.ID, c.Feature)
	}
	if c.Request.System != "You are a waiter." || c.Request.User != "Hi" || c.Request.Locale != "en" {
		t.Fatalf("request = %+v", c.Request)
	}
	if len(c.Assertions) != 2 || c.Assertions[0].Type != "contains_all" || c.Assertions[1].Type != "language_is" {
		t.Fatalf("assertions = %+v", c.Assertions)
	}
	if c.Assertions[1].Lang != "en" {
		t.Fatalf("lang = %q", c.Assertions[1].Lang)
	}
}

func TestLoadCaseJSON(t *testing.T) {
	js := `{"id":"x","feature":"director","request":{"system":"s","user":"u"},"assertions":[{"type":"regex_match","pattern":"\\d+"}]}`
	dir := t.TempDir()
	path := filepath.Join(dir, "case.json")
	if err := writeFile(t, path, js); err != nil {
		t.Fatalf("write: %v", err)
	}
	c, err := LoadCase(path)
	if err != nil {
		t.Fatalf("LoadCase: %v", err)
	}
	if c.Assertions[0].Pattern != `\d+` {
		t.Fatalf("pattern = %q", c.Assertions[0].Pattern)
	}
}

func TestLoadCaseMissingID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	_ = writeFile(t, path, "feature: waiter\n")
	if _, err := LoadCase(path); err == nil {
		t.Fatalf("expected error for missing id")
	}
}

// writeFile is a tiny test helper.
func writeFile(t *testing.T, path, content string) error {
	t.Helper()
	return osWriteFile(path, []byte(content), 0o644)
}
