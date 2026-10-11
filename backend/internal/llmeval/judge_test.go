package llmeval

import (
	"context"
	"errors"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

// scriptedProvider returns queued responses in order and records the last
// request it saw (to assert the judge enforces a strict schema).
type scriptedProvider struct {
	queue   []*llm.Response
	errs    []error
	lastReq llm.GenerateRequest
	calls   int
}

func (s *scriptedProvider) Generate(_ context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	s.lastReq = req
	i := s.calls
	s.calls++
	var err error
	if i < len(s.errs) {
		err = s.errs[i]
	}
	if i < len(s.queue) {
		return s.queue[i], err
	}
	return &llm.Response{}, err
}

func TestRunJudgePass(t *testing.T) {
	p := &scriptedProvider{queue: []*llm.Response{{Text: `{"pass":true,"reason":"polite and grounded"}`}}}
	res, err := RunJudge(context.Background(), p, "gemini", Assertion{Type: "judge_rubric", Rubric: "Answer is polite"}, &llm.Response{Text: "Welcome! Happy to help."})
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if !res.Pass {
		t.Fatalf("expected pass, got %+v", res)
	}
	// The judge call MUST set a strict response schema.
	if p.lastReq.ResponseSchema == nil || p.lastReq.ResponseSchema.Type != llm.TypeObject {
		t.Fatalf("judge must use a strict object ResponseSchema, got %+v", p.lastReq.ResponseSchema)
	}
	if p.lastReq.Model != "gemini" {
		t.Fatalf("judge model = %q, want gemini", p.lastReq.Model)
	}
}

func TestRunJudgeFail(t *testing.T) {
	p := &scriptedProvider{queue: []*llm.Response{{Text: `{"pass":false,"reason":"rude tone"}`}}}
	res, err := RunJudge(context.Background(), p, "gemini", Assertion{Type: "judge_rubric", Rubric: "Answer is polite"}, &llm.Response{Text: "go away"})
	if err != nil {
		t.Fatalf("RunJudge: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected fail, got %+v", res)
	}
}

func TestRunJudgeMalformed(t *testing.T) {
	p := &scriptedProvider{queue: []*llm.Response{{Text: "not json"}}}
	_, err := RunJudge(context.Background(), p, "gemini", Assertion{Type: "judge_rubric", Rubric: "x"}, &llm.Response{Text: "y"})
	if err == nil {
		t.Fatalf("expected error on malformed verdict")
	}
}

func TestRunJudgeProviderError(t *testing.T) {
	p := &scriptedProvider{errs: []error{errors.New("boom")}}
	_, err := RunJudge(context.Background(), p, "gemini", Assertion{Type: "judge_rubric", Rubric: "x"}, &llm.Response{Text: "y"})
	if err == nil {
		t.Fatalf("expected provider error to surface")
	}
}
