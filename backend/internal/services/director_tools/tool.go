// Package director_tools registers and runs the Director Console's
// tool-call tools. Each Tool exposes a JSON-schema-shaped
// argument signature and a Run method that returns a localized human
// summary plus structured data the model sees in the tool result.
package director_tools

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// ToolEnv carries per-invocation context handed to every tool.
type ToolEnv struct {
	BusinessID uint
	ThreadID   uint
	Locale     string
	DB         *database.DB
	Analytics  *analytics.AnalyticsService
	// Location is the business's resolved timezone, used so revenue/period
	// analytics align to the business's local calendar instead of the server
	// clock. Nil is treated as UTC by the analytics service.
	Location *time.Location
	// FreezeWrites is set when the operator explicitly said not to change
	// anything (preview only / "no cambies nada" / "do not apply"). Proposal
	// tools still return a dry-run diff but must not persist a queued apply.
	FreezeWrites bool
}

// ToolResult is what a tool returns to the loop.
// Summary is a short human-readable string surfaced in the UI pill.
// Data is the structured payload returned to the model as a tool result.
type ToolResult struct {
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data"`
	// ProposalID is set by proposal tools to the primary-key id of the pending
	// director_proposed_actions row they persisted. The loop collects non-zero
	// values so the service can assemble proposed_actions from server-truth rows
	// (never from model free-text). Zero for read tools.
	ProposalID uint `json:"-"`
}

// Tool is the contract every registered Director Console tool implements.
// Implementations must be stateless and safe to call concurrently.
type Tool interface {
	Name() string                    // snake_case identifier; matches tool call name
	HumanLabel(locale string) string // localized pill label, e.g. "Reading weekly revenue"
	// Description is the model-facing tool description sent to the provider
	// (OpenRouter function.description). Unlike HumanLabel (a localized UI pill),
	// it tells the model WHEN to call the tool and WHAT it returns. English.
	Description() string
	Schema() *llm.JSONSchema // argument schema (llm.TypeObject)
	Run(ctx context.Context, args map[string]any, env ToolEnv) (ToolResult, error)
}
