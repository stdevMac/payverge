package server

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/guardrails"

	"github.com/stretchr/testify/assert"
)

type stubClassifier struct {
	verdict guardrails.Verdict
	err     error
}

func (s stubClassifier) Classify(_ context.Context, _ guardrails.ClassifyRequest) (guardrails.Verdict, error) {
	return s.verdict, s.err
}

func TestImagePromptGuardrail_BlocksAbuse(t *testing.T) {
	allowed, code := evaluateImagePromptGuardrail(
		stubClassifier{verdict: guardrails.Verdict{Allowed: false, Category: "abuse"}},
		context.Background(), 7, "en", "draw something illegal",
	)
	assert.False(t, allowed)
	assert.Equal(t, "abuse", code)
}

func TestImagePromptGuardrail_FailOpenOnError(t *testing.T) {
	allowed, _ := evaluateImagePromptGuardrail(
		stubClassifier{err: context.DeadlineExceeded},
		context.Background(), 7, "en", "anything",
	)
	assert.True(t, allowed, "classifier error must fail open (C2)")
}

func TestImagePromptGuardrail_SkipsEmptyPrompt(t *testing.T) {
	allowed, _ := evaluateImagePromptGuardrail(
		stubClassifier{verdict: guardrails.Verdict{Allowed: false}},
		context.Background(), 7, "en", "   ",
	)
	assert.True(t, allowed, "empty custom prompt skips classification")
}

func BenchmarkImagePromptGuardrail_AllowAll(b *testing.B) {
	c := guardrails.AllowAll{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = evaluateImagePromptGuardrail(c, context.Background(), 7, "en", "a photo of a margherita pizza")
	}
}
