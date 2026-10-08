package llmeval

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func TestFixtureKeyStable(t *testing.T) {
	req := llm.GenerateRequest{
		Model:    "google/gemini-2.0-flash-001",
		System:   "you are a waiter",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "hi"}},
	}
	k1 := FixtureKey(req)
	k2 := FixtureKey(req)
	if k1 != k2 || len(k1) != 16 {
		t.Fatalf("key not stable/16-hex: %q vs %q", k1, k2)
	}
	req.Messages[0].Text = "hello"
	if FixtureKey(req) == k1 {
		t.Fatalf("key should change when message text changes")
	}
}

func TestFixtureProviderReplay(t *testing.T) {
	dir := t.TempDir()
	req := llm.GenerateRequest{
		Model:    "google/gemini-2.0-flash-001",
		System:   "sys",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "ask"}},
	}
	want := &llm.Response{Text: "canned answer", Model: "google/gemini-2.0-flash-001", Usage: llm.Usage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14}}
	if err := WriteFixture(dir, req, want); err != nil {
		t.Fatalf("WriteFixture: %v", err)
	}
	p, err := NewFixtureProvider(dir)
	if err != nil {
		t.Fatalf("NewFixtureProvider: %v", err)
	}
	got, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.Text != "canned answer" || got.Usage.TotalTokens != 14 {
		t.Fatalf("replay = %+v", got)
	}
}

func TestFixtureProviderMiss(t *testing.T) {
	dir := t.TempDir()
	// Write one fixture, then ask for a different request.
	_ = WriteFixture(dir, llm.GenerateRequest{Model: "m", Messages: []llm.Message{{Role: llm.RoleUser, Text: "a"}}}, &llm.Response{Text: "x"})
	p, _ := NewFixtureProvider(dir)
	_, err := p.Generate(context.Background(), llm.GenerateRequest{Model: "m", Messages: []llm.Message{{Role: llm.RoleUser, Text: "b"}}})
	if err == nil {
		t.Fatalf("expected a fixture-miss error")
	}
}

func TestFixtureFileName(t *testing.T) {
	// WriteFixture must place a .json file named <key>.json in dir.
	dir := t.TempDir()
	req := llm.GenerateRequest{Model: "m", Messages: []llm.Message{{Role: llm.RoleUser, Text: "z"}}}
	if err := WriteFixture(dir, req, &llm.Response{Text: "y"}); err != nil {
		t.Fatalf("WriteFixture: %v", err)
	}
	want := filepath.Join(dir, FixtureKey(req)+".json")
	if _, err := osStat(want); err != nil {
		t.Fatalf("expected fixture file %s: %v", want, err)
	}
}
