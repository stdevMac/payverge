// Package llm defines a provider-neutral interface for LLM calls so the
// application depends on no vendor SDK. OpenRouter is the first implementation
// (see internal/llm/openrouter); others can be added behind the same Provider.
package llm

import "context"

// Role identifies the author of a Message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// JSON Schema type constants (mirror the JSON Schema "type" keyword). Named to
// match the previous genai.Type* identifiers so call-site migration is a clean
// token swap.
const (
	TypeObject  = "object"
	TypeString  = "string"
	TypeNumber  = "number"
	TypeInteger = "integer"
	TypeBoolean = "boolean"
	TypeArray   = "array"
	TypeNull    = "null"
)

// ImageInput is a user-supplied image (multimodal input).
type ImageInput struct {
	MIMEType string
	Data     []byte
}

// ImageOutput is a model-generated image (image-gen output).
type ImageOutput struct {
	MIMEType string
	Data     []byte
}

// ToolCall is a function call the model requested.
type ToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// Message is one turn in the conversation.
type Message struct {
	Role       Role
	Text       string
	Images     []ImageInput // user image inputs
	ToolCalls  []ToolCall   // assistant tool-call requests
	ToolCallID string       // Role==RoleTool: the call this answers
	ToolName   string       // Role==RoleTool: the tool name
}

// JSONSchema is a provider-neutral JSON-Schema subset. Field tags let it
// marshal directly to a valid JSON Schema object for the wire.
type JSONSchema struct {
	Type                 string                 `json:"type,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Items                *JSONSchema            `json:"items,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Enum                 []string               `json:"enum,omitempty"`
	MaxLength            *int                   `json:"maxLength,omitempty"`
	MaxItems             *int                   `json:"maxItems,omitempty"`
	AdditionalProperties *bool                  `json:"additionalProperties,omitempty"`
	AnyOf                []*JSONSchema          `json:"anyOf,omitempty"`
	Const                any                    `json:"const,omitempty"`
}

// Tool is a callable function exposed to the model.
type Tool struct {
	Name        string
	Description string
	Parameters  *JSONSchema
}

// ImageConfig configures image-gen output (OpenRouter image_config).
type ImageConfig struct {
	AspectRatio string // e.g. "1:1", "16:9"
	ImageSize   string // e.g. "4K"
}

// PrivacyClass classifies the sensitivity of a model request for ZDR routing.
type PrivacyClass string

const (
	PrivacyPublic               PrivacyClass = "public"
	PrivacyCustomerSensitive    PrivacyClass = "customer_sensitive"
	PrivacyBusinessConfidential PrivacyClass = "business_confidential"
)

// GenerateRequest is a single non-streaming generation request.
type GenerateRequest struct {
	Model          string
	System         string
	Messages       []Message
	Tools          []Tool
	Temperature    *float32
	JSONMode       bool         // response_format: {type:"json_object"}
	ResponseSchema *JSONSchema  // response_format: {type:"json_schema",...}
	Modalities     []string     // e.g. ["image","text"]
	ImageConfig    *ImageConfig // image-gen only
	Fallbacks      []string     // OpenRouter models[] after the primary
	MaxTokens      int          // 0 = provider default; wire as "max_tokens"
	Feature        string       // telemetry tag: "waiter"|"waiter_whatsapp"|"director"|"wizard"|"extraction"|"image"|"guardrail" (not sent on the wire)
	CacheControl   bool         // Lane J: mark the system prompt as a cacheable prefix (cache_control: ephemeral)
	// BusinessID is set by waiter/director callers for Lane J's per-business
	// cost roll-up (master contract C1). Telemetry only; never serialized on wire.
	BusinessID uint
	// PrivacyClass drives zero-data-retention routing. Empty/unknown is invalid.
	PrivacyClass PrivacyClass
	// BudgetAudience selects the per-business daily spend scope: guest-facing
	// lanes (BudgetAudienceGuest) and owner/staff lanes (BudgetAudienceOwner)
	// each get their own ceiling so anonymous guests cannot exhaust the
	// owner's tools. Empty derives it from Feature ("waiter",
	// "waiter_whatsapp" are guest; everything else is owner). Never on wire.
	BudgetAudience string
}

// Usage holds token accounting from the response.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CachedTokens     int // prompt tokens served from cache (usage.prompt_tokens_details.cached_tokens)
}

// Response is a single non-streaming generation result.
type Response struct {
	Text              string
	ToolCalls         []ToolCall
	Images            []ImageOutput
	Model             string // model actually used (reflects any fallback)
	Usage             Usage
	FinishReason      string
	ProviderRequestID string
}

// Provider is the single contract every LLM backend implements.
type Provider interface {
	Generate(ctx context.Context, req GenerateRequest) (*Response, error)
}
