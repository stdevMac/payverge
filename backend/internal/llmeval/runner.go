package llmeval

import (
	"context"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

// Options configures a run.
type Options struct {
	Provider   llm.Provider
	JudgeModel string // model used for judge_rubric assertions (live runs only)
	// DefaultModel is used when a Case does not pin its own Request.Model.
	DefaultModel string
}

// CaseResult is the outcome of one case.
type CaseResult struct {
	ID         string
	Feature    string
	Pass       bool
	Assertions []Result
	Usage      llm.Usage
	Err        string // provider/generate error, if any
}

// SuiteReport aggregates a run.
type SuiteReport struct {
	Suite  string
	Total  int
	Passed int
	Cases  []CaseResult
	Usage  llm.Usage
}

// BuildRequest maps a Case into an llm.GenerateRequest.
func BuildRequest(opts Options, c *Case) llm.GenerateRequest { return buildRequest(opts, c) }

// buildRequest maps a Case into an llm.GenerateRequest.
func buildRequest(opts Options, c *Case) llm.GenerateRequest {
	model := c.Request.Model
	if model == "" {
		model = opts.DefaultModel
	}
	msgs := make([]llm.Message, 0, len(c.Request.History)+1)
	for _, t := range c.Request.History {
		role := llm.RoleUser
		if t.Role == "assistant" {
			role = llm.RoleAssistant
		}
		msgs = append(msgs, llm.Message{Role: role, Text: t.Text})
	}
	if c.Request.User != "" {
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Text: c.Request.User})
	}
	return llm.GenerateRequest{Model: model, System: c.Request.System, Messages: msgs}
}

// RunCase executes one case and grades all assertions.
func RunCase(ctx context.Context, opts Options, c *Case) CaseResult {
	cr := CaseResult{ID: c.ID, Feature: c.Feature}
	resp, err := opts.Provider.Generate(ctx, buildRequest(opts, c))
	if err != nil {
		cr.Err = err.Error()
		cr.Pass = false
		return cr
	}
	cr.Usage = resp.Usage
	cr.Pass = true
	for _, a := range c.Assertions {
		res := Grade(a, resp)
		if res.NeedsJudge {
			jres, jerr := RunJudge(ctx, opts.Provider, opts.JudgeModel, a, resp)
			if jerr != nil {
				res = Result{Type: a.Type, Pass: false, Detail: jerr.Error()}
			} else {
				res = jres
			}
		}
		cr.Assertions = append(cr.Assertions, res)
		if !res.Pass {
			cr.Pass = false
		}
	}
	return cr
}

// RunSuite executes all cases and aggregates results + token usage.
func RunSuite(ctx context.Context, opts Options, suite string, cases []*Case) SuiteReport {
	rep := SuiteReport{Suite: suite, Total: len(cases)}
	for _, c := range cases {
		cr := RunCase(ctx, opts, c)
		if cr.Pass {
			rep.Passed++
		}
		rep.Usage.PromptTokens += cr.Usage.PromptTokens
		rep.Usage.CompletionTokens += cr.Usage.CompletionTokens
		rep.Usage.TotalTokens += cr.Usage.TotalTokens
		rep.Cases = append(rep.Cases, cr)
	}
	return rep
}
