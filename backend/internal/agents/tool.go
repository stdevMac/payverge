// Package agents hosts the Payverge Ops Assistant persona.
package agents

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/llm"
)

// ToolEnv carries per-invocation context for agent tools.
type ToolEnv struct {
	BusinessID  uint
	ThreadID    uint
	Locale      string
	ActiveTab   string
	StaffRole   string
	IsStaffUser bool
	CanWrite    bool
	IsSuspended bool
	DB          *database.DB
	Analytics   *analytics.AnalyticsService
	EmailServer *emails.EmailServer
	Location    *time.Location
	PagePath    string
	SessionID   string
	// ClientIP is the requester's address as resolved by Gin (TRUSTED_PROXIES)
	// on public lanes; empty on owner/staff lanes.
	ClientIP string
}

// ToolResult is returned to the model loop.
type ToolResult struct {
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data"`
}

// Tool is implemented by the Ops tool registry.
type Tool interface {
	Name() string
	HumanLabel(locale string) string
	Description() string
	Schema() *llm.JSONSchema
	Run(ctx context.Context, args map[string]any, env ToolEnv) (ToolResult, error)
}
