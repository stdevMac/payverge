package llmeval

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

func resp(text string) *llm.Response { return &llm.Response{Text: text} }

func TestGradeRegexMatch(t *testing.T) {
	r := Grade(Assertion{Type: "regex_match", Pattern: `\d{4}-\d{2}-\d{2}`}, resp("today is 2026-06-06 ok"))
	if !r.Pass {
		t.Fatalf("regex_match should pass: %+v", r)
	}
	r = Grade(Assertion{Type: "regex_match", Pattern: `\d{4}`}, resp("no digits here"))
	if r.Pass {
		t.Fatalf("regex_match should fail")
	}
}

func TestGradeRegexAbsent(t *testing.T) {
	r := Grade(Assertion{Type: "regex_absent", Pattern: `(?i)blockchain`}, resp("welcome to our cafe"))
	if !r.Pass {
		t.Fatalf("regex_absent should pass when token absent: %+v", r)
	}
	r = Grade(Assertion{Type: "regex_absent", Pattern: `(?i)blockchain`}, resp("pay via Blockchain"))
	if r.Pass {
		t.Fatalf("regex_absent should fail when token present")
	}
}

func TestGradeContains(t *testing.T) {
	r := Grade(Assertion{Type: "contains_all", Values: []string{"confirm", "staff"}}, resp("please confirm with our staff"))
	if !r.Pass {
		t.Fatalf("contains_all should pass: %+v", r)
	}
	r = Grade(Assertion{Type: "contains_all", Values: []string{"confirm", "manager"}}, resp("please confirm with our staff"))
	if r.Pass {
		t.Fatalf("contains_all should fail when one missing")
	}
	r = Grade(Assertion{Type: "contains_none", Values: []string{"nut-free", "safe"}}, resp("this dish contains peanuts"))
	if !r.Pass {
		t.Fatalf("contains_none should pass: %+v", r)
	}
	r = Grade(Assertion{Type: "contains_none", Values: []string{"peanuts"}}, resp("this dish contains peanuts"))
	if r.Pass {
		t.Fatalf("contains_none should fail when present")
	}
}

func TestGradeLanguageIs(t *testing.T) {
	es := "Bienvenido a nuestro restaurante, ¿en qué puedo ayudarte con tu pedido?"
	r := Grade(Assertion{Type: "language_is", Lang: "es"}, resp(es))
	if !r.Pass {
		t.Fatalf("language_is es should pass: %+v", r)
	}
	r = Grade(Assertion{Type: "language_is", Lang: "en"}, resp(es))
	if r.Pass {
		t.Fatalf("language_is en should fail on spanish text")
	}
}

func TestGradeJSONSchemaValid(t *testing.T) {
	schema := `{"type":"object","required":["summary"],"properties":{"summary":{"type":"string"}}}`
	r := Grade(Assertion{Type: "json_schema_valid", Schema: schema}, resp(`{"summary":"sales up 12%"}`))
	if !r.Pass {
		t.Fatalf("json_schema_valid should pass: %+v", r)
	}
	r = Grade(Assertion{Type: "json_schema_valid", Schema: schema}, resp(`{"notsummary":"x"}`))
	if r.Pass {
		t.Fatalf("json_schema_valid should fail (missing required)")
	}
	r = Grade(Assertion{Type: "json_schema_valid", Schema: schema}, resp(`not json`))
	if r.Pass {
		t.Fatalf("json_schema_valid should fail (not json)")
	}
}

func TestGradeJSONFieldEquals(t *testing.T) {
	r := Grade(Assertion{Type: "json_field_equals", Field: "priority", Equals: "high"}, resp(`{"priority":"high"}`))
	if !r.Pass {
		t.Fatalf("json_field_equals nested should pass: %+v", r)
	}
	r = Grade(Assertion{Type: "json_field_equals", Field: "actions.0.priority", Equals: "low"}, resp(`{"actions":[{"priority":"low"}]}`))
	if !r.Pass {
		t.Fatalf("json_field_equals dotted path should pass: %+v", r)
	}
	r = Grade(Assertion{Type: "json_field_equals", Field: "priority", Equals: "low"}, resp(`{"priority":"high"}`))
	if r.Pass {
		t.Fatalf("json_field_equals should fail on mismatch")
	}
}

func TestGradeJudgeIsDeferred(t *testing.T) {
	r := Grade(Assertion{Type: "judge_rubric", Rubric: "is it polite"}, resp("hi"))
	if r.Pass || !r.NeedsJudge {
		t.Fatalf("judge_rubric must defer to the judge: %+v", r)
	}
}

func TestGradeUnknownType(t *testing.T) {
	r := Grade(Assertion{Type: "bogus"}, resp("x"))
	if r.Pass {
		t.Fatalf("unknown assertion type must fail closed")
	}
}
