// Package openrouter implements llm.Provider against OpenRouter's
// OpenAI-compatible /chat/completions API using a thin net/http client.
//
// With Config.OpenAICompatible the same client speaks the plain OpenAI
// chat-completions dialect to any compatible endpoint (Ollama, vLLM,
// LiteLLM, ...): OpenRouter-only request fields and headers are omitted and
// the API key becomes optional.
package openrouter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/circuitbreaker"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

const defaultBaseURL = "https://openrouter.ai/api/v1"

// Config configures the provider.
type Config struct {
	APIKey     string
	BaseURL    string       // defaults to defaultBaseURL
	Referer    string       // HTTP-Referer ranking header (optional)
	Title      string       // X-Title ranking header (optional)
	HTTPClient *http.Client // defaults to a 60s client
	// OpenAICompatible targets a non-OpenRouter endpoint: no `models`
	// fallback list, no `provider` routing preferences (ZDR), no
	// cache_control parts, no image_config, no HTTP-Referer/X-Title ranking
	// headers, and an empty APIKey sends no Authorization header.
	OpenAICompatible bool
}

// Provider talks to OpenRouter.
type Provider struct {
	apiKey    string
	baseURL   string
	referer   string
	title     string
	compat    bool
	http      *http.Client
	breaker   *circuitbreaker.Breaker
	observer  llm.Observer
	budget    requestBudget
	sleep     func(ctx context.Context, d time.Duration) error
	randFloat func() float64
}

type requestBudget interface {
	ReserveForRequest(context.Context, llm.GenerateRequest) (string, bool, error)
	FinalizeRequest(context.Context, string, *llm.Response, string) error
	ReleaseRequest(context.Context, string) error
}

// Option configures a Provider at construction time.
type Option func(*Provider)

// WithObserver installs a telemetry observer invoked exactly once per Generate
// call (success or error). See llm.Observer / Contract C1.
func WithObserver(obs llm.Observer) Option {
	return func(p *Provider) { p.observer = obs }
}

// WithBudget installs the durable spend reserver that wraps each Generate with
// reserve → call → finalize/release. nil disables (uncapped / no store).
func WithBudget(b *llm.CallBudget) Option {
	return func(p *Provider) { p.budget = b }
}

// New builds a Provider. APIKey is required for OpenRouter; it is optional in
// OpenAICompatible mode (a local Ollama needs none), which in turn requires an
// explicit BaseURL.
func New(cfg Config, opts ...Option) (*Provider, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if cfg.OpenAICompatible {
		if base == "" {
			return nil, fmt.Errorf("openrouter: base URL is required for an OpenAI-compatible endpoint")
		}
	} else if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("openrouter: API key is required")
	}
	if base == "" {
		base = defaultBaseURL
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	p := &Provider{
		apiKey:    strings.TrimSpace(cfg.APIKey),
		baseURL:   strings.TrimRight(base, "/"),
		referer:   cfg.Referer,
		title:     cfg.Title,
		compat:    cfg.OpenAICompatible,
		http:      hc,
		breaker:   circuitbreaker.New(circuitbreaker.Config{}),
		sleep:     ctxSleep,
		randFloat: rand.Float64,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p, nil
}

// compatSnapshotSuffix matches the dated snapshot suffix hosted vendors
// append to the model id they echo back: gpt-4o-mini -> gpt-4o-mini-2024-07-18,
// claude-3-5-sonnet -> claude-3-5-sonnet-20241022, mistral-large ->
// mistral-large-2411.
var compatSnapshotSuffix = regexp.MustCompile(`^-(\d{4}-\d{2}-\d{2}|\d{8}|\d{4})$`)

// compatReportedModel is the served model a compat-mode response reports to
// the budget ledger. Compat mode sends no `models` fallback list, so the
// endpoint serves the requested model; when a vendor echoes a dated snapshot
// of it, the call is billed under the requested id (the one OPENROUTER_PRICES
// names) instead of tripping the unpriced-served-model shutdown. Any other
// served id is kept verbatim so an unexpected model still fails closed.
func compatReportedModel(requested, served string) string {
	if requested == "" || served == requested || !strings.HasPrefix(served, requested) {
		return served
	}
	if compatSnapshotSuffix.MatchString(served[len(requested):]) {
		return requested
	}
	return served
}

// ---- wire types (OpenAI-compatible) ----

type wireRequest struct {
	Model          string          `json:"model"`
	Models         []string        `json:"models,omitempty"`
	Messages       []wireMessage   `json:"messages"`
	Tools          []wireTool      `json:"tools,omitempty"`
	Temperature    *float32        `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *wireRespFormat `json:"response_format,omitempty"`
	Modalities     []string        `json:"modalities,omitempty"`
	ImageConfig    *wireImageCfg   `json:"image_config,omitempty"`
	// Provider routes OpenRouter data-policy preferences (ZDR).
	Provider *wireProviderPrefs `json:"provider,omitempty"`
}

type wireProviderPrefs struct {
	// ZDR forces zero-data-retention routing for sensitive traffic.
	ZDR bool `json:"zdr,omitempty"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    any            `json:"content"` // string OR []wirePart
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type wirePart struct {
	Type         string         `json:"type"`
	Text         string         `json:"text,omitempty"`
	ImageURL     *wireImageURL  `json:"image_url,omitempty"`
	CacheControl *wireCacheCtrl `json:"cache_control,omitempty"`
}

type wireCacheCtrl struct {
	Type string `json:"type"` // "ephemeral"
}

type wireImageURL struct {
	URL string `json:"url"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  *llm.JSONSchema `json:"parameters,omitempty"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireToolCallFunc `json:"function"`
}

type wireToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON-encoded string
}

type wireRespFormat struct {
	Type       string           `json:"type"`
	JSONSchema *wireNamedSchema `json:"json_schema,omitempty"`
}

type wireNamedSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema *llm.JSONSchema `json:"schema"`
}

type wireImageCfg struct {
	AspectRatio string `json:"aspect_ratio,omitempty"`
	ImageSize   string `json:"image_size,omitempty"`
}

// buildRequestFor translates a neutral request into the wire shape. compat
// drops the OpenRouter-only fields so strict OpenAI-compatible servers accept
// it: server-side model fallbacks, provider routing (ZDR has no meaning off
// OpenRouter — a self-hosted model keeps data on the operator's own hardware),
// prompt-cache annotations, and image_config.
func buildRequestFor(req llm.GenerateRequest, compat bool) wireRequest {
	w := wireRequest{Model: req.Model, Temperature: req.Temperature, MaxTokens: req.MaxTokens, Modalities: req.Modalities}

	if !compat {
		if len(req.Fallbacks) > 0 {
			w.Models = append([]string{req.Model}, req.Fallbacks...)
		}
		// Never strip ZDR on fallbacks: when the feature requires it, every model
		// in the preference list is routed with provider.zdr=true.
		if req.RequiresZDR() {
			w.Provider = &wireProviderPrefs{ZDR: true}
		}
	}

	if req.System != "" {
		if req.CacheControl && !compat {
			w.Messages = append(w.Messages, wireMessage{
				Role: "system",
				Content: []wirePart{{
					Type:         "text",
					Text:         req.System,
					CacheControl: &wireCacheCtrl{Type: "ephemeral"},
				}},
			})
		} else {
			w.Messages = append(w.Messages, wireMessage{Role: "system", Content: req.System})
		}
	}
	for _, m := range req.Messages {
		w.Messages = append(w.Messages, buildMessage(m))
	}

	for _, t := range req.Tools {
		w.Tools = append(w.Tools, wireTool{
			Type: "function",
			Function: wireFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		})
	}

	switch {
	case req.ResponseSchema != nil:
		w.ResponseFormat = &wireRespFormat{
			Type: "json_schema",
			JSONSchema: &wireNamedSchema{
				Name:   "response",
				Strict: true,
				Schema: req.ResponseSchema,
			},
		}
	case req.JSONMode:
		w.ResponseFormat = &wireRespFormat{Type: "json_object"}
	}

	if req.ImageConfig != nil && !compat {
		w.ImageConfig = &wireImageCfg{
			AspectRatio: req.ImageConfig.AspectRatio,
			ImageSize:   req.ImageConfig.ImageSize,
		}
	}
	return w
}

func buildMessage(m llm.Message) wireMessage {
	wm := wireMessage{Role: string(m.Role)}

	switch m.Role {
	case llm.RoleTool:
		wm.ToolCallID = m.ToolCallID
		wm.Name = m.ToolName
		wm.Content = m.Text
		return wm
	case llm.RoleAssistant:
		wm.Content = m.Text // may be ""
		for _, tc := range m.ToolCalls {
			args, _ := json.Marshal(tc.Args)
			wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: wireToolCallFunc{
					Name:      tc.Name,
					Arguments: string(args),
				},
			})
		}
		return wm
	}

	// user (and any other) role: text + optional images
	if len(m.Images) == 0 {
		wm.Content = m.Text
		return wm
	}
	parts := []wirePart{}
	if m.Text != "" {
		parts = append(parts, wirePart{Type: "text", Text: m.Text})
	}
	for _, img := range m.Images {
		parts = append(parts, wirePart{
			Type:     "image_url",
			ImageURL: &wireImageURL{URL: dataURI(img.MIMEType, img.Data)},
		})
	}
	wm.Content = parts
	return wm
}

func dataURI(mime string, data []byte) string {
	if mime == "" {
		mime = "application/octet-stream"
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
}

// ---- response parsing ----

type wireResponse struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Choices []wireChoice `json:"choices"`
	Usage   wireUsage    `json:"usage"`
	Error   *wireError   `json:"error"`
}

type wireChoice struct {
	Message      wireRespMessage `json:"message"`
	FinishReason string          `json:"finish_reason"`
}

type wireRespMessage struct {
	Content   string          `json:"content"`
	ToolCalls []wireToolCall  `json:"tool_calls"`
	Images    []wireRespImage `json:"images"`
}

type wireRespImage struct {
	Type     string       `json:"type"`
	ImageURL wireImageURL `json:"image_url"`
}

type wireUsage struct {
	PromptTokens        int                `json:"prompt_tokens"`
	CompletionTokens    int                `json:"completion_tokens"`
	TotalTokens         int                `json:"total_tokens"`
	PromptTokensDetails *wirePromptDetails `json:"prompt_tokens_details,omitempty"`
}

type wirePromptDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type wireError struct {
	Message string          `json:"message"`
	Code    json.RawMessage `json:"code"` // may be int or string; unused, kept tolerant
}

// Generate sends one chat-completions request and returns the parsed result.
func (p *Provider) doGenerate(ctx context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	body, err := json.Marshal(buildRequestFor(req, p.compat))
	if err != nil {
		return nil, fmt.Errorf("openrouter: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openrouter: build http request: %w", err)
	}
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if !p.compat {
		if p.referer != "" {
			httpReq.Header.Set("HTTP-Referer", p.referer)
		}
		if p.title != "" {
			httpReq.Header.Set("X-Title", p.title)
		}
	}

	var resp *http.Response
	doErr := p.breaker.Do(func() error {
		var e error
		resp, e = p.http.Do(httpReq)
		return e
	})
	if doErr != nil {
		// Both a transport failure and a short-circuited ErrOpen surface as an
		// upstream error so callers (and isRetryable) treat them uniformly.
		return nil, fmt.Errorf("%w: %v", llm.ErrUpstream, doErr)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openrouter: read body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyError(resp.StatusCode, raw)
	}

	var wr wireResponse
	if err := json.Unmarshal(raw, &wr); err != nil {
		return nil, fmt.Errorf("%w: %v", llm.ErrMalformedResponse, err)
	}
	if len(wr.Choices) == 0 {
		return nil, fmt.Errorf("%w: no choices", llm.ErrMalformedResponse)
	}

	msg := wr.Choices[0].Message
	out := &llm.Response{
		Text:              msg.Content,
		Model:             wr.Model,
		FinishReason:      wr.Choices[0].FinishReason,
		ProviderRequestID: wr.ID,
		Usage: llm.Usage{
			PromptTokens:     wr.Usage.PromptTokens,
			CompletionTokens: wr.Usage.CompletionTokens,
			TotalTokens:      wr.Usage.TotalTokens,
			CachedTokens:     cachedTokens(wr.Usage),
		},
	}
	if p.compat {
		out.Model = compatReportedModel(req.Model, out.Model)
	}
	for _, tc := range msg.ToolCalls {
		args := map[string]any{}
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		out.ToolCalls = append(out.ToolCalls, llm.ToolCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: args,
		})
	}
	for _, img := range msg.Images {
		mime, data, derr := decodeDataURI(img.ImageURL.URL)
		if derr != nil {
			continue
		}
		out.Images = append(out.Images, llm.ImageOutput{MIMEType: mime, Data: data})
	}
	return out, nil
}

func classifyError(status int, body []byte) error {
	var we struct {
		Error wireError `json:"error"`
	}
	_ = json.Unmarshal(body, &we)
	msg := we.Error.Message
	if msg == "" {
		msg = string(body)
	}
	class := llm.ErrUpstream
	switch {
	case status == 401 || status == 403:
		class = llm.ErrAuth
	case status == 429:
		class = llm.ErrRateLimited
	}
	return &llm.APIError{Class: class, Status: status, Message: msg}
}

func cachedTokens(u wireUsage) int {
	if u.PromptTokensDetails == nil {
		return 0
	}
	return u.PromptTokensDetails.CachedTokens
}

// decodeDataURI parses "data:<mime>;base64,<payload>" into mime + bytes.
func decodeDataURI(uri string) (string, []byte, error) {
	const b64Marker = ";base64,"
	idx := strings.Index(uri, b64Marker)
	if !strings.HasPrefix(uri, "data:") || idx < 0 {
		return "", nil, fmt.Errorf("not a base64 data URI")
	}
	mime := uri[len("data:"):idx]
	data, err := base64.StdEncoding.DecodeString(uri[idx+len(b64Marker):])
	if err != nil {
		return "", nil, err
	}
	return mime, data, nil
}

var _ llm.Provider = (*Provider)(nil)
