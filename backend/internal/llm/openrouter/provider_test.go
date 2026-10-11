package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

// newTestProvider points the provider at an httptest server and captures the
// last request body it received.
func newTestProvider(t *testing.T, handler http.HandlerFunc) (*Provider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := New(Config{APIKey: "test-key", BaseURL: srv.URL, Referer: "https://payverge.io", Title: "Payverge"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p, srv
}

func TestGenerateCapturesProviderRequestIDAndFinishReason(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"gen-abc","model":"test/model","choices":[{"finish_reason":"length","message":{"content":"truncated"}}],"usage":{"completion_tokens":4096}}`))
	})
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Model: "test/model", Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
		PrivacyClass: llm.PrivacyBusinessConfidential, Feature: "wizard",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.ProviderRequestID != "gen-abc" || resp.FinishReason != "length" {
		t.Fatalf("metadata = request %q finish %q", resp.ProviderRequestID, resp.FinishReason)
	}
}

func TestGenerateTextAndHeaders(t *testing.T) {
	var gotAuth, gotReferer, gotTitle string
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotReferer = r.Header.Get("HTTP-Referer")
		gotTitle = r.Header.Get("X-Title")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"model":"google/gemini-2.0-flash-001","choices":[{"message":{"content":"hi there"}}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`))
	})

	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.0-flash-001",
		System:   "you are a waiter",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "hi there" {
		t.Fatalf("text = %q", resp.Text)
	}
	if resp.Model != "google/gemini-2.0-flash-001" || resp.Usage.TotalTokens != 5 {
		t.Fatalf("model/usage = %q/%d", resp.Model, resp.Usage.TotalTokens)
	}
	if gotAuth != "Bearer test-key" || gotReferer != "https://payverge.io" || gotTitle != "Payverge" {
		t.Fatalf("headers: auth=%q referer=%q title=%q", gotAuth, gotReferer, gotTitle)
	}
	msgs := gotBody["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" {
		t.Fatalf("messages = %v", msgs)
	}
}

func TestGenerateFallbackModelsArray(t *testing.T) {
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"model":"openai/gpt-4o-mini","choices":[{"message":{"content":"ok"}}]}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:   "test",
		Model:     "google/gemini-2.0-flash-001",
		Messages:  []llm.Message{{Role: llm.RoleUser, Text: "x"}},
		Fallbacks: []string{"openai/gpt-4o-mini"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	models := gotBody["models"].([]any)
	if len(models) != 2 || models[0] != "google/gemini-2.0-flash-001" || models[1] != "openai/gpt-4o-mini" {
		t.Fatalf("models = %v", models)
	}
}

func TestGenerateToolsAndToolCallParsing(t *testing.T) {
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"add_to_cart","arguments":"{\"item_name\":\"Burger\",\"quantity\":2}"}}]}}]}`))
	})
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.0-flash-001",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "add a burger"}},
		Tools: []llm.Tool{{
			Name:        "add_to_cart",
			Description: "Add item",
			Parameters: &llm.JSONSchema{
				Type:       llm.TypeObject,
				Properties: map[string]*llm.JSONSchema{"item_name": {Type: llm.TypeString}},
				Required:   []string{"item_name"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.Name != "add_to_cart" || tc.Args["item_name"] != "Burger" || tc.Args["quantity"].(float64) != 2 {
		t.Fatalf("tool call = %+v", tc)
	}
	tools := gotBody["tools"].([]any)
	fn := tools[0].(map[string]any)["function"].(map[string]any)
	if fn["name"] != "add_to_cart" {
		t.Fatalf("wire tool = %v", tools)
	}
}

func TestGenerateResponseSchema(t *testing.T) {
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:        "test",
		Model:          "google/gemini-2.5-flash",
		Messages:       []llm.Message{{Role: llm.RoleUser, Text: "x"}},
		ResponseSchema: &llm.JSONSchema{Type: llm.TypeObject},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	rf := gotBody["response_format"].(map[string]any)
	if rf["type"] != "json_schema" {
		t.Fatalf("response_format = %v", rf)
	}
}

func TestGenerateResponseSchemaSerializesAdditionalPropertiesFalse(t *testing.T) {
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	})
	additionalProperties := false
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.5-flash",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
		ResponseSchema: &llm.JSONSchema{
			Type:                 llm.TypeObject,
			AdditionalProperties: &additionalProperties,
		},
	})
	require.NoError(t, err)

	responseFormat := gotBody["response_format"].(map[string]any)
	namedSchema := responseFormat["json_schema"].(map[string]any)
	schema := namedSchema["schema"].(map[string]any)
	value, present := schema["additionalProperties"]
	require.True(t, present)
	require.Equal(t, false, value)
}

func TestGenerateImageOutput(t *testing.T) {
	var gotBody map[string]any
	// 1x1 transparent PNG.
	const pngB64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"choices":[{"message":{"content":"done","images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + pngB64 + `"}}]}}]}`))
	})
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:     "test",
		Model:       "google/gemini-2.5-flash-image",
		Messages:    []llm.Message{{Role: llm.RoleUser, Text: "draw a burger"}},
		Modalities:  []string{"image", "text"},
		ImageConfig: &llm.ImageConfig{AspectRatio: "1:1"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(resp.Images) != 1 || resp.Images[0].MIMEType != "image/png" || len(resp.Images[0].Data) == 0 {
		t.Fatalf("images = %+v", resp.Images)
	}
	if gotBody["image_config"].(map[string]any)["aspect_ratio"] != "1:1" {
		t.Fatalf("image_config = %v", gotBody["image_config"])
	}
}

func TestGenerateImageInputDataURI(t *testing.T) {
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"choices":[{"message":{"content":"{}"}}]}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature: "test",
		Model:   "google/gemini-2.5-flash",
		Messages: []llm.Message{{
			Role:   llm.RoleUser,
			Text:   "extract this",
			Images: []llm.ImageInput{{MIMEType: "image/jpeg", Data: []byte("fakejpeg")}},
		}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	content := gotBody["messages"].([]any)[0].(map[string]any)["content"].([]any)
	last := content[len(content)-1].(map[string]any)
	if last["type"] != "image_url" {
		t.Fatalf("content = %v", content)
	}
	url := last["image_url"].(map[string]any)["url"].(string)
	if url[:len("data:image/jpeg;base64,")] != "data:image/jpeg;base64," {
		t.Fatalf("data uri = %q", url)
	}
}

func TestGenerateErrorClassification(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limit","code":429}}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.0-flash-001",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestGenerateAuthErrorClassification(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"no key","code":"invalid_api_key"}}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.0-flash-001",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrAuth) {
		t.Fatalf("expected ErrAuth, got %v", err)
	}
	// String-typed error code must not clobber the human-readable message.
	var apiErr *llm.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *llm.APIError, got %T", err)
	}
	if apiErr.Message != "no key" {
		t.Fatalf("message = %q, want %q", apiErr.Message, "no key")
	}
}

func TestGenerateUpstreamErrorClassification(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"boom"}}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.0-flash-001",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrUpstream) {
		t.Fatalf("expected ErrUpstream, got %v", err)
	}
}

func TestGenerateMalformedJSON(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json at all`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.0-flash-001",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestGenerateNoChoices(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"model":"google/gemini-2.0-flash-001","choices":[]}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.0-flash-001",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if !errors.Is(err, llm.ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse, got %v", err)
	}
}

func TestGenerateServedModelAndUsageFromPayload(t *testing.T) {
	const fixture = `{
		"model":"openai/gpt-4o-mini",
		"choices":[{"message":{"content":"served by fallback"}}],
		"usage":{"prompt_tokens":120,"completion_tokens":45,"total_tokens":165}
	}`
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(fixture))
	})
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:   "test",
		Model:     "google/gemini-2.0-flash-001",
		Messages:  []llm.Message{{Role: llm.RoleUser, Text: "x"}},
		Fallbacks: []string{"openai/gpt-4o-mini"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Model != "openai/gpt-4o-mini" {
		t.Fatalf("served model = %q, want fallback model", resp.Model)
	}
	if resp.Usage.PromptTokens != 120 || resp.Usage.CompletionTokens != 45 || resp.Usage.TotalTokens != 165 {
		t.Fatalf("usage = %+v, want {120 45 165}", resp.Usage)
	}
}

func TestGenerateUsageDefaultsZeroWhenAbsent(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"model":"google/gemini-2.0-flash-001","choices":[{"message":{"content":"hi"}}]}`))
	})
	resp, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "google/gemini-2.0-flash-001",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Usage != (llm.Usage{}) {
		t.Fatalf("usage = %+v, want zero value", resp.Usage)
	}
	if resp.Model != "google/gemini-2.0-flash-001" {
		t.Fatalf("model = %q", resp.Model)
	}
}

func TestGenerate_SensitiveFeatureSetsProviderZDR(t *testing.T) {
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:   "waiter",
		Model:     "google/gemini-2.0-flash-001",
		Messages:  []llm.Message{{Role: llm.RoleUser, Text: "x"}},
		Fallbacks: []string{"openai/gpt-4o-mini"},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	prov, ok := gotBody["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider prefs missing: %v", gotBody)
	}
	if prov["zdr"] != true {
		t.Fatalf("provider.zdr = %v, want true", prov["zdr"])
	}
	// Privacy class must never leak onto the OpenRouter wire body.
	if _, has := gotBody["privacy_class"]; has {
		t.Fatalf("privacy_class must not be serialized: %v", gotBody)
	}
	if _, has := gotBody["PrivacyClass"]; has {
		t.Fatalf("PrivacyClass must not be serialized: %v", gotBody)
	}
}

func TestGenerate_TestFeatureOmitsZDR(t *testing.T) {
	var gotBody map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "test",
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, has := gotBody["provider"]; has {
		t.Fatalf("public/test feature must not force provider.zdr: %v", gotBody["provider"])
	}
}

func TestGenerate_UnknownFeatureFailsBeforeHTTP(t *testing.T) {
	var calls int
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	})
	_, err := p.Generate(context.Background(), llm.GenerateRequest{
		Feature:  "",
		Model:    "m",
		Messages: []llm.Message{{Role: llm.RoleUser, Text: "x"}},
	})
	if err == nil {
		t.Fatal("expected privacy policy error")
	}
	if !errors.Is(err, llm.ErrPrivacyPolicy) {
		t.Fatalf("err=%v, want ErrPrivacyPolicy", err)
	}
	if calls != 0 {
		t.Fatalf("HTTP calls=%d, want 0 (fail before network)", calls)
	}
}

func TestBuildRequestMaxTokensSerialization(t *testing.T) {
	cases := []struct {
		name      string
		maxTokens int
		wantKey   bool
		wantVal   float64
	}{
		{name: "zero omits max_tokens", maxTokens: 0, wantKey: false},
		{name: "positive sets max_tokens", maxTokens: 1024, wantKey: true, wantVal: 1024},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotBody map[string]any
			p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &gotBody)
				w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
			})
			_, err := p.Generate(context.Background(), llm.GenerateRequest{
				Model:     "google/gemini-2.0-flash-001",
				Messages:  []llm.Message{{Role: llm.RoleUser, Text: "x"}},
				MaxTokens: tc.maxTokens,
				Feature:   "waiter",
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			val, present := gotBody["max_tokens"]
			if present != tc.wantKey {
				t.Fatalf("max_tokens present=%v, want %v (body=%v)", present, tc.wantKey, gotBody)
			}
			if tc.wantKey && val.(float64) != tc.wantVal {
				t.Fatalf("max_tokens=%v, want %v", val, tc.wantVal)
			}
		})
	}
}
