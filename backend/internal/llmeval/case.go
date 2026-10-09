package llmeval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// CaseRequest is the model input for one eval case. Either a recorded request
// (System/User/History) or a pointer to a prompt asset — for H0 we use the
// direct System/User shape; suites that need richer history can extend History.
type CaseRequest struct {
	System  string `json:"system" yaml:"system"`
	User    string `json:"user" yaml:"user"`
	History []Turn `json:"history,omitempty" yaml:"history,omitempty"`
	Locale  string `json:"locale,omitempty" yaml:"locale,omitempty"`
	Model   string `json:"model,omitempty" yaml:"model,omitempty"`
}

// Turn is one prior conversation message in a case request.
type Turn struct {
	Role string `json:"role" yaml:"role"` // "user" | "assistant"
	Text string `json:"text" yaml:"text"`
}

// Assertion is one deterministic (or judge) check against the model output.
type Assertion struct {
	Type string `json:"type" yaml:"type"`
	// regex_match / regex_absent
	Pattern string `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	// json_field_equals
	Field  string `json:"field,omitempty" yaml:"field,omitempty"`
	Equals string `json:"equals,omitempty" yaml:"equals,omitempty"`
	// language_is
	Lang string `json:"lang,omitempty" yaml:"lang,omitempty"`
	// contains_all / contains_none
	Values []string `json:"values,omitempty" yaml:"values,omitempty"`
	// json_schema_valid (inline JSON Schema as raw JSON)
	Schema string `json:"schema,omitempty" yaml:"schema,omitempty"`
	// judge_rubric
	Rubric string `json:"rubric,omitempty" yaml:"rubric,omitempty"`
}

// Case is one eval scenario.
type Case struct {
	ID         string      `json:"id" yaml:"id"`
	Feature    string      `json:"feature" yaml:"feature"`
	Request    CaseRequest `json:"request" yaml:"request"`
	Assertions []Assertion `json:"assertions" yaml:"assertions"`
}

// osWriteFile is an indirection so tests share one write helper.
var osWriteFile = os.WriteFile

// LoadCase reads a single case from a .yaml/.yml or .json file.
func LoadCase(path string) (*Case, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("llmeval: read case %s: %w", path, err)
	}
	var c Case
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("llmeval: parse json case %s: %w", path, err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(raw, &c); err != nil {
			return nil, fmt.Errorf("llmeval: parse yaml case %s: %w", path, err)
		}
	default:
		return nil, fmt.Errorf("llmeval: unsupported case extension %q", filepath.Ext(path))
	}
	if strings.TrimSpace(c.ID) == "" {
		return nil, fmt.Errorf("llmeval: case %s missing id", path)
	}
	return &c, nil
}
