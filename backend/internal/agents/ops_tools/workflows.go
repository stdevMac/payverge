package ops_tools

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_guides"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"
)

var opsGuideCatalog = ops_guides.NewDefaultCatalog()

// BusinessProfileAdapter wraps director_tools.BusinessProfileTool.
type BusinessProfileAdapter struct {
	inner director_tools.BusinessProfileTool
}

func (t *BusinessProfileAdapter) Name() string                    { return "get_business_profile" }
func (t *BusinessProfileAdapter) HumanLabel(locale string) string { return t.inner.HumanLabel(locale) }
func (t *BusinessProfileAdapter) Description() string             { return t.inner.Description() }
func (t *BusinessProfileAdapter) Schema() *llm.JSONSchema         { return t.inner.Schema() }
func (t *BusinessProfileAdapter) Run(ctx context.Context, args map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	res, err := t.inner.Run(ctx, args, directorEnv(env))
	if err != nil {
		return agents.ToolResult{}, err
	}
	return agents.ToolResult{Summary: res.Summary, Data: res.Data}, nil
}

func directorEnv(env agents.ToolEnv) director_tools.ToolEnv {
	return director_tools.ToolEnv{
		BusinessID: env.BusinessID,
		ThreadID:   env.ThreadID,
		Locale:     env.Locale,
		DB:         env.DB,
		Analytics:  env.Analytics,
		Location:   env.Location,
	}
}

type WorkflowTool struct{}

func (t *WorkflowTool) Name() string { return "get_workflow" }
func (t *WorkflowTool) HumanLabel(locale string) string {
	return label(locale, "Loading workflow", "Cargando flujo")
}
func (t *WorkflowTool) Description() string {
	return "Returns multi-step guided workflow: first_menu_item, first_table_qr, connect_stripe, enable_ai_waiter."
}
func (t *WorkflowTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"workflow_id": {Type: llm.TypeString},
			"step_index":  {Type: llm.TypeInteger, Description: "0-based step index"},
		},
		Required: []string{"workflow_id"},
	}
}
func (t *WorkflowTool) Run(_ context.Context, args map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	id := strArg(args, "workflow_id")
	step := intArg(args, "step_index")
	wf, ok := workflows[id]
	if !ok {
		return agents.ToolResult{}, fmt.Errorf("unknown workflow %q", id)
	}
	if step < 0 {
		step = 0
	}
	if step >= len(wf.Steps) {
		step = len(wf.Steps) - 1
	}
	s := wf.Steps[step]
	href := s.Href
	if strings.Contains(href, "{{business_id}}") {
		href = strings.ReplaceAll(href, "{{business_id}}", fmt.Sprint(env.BusinessID))
	}
	return agents.ToolResult{
		Summary: wf.Title,
		Data: map[string]any{
			"workflow_id": id, "step_index": step, "step_total": len(wf.Steps),
			"step": map[string]any{"title": s.Title, "body": s.Body, "href": href},
		},
	}, nil
}

type DelegateToDirectorTool struct{}

func (t *DelegateToDirectorTool) Name() string { return "delegate_to_director" }
func (t *DelegateToDirectorTool) HumanLabel(locale string) string {
	return label(locale, "Preparing Director handoff", "Preparando Director")
}
func (t *DelegateToDirectorTool) Description() string {
	return "Returns Director Console handoff action with suggested prompt when the owner or director:write is available."
}
func (t *DelegateToDirectorTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"prompt": {Type: llm.TypeString, Description: "Suggested question for Director composer"},
		},
		Required: []string{"prompt"},
	}
}
func (t *DelegateToDirectorTool) Run(_ context.Context, args map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	prompt := strArg(args, "prompt")
	canHandoff := !env.IsSuspended && !env.IsStaffUser // owners; staff need director:write checked at handler
	href := dashboardHref(env.BusinessID, "director-console")
	data := map[string]any{
		"href": href, "prompt": prompt, "kind": "handoff",
		"disabled": !canHandoff,
	}
	if !canHandoff {
		data["disabled_reason"] = "Director Console requires an active business and owner or director:write permission"
	}
	return agents.ToolResult{Summary: "Director handoff prepared", Data: data}, nil
}

type SupportEscalationTool struct{}

func (t *SupportEscalationTool) Name() string { return "create_support_escalation" }
func (t *SupportEscalationTool) HumanLabel(locale string) string {
	return label(locale, "Escalating support", "Escalando soporte")
}
func (t *SupportEscalationTool) Description() string {
	return "Email support with tab, role, business id, and transcript summary. Requires assistant:write."
}
func (t *SupportEscalationTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"transcript_summary": {Type: llm.TypeString},
			"issue":              {Type: llm.TypeString},
		},
		Required: []string{"issue"},
	}
}
func (t *SupportEscalationTool) Run(_ context.Context, args map[string]any, env agents.ToolEnv) (agents.ToolResult, error) {
	if !env.CanWrite {
		return agents.ToolResult{}, fmt.Errorf("assistant:write required")
	}
	issue := strArg(args, "issue")
	transcript := strArg(args, "transcript_summary")

	// Email delivery is best-effort: a tool error here would abort the loop and
	// suppress OnEscalate — exactly when the durable escalation row + Telegram
	// fallback matter most. Record the failure in the tool result instead.
	var emailErr error
	if env.EmailServer == nil {
		emailErr = fmt.Errorf("email unavailable")
	} else {
		subject := fmt.Sprintf("Ops Assistant escalation — business %d", env.BusinessID)
		htmlBody := "<h2>Ops Assistant support escalation</h2>" +
			"<p><strong>Business ID:</strong> " + html.EscapeString(fmt.Sprint(env.BusinessID)) + "</p>" +
			"<p><strong>Tab:</strong> " + html.EscapeString(env.ActiveTab) + "</p>" +
			"<p><strong>Role:</strong> " + html.EscapeString(env.StaffRole) + "</p>" +
			"<p><strong>Issue:</strong> " + html.EscapeString(issue) + "</p>" +
			"<p><strong>Transcript:</strong> " + html.EscapeString(transcript) + "</p>"
		emailErr = env.EmailServer.SendCustomEmail(emails.AdminsEmails, subject, htmlBody, issue)
	}
	if emailErr != nil {
		logger.Logger.Warnf("ops escalation: admin email failed (business %d), escalation still recorded: %v", env.BusinessID, emailErr)
		return agents.ToolResult{
			Summary: "Escalation recorded; email notification failed",
			Data:    map[string]any{"ok": true, "email_sent": false, "note": "escalation recorded; email notification failed"},
		}, nil
	}
	return agents.ToolResult{Summary: "Support escalation sent", Data: map[string]any{"ok": true, "email_sent": true}}, nil
}

func intArg(args map[string]any, key string) int {
	if args == nil {
		return 0
	}
	v, ok := args[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	default:
		return 0
	}
}

type workflowDef struct {
	Title string
	Steps []workflowStep
}
type workflowStep struct {
	Title, Body, Href string
}

var workflows = map[string]workflowDef{
	"first_menu_item": {
		Title: "Add your first menu item",
		Steps: []workflowStep{
			{Title: "Open Menu", Body: "Go to Menu Builder", Href: "/business/{{business_id}}/dashboard?tab=menu"},
			{Title: "Add category", Body: "Create or select a category", Href: "/business/{{business_id}}/dashboard?tab=menu"},
			{Title: "Add item", Body: "Add name, price, and photo", Href: "/business/{{business_id}}/dashboard?tab=menu"},
			{Title: "Preview", Body: "Check Business Page preview", Href: "/business/{{business_id}}/dashboard?tab=business-page"},
		},
	},
	"first_table_qr": {
		Title: "Create your first table QR",
		Steps: []workflowStep{
			{Title: "Open Tables", Body: "Go to Tables tab", Href: "/business/{{business_id}}/dashboard?tab=tables"},
			{Title: "Create table", Body: "Add label and capacity", Href: "/business/{{business_id}}/dashboard?tab=tables"},
			{Title: "Download QR", Body: "Download QR from row actions", Href: "/business/{{business_id}}/dashboard?tab=tables"},
		},
	},
	"connect_stripe": {
		Title: "Connect Stripe",
		Steps: []workflowStep{
			{Title: "Plugins", Body: "Open Plugins tab", Href: "/business/{{business_id}}/dashboard?tab=plugins"},
			{Title: "Stripe", Body: "Select Stripe and enter credentials", Href: "/business/{{business_id}}/dashboard?tab=plugins"},
			{Title: "Enable", Body: "Toggle plugin on", Href: "/business/{{business_id}}/dashboard?tab=plugins"},
		},
	},
	"enable_ai_waiter": {
		Title: "Enable AI Waiter",
		Steps: []workflowStep{
			{Title: "AI Waiter", Body: "Open AI Waiter tab", Href: "/business/{{business_id}}/dashboard?tab=ai-waiter"},
			{Title: "Enable", Body: "Turn on guest AI", Href: "/business/{{business_id}}/dashboard?tab=ai-waiter"},
			{Title: "Business page", Body: "Verify toggle on public page", Href: "/business/{{business_id}}/dashboard?tab=business-page"},
		},
	},
}
