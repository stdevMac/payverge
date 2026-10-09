package llmeval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

// judgeSystem instructs the judge to be a strict binary grader.
const judgeSystem = `You are a strict evaluation judge. You are given a RUBRIC and a CANDIDATE answer produced by another assistant. Decide whether the CANDIDATE satisfies the RUBRIC. Respond with JSON only: {"pass": true|false, "reason": "<one short sentence>"}. Be conservative: if the candidate clearly violates the rubric, fail it.`

// judgeVerdict is the strict JSON the judge returns.
type judgeVerdict struct {
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

// judgeSchema is the strict ResponseSchema enforced on the judge call.
var judgeSchema = &llm.JSONSchema{
	Type: llm.TypeObject,
	Properties: map[string]*llm.JSONSchema{
		"pass":   {Type: llm.TypeBoolean},
		"reason": {Type: llm.TypeString},
	},
	Required: []string{"pass", "reason"},
}

// RunJudge evaluates a judge_rubric assertion via a second provider call with a
// strict JSON verdict schema. judgeModel selects which model grades; on a
// malformed/non-JSON verdict it returns an error (the runner records it as a
// failed assertion, not a silent pass).
func RunJudge(ctx context.Context, p llm.Provider, judgeModel string, a Assertion, candidate *llm.Response) (Result, error) {
	candidateText := ""
	if candidate != nil {
		candidateText = candidate.Text
	}
	user := fmt.Sprintf("RUBRIC:\n%s\n\nCANDIDATE:\n%s", a.Rubric, candidateText)
	resp, err := p.Generate(ctx, llm.GenerateRequest{
		Model:          judgeModel,
		System:         judgeSystem,
		Messages:       []llm.Message{{Role: llm.RoleUser, Text: user}},
		ResponseSchema: judgeSchema,
	})
	if err != nil {
		return Result{Type: a.Type}, fmt.Errorf("llmeval: judge call failed: %w", err)
	}
	var v judgeVerdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Text)), &v); err != nil {
		return Result{Type: a.Type}, fmt.Errorf("llmeval: judge returned non-JSON verdict %q: %w", resp.Text, err)
	}
	return Result{Type: a.Type, Pass: v.Pass, Detail: "judge: " + v.Reason}, nil
}
