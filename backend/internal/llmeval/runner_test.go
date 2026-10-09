package llmeval

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func TestRunCaseBuildsRequestAndGrades(t *testing.T) {
	p := &scriptedProvider{queue: []*llm.Response{{Text: "Welcome! Please confirm allergens with our staff.", Usage: llm.Usage{PromptTokens: 20, CompletionTokens: 8, TotalTokens: 28}}}}
	c := &Case{
		ID:      "waiter_disclaimer",
		Feature: "waiter",
		Request: CaseRequest{System: "You are a waiter.", User: "Is the soup nut free?", Locale: "en"},
		Assertions: []Assertion{
			{Type: "contains_all", Values: []string{"confirm", "staff"}},
			{Type: "language_is", Lang: "en"},
		},
	}
	cr := RunCase(context.Background(), Options{Provider: p, JudgeModel: "gemini"}, c)
	if !cr.Pass {
		t.Fatalf("case should pass: %+v", cr)
	}
	if cr.Usage.TotalTokens != 28 {
		t.Fatalf("usage = %+v", cr.Usage)
	}
	// The user message must reach the provider.
	if p.lastReq.System != "You are a waiter." {
		t.Fatalf("system not forwarded: %q", p.lastReq.System)
	}
	if len(p.lastReq.Messages) == 0 || p.lastReq.Messages[len(p.lastReq.Messages)-1].Text != "Is the soup nut free?" {
		t.Fatalf("user message not forwarded: %+v", p.lastReq.Messages)
	}
}

func TestRunCaseFailsOnAssertion(t *testing.T) {
	p := &scriptedProvider{queue: []*llm.Response{{Text: "Yes it is totally nut-free, enjoy!"}}}
	c := &Case{
		ID:         "bad",
		Request:    CaseRequest{System: "s", User: "is it nut free?"},
		Assertions: []Assertion{{Type: "contains_none", Values: []string{"nut-free"}}},
	}
	cr := RunCase(context.Background(), Options{Provider: p}, c)
	if cr.Pass {
		t.Fatalf("case should fail on forbidden phrase")
	}
}

func TestRunCaseRoutesJudge(t *testing.T) {
	// First call: the candidate answer. Second call: the judge verdict.
	p := &scriptedProvider{queue: []*llm.Response{
		{Text: "Welcome to our cafe!"},
		{Text: `{"pass":true,"reason":"warm greeting"}`},
	}}
	c := &Case{
		ID:         "judge",
		Request:    CaseRequest{System: "s", User: "greet me"},
		Assertions: []Assertion{{Type: "judge_rubric", Rubric: "Greeting is warm"}},
	}
	cr := RunCase(context.Background(), Options{Provider: p, JudgeModel: "gemini"}, c)
	if !cr.Pass {
		t.Fatalf("judge-routed case should pass: %+v", cr)
	}
	if p.calls != 2 {
		t.Fatalf("expected 2 provider calls (answer+judge), got %d", p.calls)
	}
}

func TestRunSuiteAggregates(t *testing.T) {
	p := &scriptedProvider{queue: []*llm.Response{
		{Text: "ok", Usage: llm.Usage{TotalTokens: 5}},
		{Text: "ok", Usage: llm.Usage{TotalTokens: 7}},
	}}
	cases := []*Case{
		{ID: "a", Request: CaseRequest{User: "x"}, Assertions: []Assertion{{Type: "contains_all", Values: []string{"ok"}}}},
		{ID: "b", Request: CaseRequest{User: "y"}, Assertions: []Assertion{{Type: "contains_all", Values: []string{"ok"}}}},
	}
	rep := RunSuite(context.Background(), Options{Provider: p}, "demo", cases)
	if rep.Passed != 2 || rep.Total != 2 {
		t.Fatalf("aggregate = %d/%d", rep.Passed, rep.Total)
	}
	if rep.Usage.TotalTokens != 12 {
		t.Fatalf("usage total = %d, want 12", rep.Usage.TotalTokens)
	}
}
