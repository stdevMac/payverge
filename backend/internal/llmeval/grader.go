package llmeval

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/llmeval/langid"
)

// langidMinConfidence is the floor below which a language_is assertion is
// treated as "insufficient evidence" and FAILS (so an ambiguous answer never
// silently passes a locale-adherence suite).
const langidMinConfidence = 0.34

// Result is the outcome of grading one assertion.
type Result struct {
	Type       string
	Pass       bool
	Detail     string // log-safe, why it passed/failed
	NeedsJudge bool   // true only for judge_rubric (routed to the LLM judge)
}

// Grade evaluates one deterministic assertion against a response. judge_rubric
// is not graded here; it is flagged via Result.NeedsJudge for the runner to
// route to RunJudge (Task 5).
func Grade(a Assertion, r *llm.Response) Result {
	text := ""
	if r != nil {
		text = r.Text
	}
	switch a.Type {
	case "regex_match":
		return gradeRegex(a, text, true)
	case "regex_absent":
		return gradeRegex(a, text, false)
	case "contains_all":
		return gradeContains(a, text, true)
	case "contains_none":
		return gradeContains(a, text, false)
	case "language_is":
		return gradeLanguage(a, text)
	case "json_schema_valid":
		return gradeSchema(a, text)
	case "json_field_equals":
		return gradeFieldEquals(a, text)
	case "judge_rubric":
		return Result{Type: a.Type, Pass: false, NeedsJudge: true, Detail: "deferred to LLM judge"}
	default:
		return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("unknown assertion type %q", a.Type)}
	}
}

func gradeRegex(a Assertion, text string, wantMatch bool) Result {
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("bad pattern %q: %v", a.Pattern, err)}
	}
	matched := re.MatchString(text)
	pass := matched == wantMatch
	return Result{Type: a.Type, Pass: pass, Detail: fmt.Sprintf("matched=%v wantMatch=%v", matched, wantMatch)}
}

func gradeContains(a Assertion, text string, all bool) Result {
	lower := strings.ToLower(text)
	for _, v := range a.Values {
		present := strings.Contains(lower, strings.ToLower(v))
		if all && !present {
			return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("missing %q", v)}
		}
		if !all && present {
			return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("forbidden %q present", v)}
		}
	}
	return Result{Type: a.Type, Pass: true, Detail: "ok"}
}

func gradeLanguage(a Assertion, text string) Result {
	got, conf := langid.Detect(text)
	if conf < langidMinConfidence {
		return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("low confidence %.2f (got %q, want %q)", conf, got, a.Lang)}
	}
	pass := got == a.Lang
	return Result{Type: a.Type, Pass: pass, Detail: fmt.Sprintf("detected %q conf %.2f want %q", got, conf, a.Lang)}
}

func gradeSchema(a Assertion, text string) Result {
	var doc any
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &doc); err != nil {
		return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("response is not JSON: %v", err)}
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(a.Schema), &schema); err != nil {
		return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("bad schema: %v", err)}
	}
	if err := validateSchema(doc, schema); err != nil {
		return Result{Type: a.Type, Pass: false, Detail: err.Error()}
	}
	return Result{Type: a.Type, Pass: true, Detail: "valid"}
}

// validateSchema is a minimal JSON-Schema subset validator: type, required,
// properties, enum, items. It is intentionally NOT a full draft-07 validator —
// it covers the shapes the eval suites assert (object/array/string/number/
// boolean with required + enum). Unknown keywords are ignored.
func validateSchema(doc any, schema map[string]any) error {
	if t, ok := schema["type"].(string); ok {
		if err := checkType(doc, t); err != nil {
			return err
		}
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			if fmt.Sprintf("%v", e) == fmt.Sprintf("%v", doc) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("value %v not in enum", doc)
		}
	}
	obj, isObj := doc.(map[string]any)
	if isObj {
		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				name, _ := r.(string)
				if _, present := obj[name]; !present {
					return fmt.Errorf("missing required field %q", name)
				}
			}
		}
		if props, ok := schema["properties"].(map[string]any); ok {
			for name, sub := range props {
				subSchema, _ := sub.(map[string]any)
				if v, present := obj[name]; present && subSchema != nil {
					if err := validateSchema(v, subSchema); err != nil {
						return fmt.Errorf("field %q: %w", name, err)
					}
				}
			}
		}
	}
	if arr, isArr := doc.([]any); isArr {
		if items, ok := schema["items"].(map[string]any); ok {
			for i, el := range arr {
				if err := validateSchema(el, items); err != nil {
					return fmt.Errorf("item %d: %w", i, err)
				}
			}
		}
	}
	return nil
}

func checkType(doc any, t string) error {
	ok := false
	switch t {
	case "object":
		_, ok = doc.(map[string]any)
	case "array":
		_, ok = doc.([]any)
	case "string":
		_, ok = doc.(string)
	case "number", "integer":
		_, ok = doc.(float64) // encoding/json decodes all numbers to float64
	case "boolean":
		_, ok = doc.(bool)
	default:
		ok = true // unknown type keyword: don't fail
	}
	if !ok {
		return fmt.Errorf("expected type %q, got %T", t, doc)
	}
	return nil
}

// gradeFieldEquals walks a dotted path (object keys + numeric array indices)
// into the JSON response and string-compares the leaf to a.Equals.
func gradeFieldEquals(a Assertion, text string) Result {
	var doc any
	if err := json.Unmarshal([]byte(strings.TrimSpace(text)), &doc); err != nil {
		return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("response is not JSON: %v", err)}
	}
	cur := doc
	for _, seg := range strings.Split(a.Field, ".") {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[seg]
			if !ok {
				return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("path segment %q not found", seg)}
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("bad array index %q", seg)}
			}
			cur = node[idx]
		default:
			return Result{Type: a.Type, Pass: false, Detail: fmt.Sprintf("cannot descend into %T at %q", cur, seg)}
		}
	}
	got := fmt.Sprintf("%v", cur)
	pass := got == a.Equals
	return Result{Type: a.Type, Pass: pass, Detail: fmt.Sprintf("got %q want %q", got, a.Equals)}
}
